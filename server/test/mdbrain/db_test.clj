(ns mdbrain.db-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [mdbrain.utils :as utils]
            [mdbrain.db :as db]
            [next.jdbc :as jdbc]
            [next.jdbc.sql :as sql]
            [next.jdbc.result-set :as rs]
            [clojure.string :as str]))

;; 测试数据库（内存 SQLite）
;; For :memory: SQLite, we need to keep the same connection alive
(def ^:dynamic *conn* nil)

(defn create-tables! [conn]
  ;; Enable foreign key enforcement for CASCADE deletes
  (jdbc/execute! conn ["PRAGMA foreign_keys = ON"])
  (jdbc/execute! conn
                 ["CREATE TABLE tenants (
                     id TEXT PRIMARY KEY,
                     name TEXT NOT NULL,
                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"])
  (jdbc/execute! conn
                 ["CREATE TABLE users (
                     id TEXT PRIMARY KEY,
                     tenant_id TEXT NOT NULL,
                     username TEXT UNIQUE NOT NULL,
                     password_hash TEXT NOT NULL,
                     role TEXT DEFAULT 'admin',
                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                     FOREIGN KEY (tenant_id) REFERENCES tenants(id))"])
  (jdbc/execute! conn
                 ["CREATE TABLE vaults (
                     id TEXT PRIMARY KEY,
                     tenant_id TEXT NOT NULL,
                     name TEXT NOT NULL,
                     domain TEXT UNIQUE,
                     sync_key TEXT UNIQUE NOT NULL,
                     logo_object_key TEXT,
                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                     FOREIGN KEY (tenant_id) REFERENCES tenants(id))"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_vaults_sync_key ON vaults(sync_key)"])
  (jdbc/execute! conn
                 ["CREATE TABLE notes (
                     id TEXT PRIMARY KEY,
                     tenant_id TEXT NOT NULL,
                     vault_id TEXT NOT NULL,
                     path TEXT NOT NULL,
                     client_id TEXT NOT NULL,
                     content TEXT,
                     metadata TEXT,
                     hash TEXT,
                     mtime TIMESTAMP,
                     deleted_at INTEGER,
                     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                     UNIQUE(vault_id, client_id),
                     FOREIGN KEY (tenant_id) REFERENCES tenants(id),
                     FOREIGN KEY (vault_id) REFERENCES vaults(id) ON DELETE CASCADE)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_notes_vault ON notes(vault_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_notes_client_id ON notes(vault_id, client_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_notes_mtime ON notes(mtime)"])
  (jdbc/execute! conn
                 ["CREATE TABLE note_links (
                     id TEXT PRIMARY KEY,
                     vault_id TEXT NOT NULL,
                     source_client_id TEXT NOT NULL,
                     target_client_id TEXT NOT NULL,
                     target_path TEXT,
                     link_type TEXT NOT NULL,
                     display_text TEXT,
                     original TEXT,
                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                     FOREIGN KEY (vault_id) REFERENCES vaults(id) ON DELETE CASCADE)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_links_source ON note_links(vault_id, source_client_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_links_target ON note_links(vault_id, target_client_id)"])
  (jdbc/execute! conn
                 ["CREATE TABLE assets (
                     id TEXT PRIMARY KEY,
                     tenant_id TEXT NOT NULL,
                     vault_id TEXT NOT NULL,
                     client_id TEXT,
                     path TEXT NOT NULL,
                     object_key TEXT NOT NULL,
                     size_bytes INTEGER NOT NULL,
                     content_type TEXT NOT NULL,
                     md5 TEXT NOT NULL,
                     original_name TEXT,
                     deleted_at INTEGER,
                     created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
                     updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
                     UNIQUE(vault_id, client_id),
                     FOREIGN KEY (tenant_id) REFERENCES tenants(id),
                     FOREIGN KEY (vault_id) REFERENCES vaults(id) ON DELETE CASCADE)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_assets_vault ON assets(vault_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_assets_path ON assets(vault_id, path)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_assets_client_id ON assets(vault_id, client_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_assets_hash ON assets(vault_id, md5)"])
  (jdbc/execute! conn
                 ["CREATE TABLE note_asset_refs (
                     id TEXT PRIMARY KEY,
                     vault_id TEXT NOT NULL,
                     note_client_id TEXT NOT NULL,
                     asset_client_id TEXT NOT NULL,
                     created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
                     UNIQUE(vault_id, note_client_id, asset_client_id),
                     FOREIGN KEY (vault_id) REFERENCES vaults(id) ON DELETE CASCADE)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_asset_refs_note ON note_asset_refs(vault_id, note_client_id)"])
  (jdbc/execute! conn
                 ["CREATE INDEX idx_asset_refs_asset ON note_asset_refs(vault_id, asset_client_id)"]))

(defn setup-test-db [f]
  (let [db (jdbc/get-datasource {:dbtype "sqlite" :dbname ":memory:"})
        conn (jdbc/get-connection db)]
    (create-tables! conn)
    (binding [*conn* conn]
      (f))))

(use-fixtures :each setup-test-db)

;; 键名转换辅助函数
(defn underscore->kebab
  "将下划线转换为连字符 (snake_case -> kebab-case)"
  [k]
  (keyword (str/replace (name k) "_" "-")))

(defn map-keys
  "对map的所有键应用函数f"
  [f m]
  (when m
    (into {} (map (fn [[k v]] [(f k) v]) m))))

(defn db-keys->clojure
  "将数据库键名 (snake_case) 转换为 Clojure 键名 (kebab-case)"
  [m]
  (map-keys underscore->kebab m))

;; 辅助函数
(defn execute-one! [sql-vec]
  (db-keys->clojure
    (jdbc/execute-one! *conn* sql-vec {:builder-fn rs/as-unqualified-lower-maps})))

(defn get-tenant [id]
  (execute-one! ["SELECT * FROM tenants WHERE id = ?" id]))

(defn create-tenant! [id name]
  (sql/insert! *conn* :tenants {:id id :name name} {:builder-fn rs/as-unqualified-lower-maps})
  ;; Return the created tenant
  (get-tenant id))

(defn get-user-by-username [username]
  (execute-one! ["SELECT * FROM users WHERE username = ?" username]))

(defn get-user-by-id [id]
  (execute-one! ["SELECT * FROM users WHERE id = ?" id]))

(defn create-user! [id tenant-id username password-hash]
  (sql/insert! *conn* :users
               {:id id :tenant_id tenant-id :username username :password_hash password-hash}
               {:builder-fn rs/as-unqualified-lower-maps})
  ;; Return the created user
  (get-user-by-id id))

(defn get-vault-by-id [id]
  (execute-one! ["SELECT * FROM vaults WHERE id = ?" id]))

(defn get-vault-by-domain [domain]
  (execute-one! ["SELECT * FROM vaults WHERE domain = ?" domain]))

(defn get-vault-by-sync-key [key]
  (execute-one! ["SELECT * FROM vaults WHERE sync_key = ?" key]))

(defn list-vaults-by-tenant [tenant-id]
  (jdbc/execute! *conn* ["SELECT * FROM vaults WHERE tenant_id = ?" tenant-id]
                 {:builder-fn rs/as-unqualified-lower-maps}))

(defn create-vault! [id tenant-id name domain sync-key]
  (sql/insert! *conn* :vaults
               {:id id :tenant_id tenant-id :name name :domain domain
                :sync_key sync-key}
               {:builder-fn rs/as-unqualified-lower-maps})
  ;; Return the created vault
  (get-vault-by-id id))

(defn get-note [id]
  (execute-one! ["SELECT * FROM notes WHERE id = ?" id]))

(defn get-note-by-path [vault-id path]
  (execute-one! ["SELECT * FROM notes WHERE vault_id = ? AND path = ?" vault-id path]))

(defn upsert-note! [id tenant-id vault-id path client-id content metadata hash mtime]
  (execute-one!
   ["INSERT INTO notes (id, tenant_id, vault_id, path, client_id, content, metadata, hash, mtime)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(vault_id, client_id) DO UPDATE SET
       path = excluded.path,
       content = excluded.content,
       metadata = excluded.metadata,
       hash = excluded.hash,
       mtime = excluded.mtime,
       updated_at = CURRENT_TIMESTAMP"
    id tenant-id vault-id path client-id content metadata hash mtime])
  ;; Return the note
  (get-note-by-path vault-id path))

(defn delete-note! [vault-id path]
  (execute-one! ["DELETE FROM notes WHERE vault_id = ? AND path = ?" vault-id path]))

(defn list-notes-by-vault [vault-id]
  (jdbc/execute! *conn* ["SELECT * FROM notes WHERE vault_id = ?" vault-id]
                 {:builder-fn rs/as-unqualified-lower-maps}))

;; Tenant 测试
(deftest test-create-tenant
  (testing "Create tenant successfully"
    (let [tenant-id (utils/generate-uuid)
          tenant-name "Test Org"
          result (create-tenant! tenant-id tenant-name)]
      (is (map? result))
      (is (= tenant-id (:id result)))
      (is (= tenant-name (:name result))))))

;; User 测试
(deftest test-create-user
  (testing "Create user successfully"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          username "console"
          password-hash "hashed-password"
          result (create-user! user-id tenant-id username password-hash)]
      (is (map? result))
      (is (= user-id (:id result)))
      (is (= username (:username result)))))

  (testing "Get user by username"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          username "testuser"
          _ (create-user! user-id tenant-id username "hash")
          result (get-user-by-username username)]
      (is (map? result))
      (is (= username (:username result)))))

  (testing "Get user by ID"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (create-user! user-id tenant-id "user" "hash")
          result (get-user-by-id user-id)]
      (is (map? result))
      (is (= user-id (:id result))))))

;; Vault 测试
(deftest test-create-vault
  (testing "Create vault successfully"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          vault-name "My Blog"
          domain "blog.test.com"
          sync-key (utils/generate-uuid)
          result (create-vault! vault-id tenant-id vault-name domain sync-key)]
      (is (map? result))
      (is (= vault-id (:id result)))
      (is (= vault-name (:name result)))
      (is (= domain (:domain result)))))

  (testing "Get vault by ID"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-id.com" sync-key)
          result (get-vault-by-id vault-id)]
      (is (map? result))
      (is (= vault-id (:id result)))))

  (testing "Get vault by domain"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "unique.test.com"
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" domain sync-key)
          result (get-vault-by-domain domain)]
      (is (map? result))
      (is (= domain (:domain result)))))

  (testing "Get vault by sync key"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-sync.com" sync-key)
          result (get-vault-by-sync-key sync-key)]
      (is (map? result))
      (is (= sync-key (:sync-key result)))))

  (testing "List vaults by tenant"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id-1 (utils/generate-uuid)
          vault-id-2 (utils/generate-uuid)
          sync-key-1 (utils/generate-uuid)
          sync-key-2 (utils/generate-uuid)
          _ (create-vault! vault-id-1 tenant-id "Blog 1" "blog1.com" sync-key-1)
          _ (create-vault! vault-id-2 tenant-id "Blog 2" "blog2.com" sync-key-2)
          results (list-vaults-by-tenant tenant-id)]
      (is (= 2 (count results)))
      (is (every? map? results)))))

