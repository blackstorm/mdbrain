(ns mdbrain.db
  (:require [honey.sql :as hsql]
            [honey.sql.helpers :as h]
            [next.jdbc :as jdbc]
            [next.jdbc.sql :as sql]
            [next.jdbc.result-set :as rs]
            [mdbrain.config :as config]
            [mdbrain.utils :as utils]
            [migratus.core :as migratus]
            [clojure.java.io :as io]
            [clojure.string :as str]
            [clojure.tools.logging :as log]))

(def datasource
  (delay
    (jdbc/get-datasource (config/get-config :database))))

(defn underscore->kebab
  [k]
  (keyword (str/replace (name k) "_" "-")))

(defn map-keys
  [f m]
  (when m
    (into {} (map (fn [[k v]] [(f k) v]) m))))

(defn db-keys->clojure
  [m]
  (map-keys underscore->kebab m))

(defn db-keys-coll->clojure
  [coll]
  (map db-keys->clojure coll))

(defn- to-sql-vec [query]
  (cond
    (map? query) (hsql/format query)
    (vector? query) query
    (string? query) [query]
    :else query))

(defn execute! [query]
  (db-keys-coll->clojure
    (jdbc/execute! @datasource (to-sql-vec query) {:builder-fn rs/as-unqualified-lower-maps})))

(defn execute-one! [query]
  (db-keys->clojure
    (jdbc/execute-one! @datasource (to-sql-vec query) {:builder-fn rs/as-unqualified-lower-maps})))

(defn insert-with-builder!
  [table data]
  (db-keys->clojure
    (sql/insert! @datasource table data {:builder-fn rs/as-unqualified-lower-maps})))

(defn find-by
  [table column value]
  (execute-one!
    (-> (h/select :*)
        (h/from table)
        (h/where [:= column value]))))

(defn find-all-by
  [table column value & {:keys [order-by]}]
  (execute!
    (cond-> (-> (h/select :*)
                (h/from table)
                (h/where [:= column value]))
      order-by (h/order-by order-by))))

(defn- extract-db-path-from-jdbc-url
  "Extract database file path from JDBC URL.
   e.g., jdbc:sqlite:data/mdbrain.db?journal_mode=WAL -> data/mdbrain.db"
  [jdbc-url]
  (when jdbc-url
    (-> jdbc-url
        (str/replace #"^jdbc:sqlite:" "")
        (str/replace #"\?.*$" ""))))

(defn- ensure-db-directory!
  "Ensure the parent directory of the database file exists."
  []
  (let [jdbc-url (config/get-config :database :jdbcUrl)
        db-path (extract-db-path-from-jdbc-url jdbc-url)
        db-file (io/file db-path)
        parent-dir (.getParentFile db-file)]
    (when (and parent-dir (not (.exists parent-dir)))
      (log/info "Creating database directory:" (.getPath parent-dir))
      (.mkdirs parent-dir))))

(defn migratus-config
  "Generate migratus config dynamically to support test fixtures with with-redefs."
  []
  {:store :database
   :migration-dir "migrations"
   :init-script nil
   :db {:datasource @datasource}})

(defn init-db!
  "Initialize database by running all pending migrations."
  []
  (ensure-db-directory!)
  (log/info "Running database migrations with migratus...")
  (migratus/migrate (migratus-config))
  (log/info "Database migrations complete."))

;; Tenant 操作
(defn create-tenant! [id name]
  (insert-with-builder! :tenants {:id id :name name}))

(defn get-tenant [id]
  (find-by :tenants :id id))

;; User 操作
(defn create-user! [id tenant-id username password-hash]
  (insert-with-builder! :users
                       {:id id
                        :tenant_id tenant-id
                        :username username
                        :password_hash password-hash}))

(defn get-user-by-username [username]
  (find-by :users :username username))

(defn get-user-by-id [id]
  (find-by :users :id id))

(defn update-user-password!
  "Update a user's password hash."
  [user-id password-hash]
  (execute-one!
    (-> (h/update :users)
        (h/set {:password_hash password-hash})
        (h/where [:= :id user-id]))))

(def ^:private has-user-cache (atom nil))

(defn has-any-user? []
  (if-let [cached @has-user-cache]
    cached
    (let [result (> (:count (execute-one!
                              (-> (h/select [(hsql/call :count :*) :count])
                                  (h/from :users))))
                    0)]
      (when result (reset! has-user-cache true))
      result)))

;; Vault 操作
(defn create-vault! [id tenant-id name domain sync-key]
  (insert-with-builder! :vaults
                       {:id id
                        :tenant_id tenant-id
                        :name name
                        :domain domain
                        :sync_key sync-key}))

(defn get-vault-by-id [id]
  (find-by :vaults :id id))

(defn get-vault-by-domain [domain]
  (find-by :vaults :domain domain))

(defn get-vault-by-sync-key [sync-key]
  (find-by :vaults :sync_key sync-key))

(defn list-vaults-by-tenant [tenant-id]
  (find-all-by :vaults :tenant_id tenant-id :order-by :created_at))

(defn delete-vault! [id]
  (execute-one!
    (-> (h/delete-from :vaults)
        (h/where [:= :id id]))))

(defn update-vault-root-note! [vault-id root-note-id]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:root_note_id root-note-id})
        (h/where [:= :id vault-id]))))

(defn update-vault! [vault-id name domain]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:name name
                :domain domain})
        (h/where [:= :id vault-id]))))

