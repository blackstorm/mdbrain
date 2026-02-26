(ns mdbrain.handlers.console.vaults
  "Vault CRUD and notes management handlers."
  (:require
   [clojure.string :as str]
   [mdbrain.db :as db]
   [mdbrain.handlers.console.common :as common]
   [mdbrain.object-store :as object-store]
   [mdbrain.response :as resp]
   [mdbrain.template-assets :as template-assets]
   [mdbrain.utils :as utils]
   [mdbrain.utils.bytes :as utils.bytes]
   [selmer.parser :as selmer]))

(defn- render-template
  [template context]
  (template-assets/register-filter!)
  (selmer/render-file template context))

(defn- enrich-vault-data
  "Add computed fields to vault for display."
  [vault]
  (let [sync-key (:sync-key vault)
        masked (str (subs sync-key 0 8) "******" (subs sync-key (- (count sync-key) 8)))
        publish-status (or (:last-publish-status vault) "never")
        notes (db/search-notes-by-vault (:id vault) "")
        storage-bytes (db/get-vault-storage-size (:id vault))
        logo-url (when-let [key (:logo-object-key vault)]
                   (common/console-asset-url (:id vault) key))]
    (assoc vault
           :masked-key masked
           :last-publish-status publish-status
           :publish-ok (= publish-status "ok")
           :publish-error (= publish-status "error")
           :publish-never (not (contains? #{"ok" "error"} publish-status))
           :notes notes
           :storage-size (utils.bytes/format-storage-size storage-bytes)
           :logo-url logo-url)))

(defn console-home
  "Console home page showing all vaults."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        tenant (db/get-tenant tenant-id)
        vaults (db/list-vaults-by-tenant tenant-id)
        vaults-with-data (mapv enrich-vault-data vaults)]
    (resp/html (render-template "templates/console/vaults.html"
                                {:tenant tenant
                                 :vaults vaults-with-data
                                 :csrf-token (:anti-forgery-token request)}))))

(defn list-vaults
  "List all vaults for current tenant."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vaults (db/list-vaults-by-tenant tenant-id)
        vaults-with-data (mapv enrich-vault-data vaults)]
    (resp/html (render-template "templates/console/vault-list.html"
                                {:vaults vaults-with-data}))))

(defn create-vault
  "Create a new vault."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        params (or (:body-params request) (:params request))
        {:keys [name domain]} params]
    (cond
      (or (nil? name) (nil? domain))
      {:status 200
       :body {:success false
              :error "Missing required fields"}}

      (db/get-vault-by-domain domain)
      {:status 200
       :body {:success false
              :error "Domain already in use"}}

      :else
      (let [vault-id (utils/generate-uuid)
            sync-key (utils/generate-uuid)]
        (db/create-vault! vault-id tenant-id name domain sync-key)
        (resp/success {:vault {:id vault-id
                               :name name
                               :domain domain
                               :sync-key sync-key}})))))

(defn delete-vault
  "Delete a vault and all its data."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        vault (db/get-vault-by-id vault-id)]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      :else
      (do
        (object-store/delete-vault-objects! vault-id)
        (db/delete-vault! vault-id)
        (resp/success {:message "Vault deleted"})))))

(defn update-vault
  "Update vault name and domain."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        params (or (:body-params request) (:params request))
        name (:name params)
        domain (:domain params)
        vault (db/get-vault-by-id vault-id)
        existing-domain (when (and domain (not (str/blank? domain)))
                          (db/get-vault-by-domain domain))]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      (or (nil? name) (str/blank? name))
      {:status 200
       :body {:success false
              :error "Vault name is required"}}

      (or (nil? domain) (str/blank? domain))
      {:status 200
       :body {:success false
              :error "Domain is required"}}

      (and existing-domain (not= (:id existing-domain) vault-id))
      {:status 200
       :body {:success false
              :error "Domain already in use"}}

      :else
      (try
        (db/update-vault! vault-id name domain)
        {:status 200
         :body {:success true
                :vault {:id vault-id
                        :name name
                        :domain domain}}}
        (catch Exception e
          (if (str/includes? (.getMessage e) "UNIQUE constraint failed: vaults.domain")
            {:status 200
             :body {:success false
                    :error "Domain already in use"}}
            (throw e)))))))

(defn search-vault-notes
  "Search notes within a vault."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        query (get-in request [:params :q])
        vault (db/get-vault-by-id vault-id)]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      (nil? query)
      {:status 200
       :body {:success false
              :error "Missing search query"}}

      :else
      (let [notes (db/search-notes-by-vault vault-id query)]
        (resp/success {:notes notes})))))

(defn update-vault-root-note
  "Set the root note for a vault."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        params (or (:body-params request) (:params request))
        root-note-id (:rootNoteId params)
        vault (db/get-vault-by-id vault-id)]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      (nil? root-note-id)
      {:status 200
       :body {:success false
              :error "Missing rootNoteId"}}

      :else
      (do
        (db/update-vault-root-note! vault-id root-note-id)
        (resp/success {:message "Root note updated"
                       :root-note-id root-note-id})))))

(defn get-root-note-selector
  "Render the root note selector component."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        vault (db/get-vault-by-id vault-id)]
    (cond
      (nil? vault)
      {:status 404
       :headers {"Content-Type" "text/html"}
       :body "<div class=\"alert alert-error\"><span>Vault not found</span></div>"}

      (not= (:tenant-id vault) tenant-id)
      {:status 403
       :headers {"Content-Type" "text/html"}
       :body "<div class=\"alert alert-error\"><span>Permission denied</span></div>"}

      :else
      (let [notes (db/search-notes-by-vault vault-id "")
            root-note-id (:root-note-id vault)]
        {:status 200
         :headers {"Content-Type" "text/html"}
         :body (render-template "templates/console/root-note-selector.html"
                                {:notes notes
                                 :vault-id vault-id
                                 :root-note-id root-note-id})}))))

(defn renew-vault-sync-key
  "Generate a new publish key for a vault."
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        vault (db/get-vault-by-id vault-id)
        new-sync-key (utils/generate-uuid)]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      :else
      (do
        (db/update-vault-sync-key! vault-id new-sync-key)
        {:status 200
         :body {:success true
                :message "Publish key renewed"
                :sync-key new-sync-key}}))))

(def ^:private max-custom-html-size 65536) ; 64KB

(defn update-custom-head-html
  [request]
  (let [tenant-id (get-in request [:session :tenant-id])
        vault-id (get-in request [:path-params :id])
        params (or (:body-params request) (:params request))
        custom-head-html (:customHeadHtml params)
        vault (db/get-vault-by-id vault-id)]
    (cond
      (nil? vault)
      {:status 200
       :body {:success false
              :error "Vault not found"}}

      (not= (:tenant-id vault) tenant-id)
      {:status 200
       :body {:success false
              :error "Permission denied"}}

      (and custom-head-html (> (count custom-head-html) max-custom-html-size))
      {:status 200
       :body {:success false
              :error (str "Custom HTML exceeds maximum size of 64KB")}}

      :else
      (do
        (db/update-vault-custom-head-html! vault-id custom-head-html)
        {:status 200
         :body {:success true
                :message "Custom HTML updated"}}))))