;; Note 测试
(deftest test-upsert-note
  (testing "Insert new note"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-insert.com" sync-key)
          note-id (utils/generate-uuid)
          path "test.md"
          client-id "test-client-1"
          content "# Test"
          metadata "{\"tags\":[]}"
          hash "abc123"
          mtime "2025-12-21T10:00:00Z"
          result (upsert-note! note-id tenant-id vault-id path client-id content metadata hash mtime)]
      (is (map? result))
      (is (= path (:path result)))))

  (testing "Update existing note"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-update.com" sync-key)
          note-id (utils/generate-uuid)
          client-id "test-client-update"
          path "test.md"
          _ (upsert-note! note-id tenant-id vault-id path client-id "# Old" "{}" "old" "2025-12-21T10:00:00Z")
          new-content "# New"
          result (upsert-note! (utils/generate-uuid) tenant-id vault-id path client-id new-content "{}" "new" "2025-12-21T11:00:00Z")
          fetched (get-note-by-path vault-id path)]
      (is (= new-content (:content fetched))))))

(deftest test-delete-note
  (testing "Delete note"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-delete.com" sync-key)
          note-id (utils/generate-uuid)
          client-id "test-client-to-delete"
          path "to-delete.md"
          _ (upsert-note! note-id tenant-id vault-id path client-id "# Delete Me" "{}" "hash" "2025-12-21T10:00:00Z")
          _ (delete-note! vault-id path)
          result (get-note-by-path vault-id path)]
      (is (nil? result)))))