(defn update-vault-sync-key! [vault-id new-sync-key]
  "Update the sync key for a vault."
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:sync_key new-sync-key})
        (h/where [:= :id vault-id]))))

(defn record-vault-publish-success!
  "Record a successful publish attempt for a vault."
  [vault-id]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:last_publish_status "ok"
                :last_publish_at [:raw "CURRENT_TIMESTAMP"]
                :last_publish_error_code nil
                :last_publish_error_message nil})
        (h/where [:= :id vault-id]))))

(defn record-vault-publish-error!
  "Record a failed publish attempt for a vault."
  [vault-id error-code error-message]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:last_publish_status "error"
                :last_publish_at [:raw "CURRENT_TIMESTAMP"]
                :last_publish_error_code error-code
                :last_publish_error_message error-message})
        (h/where [:= :id vault-id]))))

;; Note 操作
(defn upsert-note! [id tenant-id vault-id path client-id content metadata hash mtime]
  (execute-one!
    (-> (h/insert-into :notes)
        (h/columns :id :tenant_id :vault_id :path :client_id :content :metadata :hash :mtime)
        (h/values [[id tenant-id vault-id path client-id content metadata hash mtime]])
        (h/on-conflict :vault_id :client_id)
        (h/do-update-set {:path :excluded.path
                          :content :excluded.content
                          :metadata :excluded.metadata
                          :hash :excluded.hash
                          :mtime :excluded.mtime
                          :updated_at [:raw "CURRENT_TIMESTAMP"]}))))

(defn delete-note-by-client-id! [vault-id client-id]
  (execute-one!
    (-> (h/delete-from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]]))))

(defn update-note-path! [vault-id client-id new-path]
  (execute-one!
    (-> (h/update :notes)
        (h/set {:path new-path
                :updated_at [:raw "CURRENT_TIMESTAMP"]})
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]]))))

(defn delete-note! [vault-id path]
  (execute-one!
    (-> (h/delete-from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :path path]]))))

(defn get-note [id]
  (find-by :notes :id id))

(defn list-notes-by-vault [vault-id]
  (execute!
    (-> (h/select :client_id :path :hash :mtime)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:is :deleted_at nil]])
        (h/order-by :path))))

(defn get-notes-for-link-resolution
  [vault-id]
  (execute!
    (-> (h/select :client_id :path)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:is :deleted_at nil]]))))

(defn list-notes-with-wikilinks-by-vault
  [vault-id]
  (execute!
    (-> (h/select :client_id :path :content)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:is :deleted_at nil]
                  [:like :content "%[[%]]"]]))))

(defn get-note-by-path [vault-id path]
  (execute-one!
    (-> (h/select :*)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :path path]
                  [:is :deleted_at nil]]))))

(defn get-note-by-client-id [vault-id client-id]
  (execute-one!
    (-> (h/select :*)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]
                  [:is :deleted_at nil]]))))

(defn delete-note-by-client-id! [vault-id client-id]
  (execute-one!
    (-> (h/delete-from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]]))))

(defn get-note-for-app
  [vault-id client-id]
  (execute-one!
    (-> (h/select :*)
        (h/from :notes)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]
                  [:is :deleted_at nil]]))))

