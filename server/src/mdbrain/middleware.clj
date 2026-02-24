(ns mdbrain.middleware
  (:require
   [clojure.string :as str]
   [mdbrain.config :as config]
   [mdbrain.db :as db]
   [mdbrain.response :as resp]
   [ring.middleware.keyword-params :as keyword-params]
   [ring.middleware.multipart-params :as multipart]
   [ring.middleware.params :as params]
   [ring.middleware.session :as session]
   [ring.middleware.session.cookie :as cookie]
   [ring.util.response :as response]))

(def ^:private session-cookie-name
  "mdbrain-session")

(defn parse-host-domain
  "Parse an HTTP Host header into a domain string (without port).
   Returns {:domain <string>} or {:error :missing|:invalid}."
  [host]
  (let [host (when (string? host) (str/trim host))]
    (cond
      (str/blank? host)
      {:error :missing}

      (or (re-find #"\s" host)
          (str/includes? host "/"))
      {:error :invalid}

      ;; Bracketed IPv6 host, e.g. \"[::1]:8080\"
      (str/starts-with? host "[")
      (let [close-idx (str/index-of host "]")]
        (if (nil? close-idx)
          {:error :invalid}
          (let [domain (subs host 1 close-idx)
                rest (subs host (inc close-idx))]
            (cond
              (str/blank? domain) {:error :invalid}
              (str/blank? rest) {:domain domain}
              (not (str/starts-with? rest ":")) {:error :invalid}
              :else
              (let [port-str (subs rest 1)]
                (try
                  (let [port (Long/parseLong port-str)]
                    (if (<= 1 port 65535)
                      {:domain domain}
                      {:error :invalid}))
                  (catch Exception _
                    {:error :invalid})))))))

      :else
      (let [parts (str/split host #":" 3)]
        (if (> (count parts) 2)
          {:error :invalid}
          (let [domain (first parts)
                port-str (when (= (count parts) 2) (second parts))]
            (if (and (string? domain)
                     (not (str/blank? domain))
                     (re-matches #"[A-Za-z0-9._-]+" domain))
              (if (nil? port-str)
                {:domain domain}
                (try
                  (let [port (Long/parseLong port-str)]
                    (if (<= 1 port 65535)
                      {:domain domain}
                      {:error :invalid}))
                  (catch Exception _
                    {:error :invalid})))
              {:error :invalid})))))))

(defn wrap-session-middleware [handler]
  (session/wrap-session
    handler
    {:store (cookie/cookie-store {:key (config/session-secret)})
     :cookie-name session-cookie-name
     :cookie-attrs {:max-age (* 60 60 24 7)
                    :http-only true
                    :same-site :lax
                    :secure (config/production?)}}))

;; 认证中间件（检查管理员登录）
(defn wrap-auth [handler]
  (fn [request]
    (let [uri (:uri request)
          session (:session request)
          user-id (get-in request [:session :user-id])]
      (if (and user-id (not (str/blank? user-id)))
        (do
          (handler request))
        ;; 未登录，重定向到登录页
        (do
          {:status 302
           :headers {"Location" "/console/login"}})))))

;; CORS 中间件
(defn wrap-cors [handler]
  (fn [request]
    (let [uri (:uri request)
          cors-enabled? (str/starts-with? uri "/obsidian/")]
      (if-not cors-enabled?
        (handler request)
        (let [cors-headers {"Access-Control-Allow-Origin" "*"
                            "Access-Control-Allow-Methods" "GET, POST, PUT, DELETE, OPTIONS"
                            "Access-Control-Allow-Headers" "Content-Type, Authorization"}
              response (if (= :options (:request-method request))
                         {:status 200 :body ""}
                         (handler request))]
        (update response :headers (fnil merge {}) cors-headers))))))

(defn wrap-noindex
  "Add noindex headers for Console responses."
  [handler]
  (fn [request]
    (let [response (handler request)]
      (if (map? response)
        (update response :headers (fnil assoc {}) "X-Robots-Tag" "noindex, nofollow")
        response))))

(defn wrap-app-host-binding
  "Enforce Host -> vault binding for the App server (8080).
   - Missing/invalid Host: 400
   - Host not bound to a vault: 403

   Skips reserved paths (\"/console*\", \"/obsidian*\") so the app can return a
   consistent 404 for those paths."
  [handler]
  (fn [request]
    (let [uri (:uri request)]
      (if (or (str/starts-with? uri "/console")
              (str/starts-with? uri "/obsidian"))
        (handler request)
        (let [{:keys [domain error]} (parse-host-domain (get-in request [:headers "host"]))]
          (cond
            (= error :missing)
            {:status 400
             :headers {"Content-Type" "text/plain; charset=utf-8"
                       "Cache-Control" "no-store"}
             :body "Bad request"}

            (= error :invalid)
            {:status 400
             :headers {"Content-Type" "text/plain; charset=utf-8"
                       "Cache-Control" "no-store"}
             :body "Bad request"}

            :else
            (if-let [vault (db/get-vault-by-domain domain)]
              (handler (assoc request
                              :mdbrain/domain domain
                              :mdbrain/vault vault))
              {:status 403
               :headers {"Content-Type" "text/plain; charset=utf-8"
                         "Cache-Control" "no-store"}
               :body "Forbidden"})))))))

;; CSRF 中间件（仅保护 Console 路由）
(defn- get-csrf-token-from-request [request]
  (let [headers (:headers request)
        header-token (get headers "x-csrf-token")
        params (:params request)]
    (or header-token
        (get params "__anti-forgery-token")
        (get params :__anti-forgery-token))))

(defn wrap-console-csrf [handler]
  (fn [request]
    (if-not (str/starts-with? (:uri request) "/console")
      (handler request)
      (let [session (or (:session request) {})
            csrf-token (or (:csrf-token session) (config/generate-random-hex 32))
            request' (-> request
                         (assoc :anti-forgery-token csrf-token)
                         (assoc :session (assoc session :csrf-token csrf-token)))
            state-changing? (contains? #{:post :put :delete :patch} (:request-method request'))
            provided-token (when state-changing? (get-csrf-token-from-request request'))]
        (if (and state-changing? (not= provided-token csrf-token))
          (resp/json-error 403 "CSRF token missing or incorrect")
          (let [response (handler request')
                response-session (:session response)]
            (cond
              ;; Handler explicitly set :session (e.g. login/logout)
              (contains? response :session)
              (cond
                (nil? response-session) response
                (map? response-session) (assoc response :session (assoc response-session :csrf-token csrf-token))
                :else response)

              ;; Handler didn't set :session; persist CSRF token
              :else
              (assoc response :session (:session request')))))))))

;; 初始化检查中间件（检查是否有用户，没有则跳转到初始化页面）
(defn wrap-init-check [handler]
  (fn [request]
    (let [uri (:uri request)
          method (:request-method request)
          has-user (db/has-any-user?)]
      ;; Only gate Console routes. Frontend (8080) must never redirect into /console/*.
      (if-not (str/starts-with? uri "/console")
        (handler request)
      (cond
        ;; 跳过 Obsidian 同步接口、API 路由、静态资源和 favicon
        (or (str/starts-with? uri "/obsidian/")
            (str/starts-with? uri "/api/")
            (str/starts-with? uri "/static/")
            (str/starts-with? uri "/js/")
            (str/starts-with? uri "/css/")
            (= uri "/favicon.ico")
            ;; Console internal endpoints should keep working even before init.
            (= uri "/console/health")
            (= uri "/console/domain-check"))
        (handler request)

        ;; 如果已有用户，禁止访问初始化页面
        (and has-user (= uri "/console/init"))
        (response/redirect "/console/login")

        ;; 如果没有用户，只允许访问初始化页面
        (and (not has-user) (not= uri "/console/init"))
        (response/redirect "/console/init")

        ;; 其他情况正常处理
        :else
        (handler request))))))

;; 完整中间件栈
(defn wrap-middleware [handler]
  (-> handler
      wrap-console-csrf
      wrap-session-middleware
      keyword-params/wrap-keyword-params
      multipart/wrap-multipart-params
      params/wrap-params
      wrap-init-check
      ;; 移除 json/wrap-json-body 和 json/wrap-json-response
      ;; 因为 Reitit 的 muuntaja 中间件会处理 JSON
      wrap-cors))