(deftest test-list-notes
  (testing "List notes by vault"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-list.com" sync-key)
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note1.md" "client-1" "# Note 1" "{}" "hash1" "2025-12-21T10:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note2.md" "client-2" "# Note 2" "{}" "hash2" "2025-12-21T11:00:00Z")
          results (list-notes-by-vault vault-id)]
      (is (= 2 (count results)))
      (is (every? #(contains? % :path) results)))))

(deftest test-list-notes-with-wikilinks-by-vault
  (testing "Build query with wikilink LIKE pattern"
    (let [vault-id (utils/generate-uuid)
          captured-query (atom nil)]
      (with-redefs [db/execute! (fn [query]
                                  (reset! captured-query query)
                                  [])]
        (is (= [] (db/list-notes-with-wikilinks-by-vault vault-id)))
        (is (= [:client_id :path :content] (:select @captured-query)))
        (is (= [:notes] (:from @captured-query)))
        (is (= [:and
                [:= :vault_id vault-id]
                [:is :deleted_at nil]
                [:like :content "%[[%]]"]]
               (:where @captured-query)))))))

(deftest test-get-note
  (testing "Get note by ID"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "blog-get.com" sync-key)
          note-id (utils/generate-uuid)
          client-id "test-client-get"
          path "test.md"
          content "# Test Note"
          _ (upsert-note! note-id tenant-id vault-id path client-id content "{}" "hash" "2025-12-21T10:00:00Z")
          result (get-note note-id)]
      (is (map? result))
      (is (= note-id (:id result)))
      (is (= content (:content result)))))

  (testing "Get non-existent note"
    (let [result (get-note "non-existent-id")]
      (is (nil? result)))))

;;; ============================================================
;;; 测试反向链接功能
;;; ============================================================

(defn insert-note-link! [vault-id source-client-id target-client-id target-path link-type display-text original]
  "插入笔记链接（测试辅助函数）"
  (let [id (utils/generate-uuid)]
    (sql/insert! *conn* :note_links
                 {:id id
                  :vault_id vault-id
                  :source_client_id source-client-id
                  :target_client_id target-client-id
                  :target_path target-path
                  :link_type link-type
                  :display_text display-text
                  :original original}
                 {:builder-fn rs/as-unqualified-lower-maps})))