(defn search-notes-by-vault [vault-id query]
  (let [pattern (str "%" query "%")]
    (execute!
      (-> (h/select :id :client_id :path :content :metadata :mtime)
          (h/from :notes)
          (h/where [:and [:= :vault_id vault-id]
                    [:is :deleted_at nil]
                    [:or [:like :path pattern]
                     [:like :content pattern]]])
          (h/order-by :path)
          (h/limit 50)))))

;; Note Links 操作
(defn delete-note-links-by-source! [vault-id source-client-id]
  (log/debug "Deleting all links from source:" source-client-id)
  (let [result (execute-one!
                 (-> (h/delete-from :note_links)
                     (h/where [:and [:= :vault_id vault-id]
                               [:= :source_client_id source-client-id]])))]
    (log/debug "Delete result:" result)
    result))

(defn delete-note-link-by-target! [vault-id source-client-id target-client-id]
  (log/debug "Deleting specific link - source:" source-client-id "target:" target-client-id)
  (let [result (execute-one!
                 (-> (h/delete-from :note_links)
                     (h/where [:and [:= :vault_id vault-id]
                               [:= :source_client_id source-client-id]
                               [:= :target_client_id target-client-id]])))]
    (log/debug "Delete result:" result)
    result))

(defn insert-note-link! [vault-id source-client-id target-client-id target-path link-type display-text original]
  (log/debug "Inserting note link:")
  (log/debug "  vault-id:" vault-id)
  (log/debug "  source-client-id:" source-client-id)
  (log/debug "  target-client-id:" target-client-id)
  (log/debug "  target-path:" target-path)
  (log/debug "  link-type:" link-type)
  (log/debug "  display-text:" display-text)
  (log/debug "  original:" original)
  (let [id (utils/generate-uuid)]
    (log/debug "Generated link id:" id)
    (let [data {:id id
                :vault_id vault-id
                :source_client_id source-client-id
                :target_client_id target-client-id
                :target_path target-path
                :link_type link-type
                :display_text display-text
                :original original}]
      (log/debug "Insert data:" data)
      (try
        (let [result (insert-with-builder! :note_links data)]
          (log/debug "Insert result:" result)
          result)
        (catch Exception e
          (log/error "Failed to insert note link:" (.getMessage e))
          (log/error "SQL Exception details:" e)
          (throw e))))))

(defn get-note-links [vault-id client-id]
  (execute!
    (-> (h/select :*)
        (h/from :note_links)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :source_client_id client-id]]))))

(defn get-backlinks-with-notes [vault-id client-id]
  (execute!
    (-> (h/select :n.*
                 [:nl.display_text :link_display_text]
                 :nl.link_type)
        (h/from [:note_links :nl])
        (h/join [:notes :n]
                [:and [:= :n.vault_id :nl.vault_id]
                 [:= :n.client_id :nl.source_client_id]])
        (h/where [:and [:= :nl.vault_id vault-id]
                  [:= :nl.target_client_id client-id]])
        (h/order-by :n.path))))

(defn delete-orphan-links!
  [vault-id]
  (log/info "Deleting orphan links - vault-id:" vault-id)
  (let [subquery (-> (h/select 1)
                     (h/from :notes)
                     (h/where [:and [:= :notes.vault_id :note_links.vault_id]
                               [:= :notes.client_id :note_links.target_client_id]]))]
    (execute-one!
      (-> (h/delete-from :note_links)
          (h/where [:and [:= :vault_id vault-id]
                    [:not [:exists subquery]]])))))

;; Vault Logo 操作
(defn update-vault-logo! [vault-id logo-object-key]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:logo_object_key logo-object-key})
        (h/where [:= :id vault-id]))))

(defn update-vault-custom-head-html! [vault-id custom-head-html]
  (execute-one!
    (-> (h/update :vaults)
        (h/set {:custom_head_html custom-head-html})
        (h/where [:= :id vault-id]))))

