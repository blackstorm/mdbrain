(ns mdbrain.handlers.app
  (:require
   [clojure.string :as str]
   [clojure.tools.logging :as log]
   [mdbrain.config :as config]
   [mdbrain.db :as db]
   [mdbrain.markdown :as md]
   [mdbrain.middleware :as middleware]
   [mdbrain.object-store :as object-store]
   [mdbrain.response :as resp]
   [mdbrain.template-assets :as template-assets]
   [mdbrain.utils.stream :as utils.stream]
   [selmer.parser :as selmer])
  (:import [java.io InputStream]))

(defn- render-template
  [template context]
  (template-assets/register-filter!)
  (selmer/render-file template context))

(defn get-current-vault
  [request]
  (or (:mdbrain/vault request)
      (when-let [host (get-in request [:headers "host"])]
        (let [domain (first (str/split host #":"))]
          (db/get-vault-by-domain domain)))))

(defn- vault-context
  "Resolve current vault from request.
   Returns:
   - {:vault <vault>} when resolved
   - {:error :missing|:invalid|:unbound} otherwise"
  [request]
  (if-let [vault (:mdbrain/vault request)]
    {:vault vault}
    (let [{:keys [domain error]} (middleware/parse-host-domain (get-in request [:headers "host"]))]
      (cond
        error {:error error}
        :else (if-let [vault (db/get-vault-by-domain domain)]
                {:vault vault}
                {:error :unbound})))))

(defn- host-error-response
  [error]
  (case error
    (:missing :invalid)
    {:status 400
     :headers {"Content-Type" "text/plain; charset=utf-8"
               "Cache-Control" "no-store"}
     :body "Bad request"}

    :unbound
    {:status 403
     :headers {"Content-Type" "text/plain; charset=utf-8"
               "Cache-Control" "no-store"}
     :body "Forbidden"}))

(defn- with-vault
  "Run (f vault request) when host->vault is resolved.
   Otherwise return an error response (400/403)."
  [request f]
  (let [{:keys [vault error]} (vault-context request)]
    (if error
      (host-error-response error)
      (f vault request))))

(defn- query-param
  [request param-name]
  (or (get-in request [:query-params param-name])
      (get-in request [:params param-name])
      (get-in request [:params (keyword param-name)])
      (when-let [qs (:query-string request)]
        (some (fn [pair]
                (let [[k v] (str/split pair #"=" 2)]
                  (when (= k param-name)
                    (when v
                      (try
                        (java.net.URLDecoder/decode v "UTF-8")
                        (catch Exception _ v))))))
              (str/split qs #"&")))))

(defn parse-path-ids
  [path]
  (if (or (nil? path) (= path "/") (= path ""))
    []
    (-> path
        (str/replace #"^/" "")
        (str/split #"\+")
        vec)))

(defn build-push-url
  [current-url from-note-id target-note-id root-note-id]
  (let [current-path (if current-url
                       (-> current-url
                           (str/replace #"^https?://[^/]+" "")
                           (str/replace #"\?.*$" ""))
                       "/")]
    (cond
      (= current-path "/")
      (if root-note-id
        (str "/" root-note-id "+" target-note-id)
        (str "/" target-note-id))
      
      :else
      (let [path-parts (-> current-path
                           (str/replace #"^/" "")
                           (str/split #"\+"))]
        (if from-note-id
          (let [idx (.indexOf (vec path-parts) from-note-id)
                kept-parts (if (>= idx 0)
                             (take (inc idx) path-parts)
                             path-parts)]
            (str "/" (str/join "+" (concat kept-parts [target-note-id]))))
          (str "/" (str/join "+" (concat path-parts [target-note-id]))))))))

(defn- prepare-note-data
  [note vault-id]
  (let [links (db/get-note-links vault-id (:client-id note))
        html-content (md/render-markdown (:content note) vault-id links)
        title (or (md/extract-title (:content note))
                  (-> (:path note)
                      (str/replace #"\.md$" "")
                      (str/replace #"/" " / ")))
        backlinks (db/get-backlinks-with-notes vault-id (:client-id note))
        backlinks-with-meta (mapv (fn [backlink]
                                    (assoc backlink
                                           :title (or (md/extract-title (:content backlink))
                                                      (str/replace (:path backlink) #"\.md$" ""))
                                           :description (md/extract-description (:content backlink))))
                                  backlinks)]
    {:note {:client-id (:client-id note)
            :title title
            :html-content html-content
            :path (:path note)
            :updated-at (:updated-at note)}
     :backlinks backlinks-with-meta}))

(defn get-note-fragment
  [request]
  (let [client-id (get-in request [:path-params :id])
        ]
    (with-vault request
      (fn [vault request]
        (let [vault-id (:id vault)]
          (if-let [note (db/get-note-for-app vault-id client-id)]
            (let [render-data (prepare-note-data note vault-id)

                  from-note-id (get-in request [:headers "x-from-note-id"])
                  current-url (get-in request [:headers "hx-current-url"])
                  root-note-id (:root-note-id vault)

                  push-url (build-push-url current-url from-note-id client-id root-note-id)

                  html-body (render-template "templates/app/note.html" render-data)]

              {:status 200
               :headers {"Content-Type" "text/html; charset=utf-8"
                         "HX-Push-Url" push-url}
               :body html-body})
            {:status 404 :body "Note not found"}))))))

(defn- extract-logo-hash
  "Extract content hash from logo-object-key.
   Format: site/logo/{hash}.{ext} -> returns {hash}"
  [logo-object-key]
  (when logo-object-key
    (let [filename (last (str/split logo-object-key #"/"))]
      (first (str/split filename #"\.")))))

(defn- public-vault
  [vault]
  (let [vault-id (:id vault)
        logo-object-key (:logo-object-key vault)
        logo-hash (extract-logo-hash logo-object-key)]
    (cond-> vault
      logo-object-key
      (assoc :logo-url (object-store/public-asset-url vault-id logo-object-key)
             :favicon-version (java.net.URLEncoder/encode logo-object-key "UTF-8"))

      logo-hash
      (assoc :logo-hash logo-hash))))

(defn get-note
  [request]
  (let [path (get-in request [:path-params :path] "/")
        path-client-ids (parse-path-ids path)
        is-htmx? (get-in request [:headers "hx-request"])]
    (with-vault request
      (fn [vault request]
        (let [vault-id (:id vault)
              vault (public-vault vault)]
          (if (empty? path-client-ids)
            (if-let [root-client-id (:root-note-id vault)]
              (if-let [root-note (db/get-note-for-app vault-id root-client-id)]
                (let [render-data (prepare-note-data root-note vault-id)
                      description (md/extract-description (:content root-note) 160)]
                  (if is-htmx?
                    {:status 200
                     :headers {"Content-Type" "text/html; charset=utf-8"}
                     :body (render-template "templates/app/note.html" render-data)}
                    (resp/html (render-template "templates/app/note-page.html"
                                                {:notes [render-data]
                                                 :vault vault
                                                 :description description}))))
                (let [notes (db/list-notes-by-vault vault-id)]
                  (resp/html (render-template "templates/app/home.html"
                                              {:vault vault
                                               :notes notes}))))
              (let [notes (db/list-notes-by-vault vault-id)]
                (resp/html (render-template "templates/app/home.html"
                                            {:vault vault
                                             :notes notes}))))

            (let [valid-notes (keep #(db/get-note-for-app vault-id %) path-client-ids)
                  valid-client-ids (mapv :client-id valid-notes)

                  needs-correction? (not= (count valid-client-ids) (count path-client-ids))
                  corrected-path (when needs-correction?
                                   (if (empty? valid-client-ids)
                                     "/"
                                     (str "/" (str/join "+" valid-client-ids))))]

              (cond
                (empty? valid-notes)
                {:status 404 :body "Note not found"}

                is-htmx?
                (let [last-note (last valid-notes)
                      render-data (prepare-note-data last-note vault-id)

                      from-note-id (get-in request [:headers "x-from-note-id"])
                      current-url (get-in request [:headers "hx-current-url"])
                      root-note-id (:root-note-id vault)
                      push-url (or corrected-path
                                   (build-push-url current-url from-note-id (:client-id last-note) root-note-id))]

                  {:status 200
                   :headers {"Content-Type" "text/html; charset=utf-8"
                             "HX-Push-Url" push-url}
                   :body (render-template "templates/app/note.html" render-data)})

                :else
                (let [notes-data (mapv #(prepare-note-data % vault-id) valid-notes)
                      first-note (first valid-notes)
                      description (md/extract-description (:content first-note) 160)
                      response-body (render-template "templates/app/note-page.html"
                                                     {:notes notes-data
                                                      :vault vault
                                                      :description description})]
                  (if needs-correction?
                    {:status 200
                     :headers {"Content-Type" "text/html; charset=utf-8"
                               "HX-Replace-Url" corrected-path}
                     :body response-body}
                    (resp/html response-body)))))))))))

;; ============================================================
;; Logo & Favicon Serving
;; ============================================================

(defn serve-favicon
  "Serve vault favicon (32x32) for public access.
   Route: GET /favicon.ico
   
   Redirects to a versioned favicon URL."
  [request]
  (with-vault request
    (fn [vault request]
      (cond
        (str/blank? (:logo-object-key vault))
        {:status 404
         :headers {"Content-Type" "text/plain"}
         :body "Not found"}

        :else
        (let [vault-id (:id vault)
              logo-key (:logo-object-key vault)
              current-hash (extract-logo-hash logo-key)
              etag (str "\"" current-hash "\"")
              version (query-param request "v")
              cache-control (if version
                              "public, max-age=31536000, immutable"
                              "public, max-age=300")
              favicon-key (object-store/favicon-object-key logo-key)
              asset-key (if (and favicon-key (object-store/object-exists? vault-id favicon-key))
                          favicon-key
                          logo-key)
              location (object-store/public-asset-url vault-id asset-key)]
          {:status 302
           :headers {"Location" location
                     "Cache-Control" cache-control
                     "ETag" etag}})))))

(defn serve-asset
  "Serve assets from local storage.
   Route: GET /storage/*path
   
   The path is the full object_key (e.g., 'assets/{client_id}').
   Vault isolation is enforced via Host header -> vault lookup."
  [request]
  (let [path (get-in request [:path-params :path])
        ]
    (with-vault request
      (fn [vault _request]
        (cond
          (= :s3 (config/storage-type))
          (do
            (log/warn "Asset request to /storage/* when using S3 storage")
            {:status 404
             :headers {"Content-Type" "text/plain"}
             :body "Not found"})

          (or (nil? path) (str/blank? path))
          {:status 400
           :headers {"Content-Type" "text/plain"}
           :body "Bad request"}

          :else
          (let [vault-id (:id vault)
                result (object-store/get-object vault-id path)]
            (if result
              (let [body (utils.stream/input-stream->bytes (:Body result))
                    content-type (or (:ContentType result) "application/octet-stream")]
                (log/debug "Serving asset:" path "for vault:" vault-id)
                {:status 200
                 :headers {"Content-Type" content-type
                           "Cache-Control" "public, max-age=31536000, immutable"}
                 :body body})
              (do
                (log/debug "Asset not found:" path "for vault:" vault-id)
                {:status 404
                 :headers {"Content-Type" "text/plain"}
                 :body "Not found"}))))))))