(defn get-note-links [vault-id source-client-id]
  "获取从指定笔记出发的所有链接（测试辅助函数）"
  (let [results (jdbc/execute! *conn*
                                ["SELECT * FROM note_links
                                  WHERE vault_id = ? AND source_client_id = ?
                                  ORDER BY created_at ASC"
                                 vault-id source-client-id]
                                {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(defn get-backlinks-with-notes [vault-id client-id]
  "获取反向链接及完整笔记信息（测试用例）"
  (let [results (jdbc/execute! *conn*
                                ["SELECT d.*, dl.display_text, dl.link_type
                                  FROM note_links dl
                                  INNER JOIN notes d ON d.vault_id = dl.vault_id AND d.client_id = dl.source_client_id
                                  WHERE dl.vault_id = ? AND dl.target_client_id = ?
                                  ORDER BY d.path ASC"
                                 vault-id client-id]
                                {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

;; Full Sync - 孤儿笔记清理辅助函数（测试用）

(defn list-note-client-ids-by-vault
  "获取 vault 中所有笔记的 client_ids（测试用）"
  [vault-id]
  (let [results (jdbc/execute! *conn*
                                ["SELECT client_id FROM notes WHERE vault_id = ?" vault-id]
                                {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(defn delete-notes-not-in-list!
  "删除不在列表中的笔记（测试用）"
  [vault-id client-ids]
  (if (empty? client-ids)
    (jdbc/execute-one! *conn*
                       ["DELETE FROM notes WHERE vault_id = ?" vault-id]
                       {:builder-fn rs/as-unqualified-lower-maps})
    (let [placeholders (str/join "," (repeat (count client-ids) "?"))
          sql (str "DELETE FROM notes WHERE vault_id = ? AND client_id NOT IN (" placeholders ")")
          params (into [vault-id] client-ids)]
      (jdbc/execute-one! *conn*
                         (into [sql] params)
                         {:builder-fn rs/as-unqualified-lower-maps}))))

(defn delete-orphan-links!
  "清理指向不存在笔记的链接（测试用）"
  [vault-id]
  (jdbc/execute-one! *conn*
                     ["DELETE FROM note_links
                       WHERE vault_id = ?
                       AND NOT EXISTS (
                         SELECT 1 FROM notes
                         WHERE notes.vault_id = note_links.vault_id
                         AND notes.client_id = note_links.target_client_id
                       )" vault-id]
                     {:builder-fn rs/as-unqualified-lower-maps}))

(deftest test-get-backlinks-with-notes
  (testing "获取反向链接及完整笔记信息"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "backlinks.com" sync-key)

          ;; 创建笔记
          target-note-id (utils/generate-uuid)
          target-client-id "target-client-123"
          _ (upsert-note! target-note-id tenant-id vault-id "Target.md" target-client-id "# Target Note" "{}" "hash1" "2025-12-21T10:00:00Z")

          source-note1-id (utils/generate-uuid)
          source-client-id1 "source-client-1"
          _ (upsert-note! source-note1-id tenant-id vault-id "Source A.md" source-client-id1 "# Source A" "{}" "hash2" "2025-12-21T11:00:00Z")

          source-note2-id (utils/generate-uuid)
          source-client-id2 "source-client-2"
          _ (upsert-note! source-note2-id tenant-id vault-id "Source B.md" source-client-id2 "# Source B" "{}" "hash3" "2025-12-21T12:00:00Z")

          ;; 创建链接：Source A -> Target, Source B -> Target
          _ (insert-note-link! vault-id source-client-id1 target-client-id "Target.md" "link" "Target" "[[Target]]")
          _ (insert-note-link! vault-id source-client-id2 target-client-id "Target.md" "link" "Target Note" "[[Target|Target Note]]")

          ;; 获取反向链接
          backlinks (get-backlinks-with-notes vault-id target-client-id)]

      (is (= 2 (count backlinks)))
      (is (= #{"Source A.md" "Source B.md"} (set (map :path backlinks))))
      (is (every? #(contains? % :content) backlinks))
      (is (every? #(contains? % :display-text) backlinks)))))

(deftest test-get-backlinks-with-notes-no-results
  (testing "笔记没有反向链接时返回空列表"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          sync-key (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "no-backlinks.com" sync-key)

          note-id (utils/generate-uuid)
          client-id "lonely-note"
          _ (upsert-note! note-id tenant-id vault-id "Lonely.md" client-id "# Lonely" "{}" "hash" "2025-12-21T10:00:00Z")

          backlinks (get-backlinks-with-notes vault-id client-id)]

      (is (= 0 (count backlinks))))))

;;; ============================================================
;;; 测试孤儿笔记清理功能 (Full Sync)
;;; ============================================================

(deftest test-list-note-client-ids-by-vault
  (testing "返回 vault 中所有笔记的 client_id"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "full-sync.com" "sync-key")
          note1-id (utils/generate-uuid)
          note2-id (utils/generate-uuid)
          client1 "client-1"
          client2 "client-2"]
      ;; Given: 创建 2 个笔记
      (upsert-note! note1-id tenant-id vault-id "a.md" client1 "c1" "{}" "h1" "2024-01-01T00:00:00Z")
      (upsert-note! note2-id tenant-id vault-id "b.md" client2 "c2" "{}" "h2" "2024-01-01T00:00:00Z")

      ;; When: 查询 client_ids
      (let [results (list-note-client-ids-by-vault vault-id)
            client-ids (map :client-id results)]

      ;; Then: 返回 2 个 client_id
        (is (= 2 (count client-ids)))
        (is (some #(= client1 %) client-ids))
        (is (some #(= client2 %) client-ids))))))

(deftest test-delete-notes-not-in-list
  (testing "删除不在列表中的笔记 (集合差集: server \\ client)"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "orphan-cleanup.com" "sync-key")
          ;; Given: 服务器有 3 个笔记 (c1, c2, c3)
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "a.md" "c1" "c" "{}" "h" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "b.md" "c2" "c" "{}" "h" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "c.md" "c3" "c" "{}" "h" "2024-01-01T00:00:00Z")

          ;; 客户端只有 c1 和 c2
          client-list ["c1" "c2"]

          before-count (-> (jdbc/execute-one! *conn*
                                          ["SELECT COUNT(*) as count FROM notes WHERE vault_id = ?" vault-id]
                                          {:builder-fn rs/as-unqualified-lower-maps})
                          :count)

          ;; When: 删除不在列表中的笔记
          _ (delete-notes-not-in-list! vault-id client-list)

          ;; Then: 删除了 c3，只剩 2 个笔记
          after-count (-> (jdbc/execute-one! *conn*
                                           ["SELECT COUNT(*) as count FROM notes WHERE vault_id = ?" vault-id]
                                           {:builder-fn rs/as-unqualified-lower-maps})
                          :count)
          remaining (jdbc/execute! *conn*
                                 ["SELECT client_id FROM notes WHERE vault_id = ? ORDER BY client_id" vault-id]
                                 {:builder-fn rs/as-unqualified-lower-maps})
          remaining-ids (map :client_id remaining)]

      (is (= 3 before-count))
      (is (= 2 after-count))
      (is (= ["c1" "c2"] remaining-ids)))))

(deftest test-delete-notes-not-in-list-empty
  (testing "空列表删除所有笔记"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "empty-sync.com" "sync-key")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "a.md" "c1" "c" "{}" "h" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "b.md" "c2" "c" "{}" "h" "2024-01-01T00:00:00Z")

          before-count (-> (jdbc/execute-one! *conn*
                                          ["SELECT COUNT(*) as count FROM notes WHERE vault_id = ?" vault-id]
                                          {:builder-fn rs/as-unqualified-lower-maps})
                          :count)

          ;; When: 空列表
          _ (delete-notes-not-in-list! vault-id [])

          ;; Then: 所有笔记被删除
          after-count (-> (jdbc/execute-one! *conn*
                                             ["SELECT COUNT(*) as count FROM notes WHERE vault_id = ?" vault-id]
                                             {:builder-fn rs/as-unqualified-lower-maps})
                            :count
                            )]

      (is (= 2 before-count))
      (is (= 0 after-count)))))

(deftest test-delete-orphan-links
  (testing "删除指向不存在笔记的链接"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "orphan-links.com" "sync-key")

          ;; Given: note1 链接到 note2
          note1-id (utils/generate-uuid)
          note2-id (utils/generate-uuid)
          client1 "source-client"
          client2 "target-client"
          _ (upsert-note! note1-id tenant-id vault-id "a.md" client1 "c" "{}" "h" "2024-01-01T00:00:00Z")
          _ (upsert-note! note2-id tenant-id vault-id "b.md" client2 "c" "{}" "h" "2024-01-01T00:00:00Z")
          link-id (utils/generate-uuid)
          _ (insert-note-link! vault-id client1 client2 "b.md" "link" "text" "[[b]]")

          before-count (count (get-note-links vault-id client1))

          ;; When: 删除 note2
          _ (jdbc/execute-one! *conn*
                             ["DELETE FROM notes WHERE vault_id = ? AND client_id = ?" vault-id client2])

          ;; 清理孤儿链接
          _ (delete-orphan-links! vault-id)

          ;; Then: 链接被清理
          after-count (count (get-note-links vault-id client1))]

      (is (= 1 before-count))
      (is (= 0 after-count)))))

;;; ============================================================
;;; 测试链接解析优化函数
;;; ============================================================

(defn get-notes-for-link-resolution
  "获取链接解析所需的最小数据（测试用）
   只返回 client_id 和 path，不返回 content 等大字段"
  [vault-id]
  (let [results (jdbc/execute! *conn*
                               ["SELECT client_id, path FROM notes WHERE vault_id = ?"
                                vault-id]
                               {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(deftest test-get-notes-for-link-resolution
  (testing "返回 vault 中所有笔记的 client_id 和 path（不含 content）"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "link-resolution.com" "sync-key")

          ;; Given: 创建多个笔记，包含较大的 content
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "folder/Note A.md" "client-a"
                              (apply str (repeat 10000 "Large content ")) "{}" "h1" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "Note B.md" "client-b"
                              (apply str (repeat 10000 "More large content ")) "{}" "h2" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "deep/nested/Note C.md" "client-c"
                              "Small content" "{}" "h3" "2024-01-01T00:00:00Z")

          ;; When: 获取链接解析所需数据
          results (get-notes-for-link-resolution vault-id)]

      ;; Then: 返回 3 个笔记
      (is (= 3 (count results)))

      ;; 每个结果只包含 client-id 和 path
      (is (every? #(contains? % :client-id) results))
      (is (every? #(contains? % :path) results))

      ;; 不包含 content（这是优化的关键）
      (is (every? #(not (contains? % :content)) results))

      ;; 验证数据正确性
      (let [by-client-id (into {} (map (fn [r] [(:client-id r) r]) results))]
        (is (= "folder/Note A.md" (:path (get by-client-id "client-a"))))
        (is (= "Note B.md" (:path (get by-client-id "client-b"))))
        (is (= "deep/nested/Note C.md" (:path (get by-client-id "client-c")))))))

  (testing "空 vault 返回空列表"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Empty Blog" "empty-link.com" "sync-key-2")

          results (get-notes-for-link-resolution vault-id)]

      (is (= 0 (count results)))
      (is (empty? results))))

  (testing "不同 vault 之间数据隔离"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id-1 (utils/generate-uuid)
          vault-id-2 (utils/generate-uuid)
          _ (create-vault! vault-id-1 tenant-id "Blog 1" "isolation1.com" "sync-key-3")
          _ (create-vault! vault-id-2 tenant-id "Blog 2" "isolation2.com" "sync-key-4")

          ;; vault-1 有 2 个笔记
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id-1 "a.md" "v1-a" "c" "{}" "h" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id-1 "b.md" "v1-b" "c" "{}" "h" "2024-01-01T00:00:00Z")

          ;; vault-2 有 1 个笔记
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id-2 "c.md" "v2-c" "c" "{}" "h" "2024-01-01T00:00:00Z")

          results-1 (get-notes-for-link-resolution vault-id-1)
          results-2 (get-notes-for-link-resolution vault-id-2)]

      (is (= 2 (count results-1)))
      (is (= 1 (count results-2)))
      (is (= #{"v1-a" "v1-b"} (set (map :client-id results-1))))
      (is (= #{"v2-c"} (set (map :client-id results-2)))))))

;;; ============================================================
;;; Asset 测试
;;; ============================================================

(defn upsert-asset! [id tenant-id vault-id client-id path object-key size-bytes content-type md5]
  (execute-one!
    ["INSERT INTO assets (id, tenant_id, vault_id, client_id, path, object_key, size_bytes, content_type, md5, deleted_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
      ON CONFLICT(vault_id, client_id) DO UPDATE SET
        path = excluded.path,
        size_bytes = excluded.size_bytes,
        content_type = excluded.content_type,
        md5 = excluded.md5,
        deleted_at = NULL,
        updated_at = strftime('%s', 'now')"
     id tenant-id vault-id client-id path object-key size-bytes content-type md5]))

(defn soft-delete-asset! [vault-id client-id]
  (execute-one!
    ["UPDATE assets SET deleted_at = strftime('%s', 'now'), updated_at = strftime('%s', 'now')
      WHERE vault_id = ? AND client_id = ? AND deleted_at IS NULL"
     vault-id client-id]))

(defn get-asset-by-client-id [vault-id client-id]
  (execute-one!
    ["SELECT * FROM assets WHERE vault_id = ? AND client_id = ? AND deleted_at IS NULL"
     vault-id client-id]))

(defn get-asset-by-path [vault-id path]
  (execute-one!
    ["SELECT * FROM assets WHERE vault_id = ? AND path = ? AND deleted_at IS NULL"
     vault-id path]))

(defn get-asset-by-filename [vault-id filename]
  (execute-one!
    ["SELECT * FROM assets WHERE vault_id = ? AND path LIKE ? AND deleted_at IS NULL LIMIT 1"
     vault-id (str "%" filename)]))

(defn find-asset [vault-id path]
  (or (get-asset-by-path vault-id path)
      (get-asset-by-filename vault-id (str "/" path))))

(defn list-assets-by-vault [vault-id]
  (let [results (jdbc/execute! *conn*
                               ["SELECT client_id, path, md5, size_bytes FROM assets
                                 WHERE vault_id = ? AND deleted_at IS NULL
                                 ORDER BY path ASC"
                                vault-id]
                               {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(deftest test-upsert-asset
  (testing "Insert new asset with client_id"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-insert.com" (utils/generate-uuid))
          asset-id (utils/generate-uuid)
          client-id (utils/generate-uuid)
          path "images/photo.png"
          object-key "assets/images/photo.png"
          size-bytes 12345
          content-type "image/png"
          md5 "abc123def456"]
      (upsert-asset! asset-id tenant-id vault-id client-id path object-key size-bytes content-type md5)
      (let [result (get-asset-by-client-id vault-id client-id)]
        (is (map? result))
        (is (= client-id (:client-id result)))
        (is (= path (:path result)))
        (is (= object-key (:object-key result)))
        (is (= size-bytes (:size-bytes result)))
        (is (= content-type (:content-type result)))
        (is (= md5 (:md5 result))))))

  (testing "Update existing asset by client_id"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-update.com" (utils/generate-uuid))
          asset-id (utils/generate-uuid)
          client-id (utils/generate-uuid)
          path "images/logo.svg"
          _ (upsert-asset! asset-id tenant-id vault-id client-id path "assets/old" 100 "image/svg+xml" "old-hash")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id "images/logo-renamed.svg" "assets/new" 200 "image/svg+xml" "new-hash")
          result (get-asset-by-client-id vault-id client-id)]
      (is (= 200 (:size-bytes result)))
      (is (= "new-hash" (:md5 result)))
      (is (= "images/logo-renamed.svg" (:path result)))))

  (testing "MOVE scenario: object_key remains stable when path changes"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-move.com" (utils/generate-uuid))
          client-id (utils/generate-uuid)
          original-object-key (str "assets/" client-id)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id "images/photo.png" original-object-key 1000 "image/png" "hash1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id "attachments/photo.png" "assets/new-path" 1000 "image/png" "hash1")
          result (get-asset-by-client-id vault-id client-id)]
      (is (= "attachments/photo.png" (:path result)) "path should update")
      (is (= original-object-key (:object-key result)) "object_key should NOT change on move"))))

(deftest test-soft-delete-asset
  (testing "Soft delete asset by client_id"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-delete.com" (utils/generate-uuid))
          client-id (utils/generate-uuid)
          path "images/delete-me.png"
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id path "assets/x" 100 "image/png" "hash")
          before (get-asset-by-client-id vault-id client-id)
          _ (soft-delete-asset! vault-id client-id)
          after (get-asset-by-client-id vault-id client-id)]
      (is (some? before))
      (is (nil? after))))

  (testing "Restore soft-deleted asset via upsert"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-restore.com" (utils/generate-uuid))
          client-id (utils/generate-uuid)
          path "images/restore.png"
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id path "assets/x" 100 "image/png" "hash1")
          _ (soft-delete-asset! vault-id client-id)
          deleted (get-asset-by-client-id vault-id client-id)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id path "assets/x" 150 "image/png" "hash2")
          restored (get-asset-by-client-id vault-id client-id)]
      (is (nil? deleted))
      (is (some? restored))
      (is (= 150 (:size-bytes restored))))))

(deftest test-list-assets-by-vault
  (testing "List assets by vault"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-list.com" (utils/generate-uuid))
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "images/a.png" "res/a" 100 "image/png" "hash-a")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "images/b.png" "res/b" 200 "image/png" "hash-b")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "docs/c.pdf" "res/c" 300 "application/pdf" "hash-c")
          results (list-assets-by-vault vault-id)]
      (is (= 3 (count results)))
      (is (= ["docs/c.pdf" "images/a.png" "images/b.png"] (map :path results)))))

  (testing "List excludes soft-deleted assets"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "asset-list-delete.com" (utils/generate-uuid))
          client-id-a (utils/generate-uuid)
          client-id-b (utils/generate-uuid)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id-a "a.png" "res/a" 100 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id-b "b.png" "res/b" 200 "image/png" "h2")
          _ (soft-delete-asset! vault-id client-id-a)
          results (list-assets-by-vault vault-id)]
      (is (= 1 (count results)))
      (is (= "b.png" (:path (first results)))))))

(deftest test-find-asset
  (testing "Find by exact path"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "find-exact.com" (utils/generate-uuid))
          client-id (utils/generate-uuid)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id "images/photo.png" "assets/images/photo.png" 100 "image/png" "hash1")
          result (find-asset vault-id "images/photo.png")]
      (is (some? result))
      (is (= "images/photo.png" (:path result)))))

  (testing "Find by filename when exact path not found"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "find-filename.com" (utils/generate-uuid))
          client-id (utils/generate-uuid)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id "images/mdb.png" "assets/images/mdb.png" 100 "image/png" "hash1")
          result (find-asset vault-id "mdb.png")]
      (is (some? result))
      (is (= "images/mdb.png" (:path result)))
      (is (= "assets/images/mdb.png" (:object-key result)))))

  (testing "Returns nil when asset not found"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "find-nil.com" (utils/generate-uuid))
          result (find-asset vault-id "nonexistent.png")]
      (is (nil? result)))))