;; Asset 操作
(defn upsert-asset!
  [id tenant-id vault-id client-id path object-key size-bytes content-type md5]
  (execute-one!
    (-> (h/insert-into :assets)
        (h/columns :id :tenant_id :vault_id :client_id :path :object_key :size_bytes :content_type :md5 :deleted_at)
        (h/values [[id tenant-id vault-id client-id path object-key size-bytes content-type md5 nil]])
        (h/on-conflict :vault_id :client_id)
        (h/do-update-set {:path :excluded.path
                          :size_bytes :excluded.size_bytes
                          :content_type :excluded.content_type
                          :md5 :excluded.md5
                          :deleted_at nil
                          :updated_at [:raw "CURRENT_TIMESTAMP"]}))))

(defn get-asset-by-client-id [vault-id client-id]
  (execute-one!
    (-> (h/select :*)
        (h/from :assets)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]
                  [:is :deleted_at nil]]))))

(defn delete-asset-by-client-id! [vault-id client-id]
  (execute-one!
    (-> (h/delete-from :assets)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :client_id client-id]]))))

(defn get-asset-by-path [vault-id path]
  (execute-one!
    (-> (h/select :*)
        (h/from :assets)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :path path]
                  [:is :deleted_at nil]]))))

(defn get-asset-by-filename [vault-id filename]
  (execute-one!
    (-> (h/select :*)
        (h/from :assets)
        (h/where [:and [:= :vault_id vault-id]
                  [:like :path (str "%" filename)]
                  [:is :deleted_at nil]])
        (h/limit 1))))

(defn find-asset
  [vault-id path]
  (or (get-asset-by-path vault-id path)
      (get-asset-by-filename vault-id (str "/" path))))

(defn list-assets-by-vault [vault-id]
  (execute!
    (-> (h/select :client_id :path :md5 :size_bytes)
        (h/from :assets)
        (h/where [:and [:= :vault_id vault-id]
                  [:is :deleted_at nil]])
        (h/order-by :path))))


(defn get-vault-storage-size
  [vault-id]
  (let [result (execute-one!
                 (-> (h/select [(hsql/call :coalesce
                                           (hsql/call :sum :size_bytes)
                                           0) :total_bytes])
                     (h/from :assets)
                     (h/where [:and [:= :vault_id vault-id]
                               [:is :deleted_at nil]])))]
    (:total-bytes result)))

;; Note Asset Refs 操作
(defn upsert-note-asset-ref! [vault-id note-client-id asset-client-id]
  (let [id (utils/generate-uuid)]
    (execute-one!
      (-> (h/insert-into :note_asset_refs)
          (h/columns :id :vault_id :note_client_id :asset_client_id)
          (h/values [[id vault-id note-client-id asset-client-id]])
          (h/on-conflict :vault_id :note_client_id :asset_client_id)
          (h/do-nothing)))))

(defn delete-note-asset-refs-by-note! [vault-id note-client-id]
  (execute-one!
    (-> (h/delete-from :note_asset_refs)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :note_client_id note-client-id]]))))

(defn delete-note-asset-refs-by-asset! [vault-id asset-client-id]
  (execute-one!
    (-> (h/delete-from :note_asset_refs)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :asset_client_id asset-client-id]]))))

(defn delete-note-asset-ref! [vault-id note-client-id asset-client-id]
  (execute-one!
    (-> (h/delete-from :note_asset_refs)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :note_client_id note-client-id]
                  [:= :asset_client_id asset-client-id]]))))

(defn get-asset-refs-by-note [vault-id note-client-id]
  (execute!
    (-> (h/select :*)
        (h/from :note_asset_refs)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :note_client_id note-client-id]]))))

(defn get-asset-refs-by-asset [vault-id asset-client-id]
  (execute!
    (-> (h/select :*)
        (h/from :note_asset_refs)
        (h/where [:and [:= :vault_id vault-id]
                  [:= :asset_client_id asset-client-id]]))))

(defn count-asset-refs [vault-id asset-client-id]
  (let [result (execute-one!
                 (-> (h/select [(hsql/call :count :*) :count])
                     (h/from :note_asset_refs)
                     (h/where [:and [:= :vault_id vault-id]
                               [:= :asset_client_id asset-client-id]])))]
    (:count result)))

(defn update-note-asset-refs!
  [vault-id note-client-id new-asset-client-ids]
  (delete-note-asset-refs-by-note! vault-id note-client-id)
  (doseq [asset-client-id new-asset-client-ids]
    (upsert-note-asset-ref! vault-id note-client-id asset-client-id)))

 
