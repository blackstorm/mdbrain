(ns mdbrain.handlers.console.auth
  "Console authentication and initialization handlers."
  (:require
   [clojure.string :as str]
   [mdbrain.db :as db]
   [mdbrain.response :as resp]
   [mdbrain.template-assets :as template-assets]
   [mdbrain.utils :as utils]
   [selmer.parser :as selmer]))

(defn- render-template
  [template context]
  (template-assets/register-filter!)
  (selmer/render-file template context))

(defn init-console
  "Initialize system with first console user."
  [request]
  (if (db/has-any-user?)
    {:status 403
     :body {:success false
            :error "System already initialized"}}
    (let [params (or (:body-params request) (:params request))
          {:keys [username password tenant-name]} params]
      (cond
        (or (nil? username) (nil? password) (nil? tenant-name))
        {:status 200
         :body {:success false
                :error "Missing required fields"}}

        (db/get-user-by-username username)
        {:status 200
         :body {:success false
                :error "Username already exists"}}

        :else
        (let [tenant-id (utils/generate-uuid)
              user-id (utils/generate-uuid)
              password-hash (utils/hash-password password)]
          (db/create-tenant! tenant-id tenant-name)
          (db/create-user! user-id tenant-id username password-hash)
          (resp/success {:tenant-id tenant-id
                         :user-id user-id}))))))

(defn login
  "Console login handler."
  [request]
  (let [params (or (:body-params request) (:params request))
        {:keys [username password]} params
        user (db/get-user-by-username username)]
    (if (and user (utils/verify-password password (:password-hash user)))
      (resp/success {:user {:id (:id user)
                            :username (:username user)
                            :tenant-id (:tenant-id user)}}
                    {:user-id (:id user)
                     :tenant-id (:tenant-id user)})
      {:status 200
       :body {:success false
              :error "Invalid username or password"}})))

(defn logout
  "Console logout handler."
  [request]
  {:status 302
   :session nil
   :headers {"Location" "/console/login"}})

(defn login-page
  "Render login page."
  [request]
  (resp/html (render-template "templates/console/login.html"
                              {:csrf-token (:anti-forgery-token request)})))

(defn init-page
  "Render initialization page."
  [request]
  (resp/html (render-template "templates/console/init.html"
                              {:csrf-token (:anti-forgery-token request)})))

(defn change-password
  "Change current console user's password."
  [request]
  (let [user-id (get-in request [:session :user-id])
        params (or (:body-params request) (:params request))
        {:keys [current-password new-password confirm-password]} params
        current (some-> current-password str)
        next (some-> new-password str)
        confirm (some-> confirm-password str)]
    (cond
      (or (str/blank? current) (str/blank? next) (str/blank? confirm))
      (resp/error 400 "Missing required fields")

      (not= next confirm)
      (resp/error 400 "New password confirmation does not match")

      (< (count next) 8)
      (resp/error 400 "New password must be at least 8 characters")

      :else
      (let [user (db/get-user-by-id user-id)]
        (cond
          (nil? user)
          (resp/error 404 "User not found")

          (not (utils/verify-password current (:password-hash user)))
          (resp/error 400 "Current password is incorrect")

          :else
          (let [password-hash (utils/hash-password next)]
            (db/update-user-password! user-id password-hash)
            (resp/success {:message "Password updated"})))))))