;;; ============================================================
;;; Vault Logo 测试
;;; ============================================================

(defn update-vault-logo! [vault-id logo-object-key]
  (execute-one! ["UPDATE vaults SET logo_object_key = ? WHERE id = ?" logo-object-key vault-id]))

(deftest test-update-vault-logo
  (testing "Update vault logo"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "logo-test.com" (utils/generate-uuid))
          _ (update-vault-logo! vault-id "site/logo/my-logo.png")
          result (get-vault-by-id vault-id)]
      (is (= "site/logo/my-logo.png" (:logo-object-key result)))))

  (testing "Clear vault logo"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "logo-clear.com" (utils/generate-uuid))
          _ (update-vault-logo! vault-id "site/logo/logo.svg")
          _ (update-vault-logo! vault-id nil)
          result (get-vault-by-id vault-id)]
      (is (nil? (:logo-object-key result))))))

;;; ============================================================
;;; Vault Storage Size 测试
;;; ============================================================

(defn get-vault-storage-size [vault-id]
  (let [result (execute-one!
                 ["SELECT COALESCE(SUM(size_bytes), 0) as total_bytes
                   FROM assets
                   WHERE vault_id = ? AND deleted_at IS NULL"
                  vault-id])]
    (:total-bytes result)))

(deftest test-get-vault-storage-size
  (testing "Empty vault returns 0"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "storage-empty.com" (utils/generate-uuid))
          size (get-vault-storage-size vault-id)]
      (is (= 0 size))))

  (testing "Sum of all asset sizes"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "storage-sum.com" (utils/generate-uuid))
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "a.png" "res/a" 1000 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "b.jpg" "res/b" 2500 "image/jpeg" "h2")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "c.pdf" "res/c" 5000 "application/pdf" "h3")
          size (get-vault-storage-size vault-id)]
      (is (= 8500 size))))

  (testing "Excludes soft-deleted assets"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "storage-deleted.com" (utils/generate-uuid))
          client-id-a (utils/generate-uuid)
          client-id-b (utils/generate-uuid)
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id-a "a.png" "res/a" 1000 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id client-id-b "b.png" "res/b" 2000 "image/png" "h2")
          _ (soft-delete-asset! vault-id client-id-b)
          size (get-vault-storage-size vault-id)]
      (is (= 1000 size))))

  (testing "Different vaults isolated"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id-1 (utils/generate-uuid)
          vault-id-2 (utils/generate-uuid)
          _ (create-vault! vault-id-1 tenant-id "Blog 1" "storage-iso1.com" (utils/generate-uuid))
          _ (create-vault! vault-id-2 tenant-id "Blog 2" "storage-iso2.com" (utils/generate-uuid))
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id-1 (utils/generate-uuid) "a.png" "res/a" 1000 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id-2 (utils/generate-uuid) "b.png" "res/b" 5000 "image/png" "h2")
          size-1 (get-vault-storage-size vault-id-1)
          size-2 (get-vault-storage-size vault-id-2)]
      (is (= 1000 size-1))
      (is (= 5000 size-2)))))

;;; ============================================================
;;; Note Asset Refs 测试 (引用追踪)
;;; ============================================================

(defn upsert-note-asset-ref! [vault-id note-client-id asset-client-id]
  (let [id (utils/generate-uuid)]
    (execute-one!
      ["INSERT INTO note_asset_refs (id, vault_id, note_client_id, asset_client_id)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(vault_id, note_client_id, asset_client_id) DO NOTHING"
       id vault-id note-client-id asset-client-id])))

(defn delete-note-asset-refs-by-note! [vault-id note-client-id]
  (execute-one!
    ["DELETE FROM note_asset_refs WHERE vault_id = ? AND note_client_id = ?"
     vault-id note-client-id]))

(defn get-asset-refs-by-note [vault-id note-client-id]
  (let [results (jdbc/execute! *conn*
                               ["SELECT * FROM note_asset_refs WHERE vault_id = ? AND note_client_id = ?"
                                vault-id note-client-id]
                               {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(defn get-asset-refs-by-asset [vault-id asset-client-id]
  (let [results (jdbc/execute! *conn*
                               ["SELECT * FROM note_asset_refs WHERE vault_id = ? AND asset_client_id = ?"
                                vault-id asset-client-id]
                               {:builder-fn rs/as-unqualified-lower-maps})]
    (map db-keys->clojure results)))

(defn count-asset-refs [vault-id asset-client-id]
  (let [result (execute-one!
                 ["SELECT COUNT(*) as count FROM note_asset_refs 
                   WHERE vault_id = ? AND asset_client_id = ?"
                  vault-id asset-client-id])]
    (:count result)))

(defn update-note-asset-refs!
  [vault-id note-client-id new-asset-client-ids]
  (delete-note-asset-refs-by-note! vault-id note-client-id)
  (doseq [asset-client-id new-asset-client-ids]
    (upsert-note-asset-ref! vault-id note-client-id asset-client-id)))

(deftest test-upsert-note-asset-ref
  (testing "Insert new reference"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-insert.com" (utils/generate-uuid))
          note-client-id (utils/generate-uuid)
          asset-client-id (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-client-id asset-client-id)
      (let [refs (get-asset-refs-by-note vault-id note-client-id)]
        (is (= 1 (count refs)))
        (is (= asset-client-id (:asset-client-id (first refs)))))))

  (testing "Duplicate reference is ignored"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-dup.com" (utils/generate-uuid))
          note-client-id (utils/generate-uuid)
          asset-client-id (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-client-id asset-client-id)
      (upsert-note-asset-ref! vault-id note-client-id asset-client-id)
      (let [refs (get-asset-refs-by-note vault-id note-client-id)]
        (is (= 1 (count refs)))))))

(deftest test-delete-note-asset-refs
  (testing "Delete all refs for a note"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-delete.com" (utils/generate-uuid))
          note-client-id (utils/generate-uuid)
          asset-1 (utils/generate-uuid)
          asset-2 (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-client-id asset-1)
      (upsert-note-asset-ref! vault-id note-client-id asset-2)
      (is (= 2 (count (get-asset-refs-by-note vault-id note-client-id))))
      (delete-note-asset-refs-by-note! vault-id note-client-id)
      (is (= 0 (count (get-asset-refs-by-note vault-id note-client-id)))))))

(deftest test-count-asset-refs
  (testing "Count refs for an asset"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-count.com" (utils/generate-uuid))
          note-1 (utils/generate-uuid)
          note-2 (utils/generate-uuid)
          note-3 (utils/generate-uuid)
          shared-asset (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-1 shared-asset)
      (upsert-note-asset-ref! vault-id note-2 shared-asset)
      (upsert-note-asset-ref! vault-id note-3 shared-asset)
      (is (= 3 (count-asset-refs vault-id shared-asset)))))

  (testing "Zero refs for orphan asset"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-orphan.com" (utils/generate-uuid))
          orphan-asset (utils/generate-uuid)]
      (is (= 0 (count-asset-refs vault-id orphan-asset))))))

(deftest test-update-note-asset-refs
  (testing "Replace all refs for a note"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "ref-update.com" (utils/generate-uuid))
          note-client-id (utils/generate-uuid)
          old-asset-1 (utils/generate-uuid)
          old-asset-2 (utils/generate-uuid)
          new-asset-1 (utils/generate-uuid)
          new-asset-3 (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-client-id old-asset-1)
      (upsert-note-asset-ref! vault-id note-client-id old-asset-2)
      (is (= 2 (count (get-asset-refs-by-note vault-id note-client-id))))
      (update-note-asset-refs! vault-id note-client-id [new-asset-1 new-asset-3])
      (let [refs (get-asset-refs-by-note vault-id note-client-id)
            asset-ids (set (map :asset-client-id refs))]
        (is (= 2 (count refs)))
        (is (contains? asset-ids new-asset-1))
        (is (contains? asset-ids new-asset-3))
        (is (not (contains? asset-ids old-asset-1)))
        (is (not (contains? asset-ids old-asset-2)))))))

(deftest test-asset-orphan-detection
  (testing "Asset becomes orphan when last note reference is removed"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "orphan-detect.com" (utils/generate-uuid))
          note-1 (utils/generate-uuid)
          note-2 (utils/generate-uuid)
          shared-asset (utils/generate-uuid)
          unique-asset (utils/generate-uuid)]
      (upsert-note-asset-ref! vault-id note-1 shared-asset)
      (upsert-note-asset-ref! vault-id note-2 shared-asset)
      (upsert-note-asset-ref! vault-id note-1 unique-asset)
      (is (= 2 (count-asset-refs vault-id shared-asset)))
      (is (= 1 (count-asset-refs vault-id unique-asset)))
      (delete-note-asset-refs-by-note! vault-id note-1)
      (is (= 1 (count-asset-refs vault-id shared-asset)))
      (is (= 0 (count-asset-refs vault-id unique-asset))))))

;;; ============================================================
;;; Vault Cascade Delete 测试
;;; ============================================================

(defn delete-vault! [vault-id]
  "删除 vault（测试辅助函数）"
  (execute-one! ["DELETE FROM vaults WHERE id = ?" vault-id]))

(defn count-notes-by-vault [vault-id]
  (let [result (execute-one! ["SELECT COUNT(*) as count FROM notes WHERE vault_id = ?" vault-id])]
    (:count result)))

(defn count-note-links-by-vault [vault-id]
  (let [result (execute-one! ["SELECT COUNT(*) as count FROM note_links WHERE vault_id = ?" vault-id])]
    (:count result)))

(defn count-assets-by-vault [vault-id]
  (let [result (execute-one! ["SELECT COUNT(*) as count FROM assets WHERE vault_id = ?" vault-id])]
    (:count result)))

(defn count-note-asset-refs-by-vault [vault-id]
  (let [result (execute-one! ["SELECT COUNT(*) as count FROM note_asset_refs WHERE vault_id = ?" vault-id])]
    (:count result)))

(deftest test-vault-cascade-delete-notes
  (testing "Deleting vault cascades to delete all notes"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "cascade-notes.com" (utils/generate-uuid))
          ;; Create multiple notes
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note1.md" "c1" "# Note 1" "{}" "h1" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note2.md" "c2" "# Note 2" "{}" "h2" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "folder/note3.md" "c3" "# Note 3" "{}" "h3" "2024-01-01T00:00:00Z")
          
          ;; Verify notes exist
          notes-before (count-notes-by-vault vault-id)
          _ (is (= 3 notes-before))
          
          ;; Delete vault
          _ (delete-vault! vault-id)
          
          ;; Verify notes are deleted via CASCADE
          notes-after (count-notes-by-vault vault-id)]
      (is (= 0 notes-after)))))

(deftest test-vault-cascade-delete-note-links
  (testing "Deleting vault cascades to delete all note_links"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "cascade-links.com" (utils/generate-uuid))
          ;; Create notes with links
          source-client-id "source-client"
          target-client-id "target-client"
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "source.md" source-client-id "# Source" "{}" "h1" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "target.md" target-client-id "# Target" "{}" "h2" "2024-01-01T00:00:00Z")
          _ (insert-note-link! vault-id source-client-id target-client-id "target.md" "link" "Target" "[[Target]]")
          _ (insert-note-link! vault-id source-client-id target-client-id "target.md" "embed" "Target" "![[Target]]")
          
          ;; Verify links exist
          links-before (count-note-links-by-vault vault-id)
          _ (is (= 2 links-before))
          
          ;; Delete vault
          _ (delete-vault! vault-id)
          
          ;; Verify links are deleted via CASCADE
          links-after (count-note-links-by-vault vault-id)]
      (is (= 0 links-after)))))

(deftest test-vault-cascade-delete-assets
  (testing "Deleting vault cascades to delete all assets"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "cascade-assets.com" (utils/generate-uuid))
          ;; Create assets
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "images/a.png" "assets/a" 1000 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "images/b.jpg" "assets/b" 2000 "image/jpeg" "h2")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id (utils/generate-uuid) "docs/c.pdf" "assets/c" 3000 "application/pdf" "h3")
          
          ;; Verify assets exist
          assets-before (count-assets-by-vault vault-id)
          _ (is (= 3 assets-before))
          
          ;; Delete vault
          _ (delete-vault! vault-id)
          
          ;; Verify assets are deleted via CASCADE
          assets-after (count-assets-by-vault vault-id)]
      (is (= 0 assets-after)))))

(deftest test-vault-cascade-delete-note-asset-refs
  (testing "Deleting vault cascades to delete all note_asset_refs"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "cascade-refs.com" (utils/generate-uuid))
          ;; Create note-asset references
          note-client-1 (utils/generate-uuid)
          note-client-2 (utils/generate-uuid)
          asset-client-1 (utils/generate-uuid)
          asset-client-2 (utils/generate-uuid)
          _ (upsert-note-asset-ref! vault-id note-client-1 asset-client-1)
          _ (upsert-note-asset-ref! vault-id note-client-1 asset-client-2)
          _ (upsert-note-asset-ref! vault-id note-client-2 asset-client-1)
          
          ;; Verify refs exist
          refs-before (count-note-asset-refs-by-vault vault-id)
          _ (is (= 3 refs-before))
          
          ;; Delete vault
          _ (delete-vault! vault-id)
          
          ;; Verify refs are deleted via CASCADE
          refs-after (count-note-asset-refs-by-vault vault-id)]
      (is (= 0 refs-after)))))

(deftest test-vault-cascade-delete-all-related
  (testing "Deleting vault cascades to delete all related data at once"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          _ (create-vault! vault-id tenant-id "Blog" "cascade-all.com" (utils/generate-uuid))
          
          ;; Create comprehensive data set
          note-client-1 "note-1"
          note-client-2 "note-2"
          asset-client-1 (utils/generate-uuid)
          asset-client-2 (utils/generate-uuid)
          
          ;; Notes
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note1.md" note-client-1 "# Note 1" "{}" "h1" "2024-01-01T00:00:00Z")
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id "note2.md" note-client-2 "# Note 2" "{}" "h2" "2024-01-01T00:00:00Z")
          
          ;; Links between notes
          _ (insert-note-link! vault-id note-client-1 note-client-2 "note2.md" "link" "Note 2" "[[note2]]")
          
          ;; Assets
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id asset-client-1 "img/a.png" "assets/a" 1000 "image/png" "h1")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id asset-client-2 "img/b.png" "assets/b" 2000 "image/png" "h2")
          
          ;; Asset references
          _ (upsert-note-asset-ref! vault-id note-client-1 asset-client-1)
          _ (upsert-note-asset-ref! vault-id note-client-2 asset-client-2)
          
          ;; Verify all data exists
          _ (is (= 2 (count-notes-by-vault vault-id)))
          _ (is (= 1 (count-note-links-by-vault vault-id)))
          _ (is (= 2 (count-assets-by-vault vault-id)))
          _ (is (= 2 (count-note-asset-refs-by-vault vault-id)))
          
          ;; Delete vault
          _ (delete-vault! vault-id)]
      
      ;; Verify everything is deleted
      (is (= 0 (count-notes-by-vault vault-id)))
      (is (= 0 (count-note-links-by-vault vault-id)))
      (is (= 0 (count-assets-by-vault vault-id)))
      (is (= 0 (count-note-asset-refs-by-vault vault-id))))))

(deftest test-vault-cascade-delete-isolation
  (testing "Deleting one vault does not affect other vault's data"
    (let [tenant-id (utils/generate-uuid)
          _ (create-tenant! tenant-id "Test Org")
          vault-id-1 (utils/generate-uuid)
          vault-id-2 (utils/generate-uuid)
          _ (create-vault! vault-id-1 tenant-id "Blog 1" "cascade-iso1.com" (utils/generate-uuid))
          _ (create-vault! vault-id-2 tenant-id "Blog 2" "cascade-iso2.com" (utils/generate-uuid))
          
          ;; Create data in vault 1
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id-1 "v1-note.md" "v1-c" "# V1" "{}" "h1" "2024-01-01T00:00:00Z")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id-1 (utils/generate-uuid) "v1.png" "assets/v1" 100 "image/png" "h1")
          
          ;; Create data in vault 2
          _ (upsert-note! (utils/generate-uuid) tenant-id vault-id-2 "v2-note.md" "v2-c" "# V2" "{}" "h2" "2024-01-01T00:00:00Z")
          _ (upsert-asset! (utils/generate-uuid) tenant-id vault-id-2 (utils/generate-uuid) "v2.png" "assets/v2" 200 "image/png" "h2")
          
          ;; Verify both have data
          _ (is (= 1 (count-notes-by-vault vault-id-1)))
          _ (is (= 1 (count-notes-by-vault vault-id-2)))
          _ (is (= 1 (count-assets-by-vault vault-id-1)))
          _ (is (= 1 (count-assets-by-vault vault-id-2)))
          
          ;; Delete vault 1 only
          _ (delete-vault! vault-id-1)]
      
      ;; Vault 1 data is gone
      (is (= 0 (count-notes-by-vault vault-id-1)))
      (is (= 0 (count-assets-by-vault vault-id-1)))
      
      ;; Vault 2 data is intact
      (is (= 1 (count-notes-by-vault vault-id-2)))
      (is (= 1 (count-assets-by-vault vault-id-2))))))
