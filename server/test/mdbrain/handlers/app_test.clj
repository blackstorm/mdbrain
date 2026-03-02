(ns mdbrain.handlers.app-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [clojure.string :as str]
            [clojure.java.io :as io]
            [mdbrain.db :as db]
            [mdbrain.config :as config]
            [mdbrain.object-store :as object-store]
            [mdbrain.object-store.local :as local-store]
            [mdbrain.utils :as utils]
            [mdbrain.handlers.app :as app]
            [mdbrain.handlers.console :as console]
            [next.jdbc :as jdbc]
            [ring.mock.request :as mock]
            [selmer.parser :as selmer]))

;; 测试数据库（临时文件 SQLite）
(defn setup-test-db [f]
  (let [temp-file (java.io.File/createTempFile "test-db" ".db")
        _ (.deleteOnExit temp-file)
        test-ds (delay (jdbc/get-datasource {:dbtype "sqlite" :dbname (.getPath temp-file)}))]
    (with-redefs [db/datasource test-ds
                  selmer/render-file (fn [_ data] (str "Mocked template: " data))]
      (db/init-db!)
      (f)
      (.delete temp-file))))

(use-fixtures :each setup-test-db)

;; Get Vault by Domain 测试
(deftest test-get-vault-by-domain
  (testing "Get vault with valid domain from Host header"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "myblog.com"
          _ (db/create-vault! vault-id tenant-id "My Blog" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" domain}))
          response (app/get-note request)]
      (is (= 200 (:status response)))
      (is (string? (:body response)))
      (is (clojure.string/includes? (:body response) "My Blog"))))

  (testing "Get vault with port in Host header"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "localhost"
          _ (db/create-vault! vault-id tenant-id "Local Blog" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" "localhost:3000"}))
          response (app/get-note request)]
      (is (= 200 (:status response)))
      (is (string? (:body response)))))

  (testing "Non-existent domain returns 403"
    (let [request (-> (mock/request :get "/")
                      (assoc :headers {"host" "nonexistent.com"}))
          response (app/get-note request)]
      (is (= 403 (:status response)))
      (is (string? (:body response)))
      (is (clojure.string/includes? (:body response) "Forbidden"))))

  (testing "Missing Host header"
    (let [request (assoc (mock/request :get "/") :headers {})
          response (app/get-note request)]
      (is (= 400 (:status response)))
      (is (clojure.string/includes? (:body response) "Bad request"))))

  (testing "Invalid Host header"
    (let [request (-> (mock/request :get "/")
                      (assoc :headers {"host" "bad host"}))
          response (app/get-note request)]
      (is (= 400 (:status response)))
      (is (clojure.string/includes? (:body response) "Bad request")))))

;; Console Pages 测试
(deftest test-console-pages
  (testing "Console home page renders"
    (let [request (mock/request :get "/console")
          response (console/console-home request)]
      (is (= 200 (:status response)))
      (is (string? (:body response)))))

  (testing "Login page renders"
    (let [request (mock/request :get "/console/login")
          response (console/login-page request)]
      (is (= 200 (:status response)))
      (is (string? (:body response))))))

;; Domain Parsing 测试
(deftest test-domain-parsing
  (testing "Parse domain without port"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "example.com"
          _ (db/create-vault! vault-id tenant-id "Site" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" "example.com"}))
          response (app/get-note request)]
      (is (= 200 (:status response)))))

  (testing "Parse domain with port"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "exampleport.com"
          _ (db/create-vault! vault-id tenant-id "Site" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" "exampleport.com:8080"}))
          response (app/get-note request)]
      (is (= 200 (:status response)))))

  (testing "Parse IPv4 address"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "192.168.1.1"
          _ (db/create-vault! vault-id tenant-id "Site" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" "192.168.1.1:3000"}))
          response (app/get-note request)]
      (is (= 200 (:status response)))))

  (testing "Parse localhost"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "localhost"
          _ (db/create-vault! vault-id tenant-id "Local" domain (utils/generate-uuid))
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" "localhost:3000"}))
          response (app/get-note request)]
      (is (= 200 (:status response))))))

;; ============================================================
;; URL 使用 client_id 而非内部 id 测试
;; ============================================================

(deftest test-url-uses-client-id
  (testing "URL 中应该使用 client_id 而不是内部 id"
    (let [;; 创建测试数据
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "clientid-test.com"
          _ (db/create-vault! vault-id tenant-id "Client ID Test" domain (utils/generate-uuid))

          ;; 创建文档 - 注意 id 和 client_id 是不同的
          internal-id (utils/generate-uuid)
          client-id "user-generated-client-id-12345"
          _ (db/upsert-note! internal-id tenant-id vault-id "test.md" client-id
                             "# Test Content" nil "hash123" "2024-01-01T00:00:00Z")

          ;; 通过 client_id 访问文档（应该成功）
          request (-> (mock/request :get (str "/" client-id))
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path client-id}))
          response (app/get-note request)]

      ;; 验证能通过 client_id 访问
      (is (= 200 (:status response)))
      ;; 验证模板数据包含 client-id（因为模板被 mock 了，检查数据而非渲染结果）
      (is (str/includes? (:body response) ":client-id"))
      (is (str/includes? (:body response) client-id))))

  (testing "通过内部 id 访问应该返回 404（不暴露内部 id）"
    (let [;; 创建测试数据
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 2")
          vault-id (utils/generate-uuid)
          domain "internal-id-test.com"
          _ (db/create-vault! vault-id tenant-id "Internal ID Test" domain (utils/generate-uuid))

          ;; 创建文档
          internal-id (utils/generate-uuid)
          client-id "my-client-id-67890"
          _ (db/upsert-note! internal-id tenant-id vault-id "test2.md" client-id
                             "# Private Content" nil "hash456" "2024-01-01T00:00:00Z")

          ;; 尝试通过内部 id 访问（应该失败）
          request (-> (mock/request :get (str "/" internal-id))
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path internal-id}))
          response (app/get-note request)]

      ;; 验证不能通过内部 id 访问
      (is (= 404 (:status response)))))

  (testing "home 页面文档列表应该使用 client_id"
    (let [;; 创建测试数据
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 3")
          vault-id (utils/generate-uuid)
          domain "home-test.com"
          _ (db/create-vault! vault-id tenant-id "Home Test" domain (utils/generate-uuid))

          ;; 创建多个文档
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "doc1.md"
                             "client-id-aaa" "# Doc 1" nil "hash1" "2024-01-01T00:00:00Z")
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "doc2.md"
                             "client-id-bbb" "# Doc 2" nil "hash2" "2024-01-01T00:00:00Z")

          ;; 访问根路径（没有 root_doc_id，显示文档列表）
          ;; 根路径 path 为 "/" 或空
          request (-> (mock/request :get "/")
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path "/"}))
          response (app/get-note request)]

      ;; 验证返回成功
      (is (= 200 (:status response)))
      ;; 验证链接使用 client_id 而非内部 id
      (is (str/includes? (:body response) "client-id-aaa"))
      (is (str/includes? (:body response) "client-id-bbb"))))

  (testing "backlinks 应该使用 client_id"
    (let [;; 创建测试数据
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 4")
          vault-id (utils/generate-uuid)
          domain "backlinks-test.com"
          _ (db/create-vault! vault-id tenant-id "Backlinks Test" domain (utils/generate-uuid))

          ;; 创建目标文档
          target-client-id "target-doc-client-id"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "target.md"
                             target-client-id "# Target Doc" nil "hash-t" "2024-01-01T00:00:00Z")

          ;; 创建源文档（包含链接到目标）
          source-client-id "source-doc-client-id"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "source.md"
                             source-client-id "# Source Doc" nil "hash-s" "2024-01-01T00:00:00Z")

          ;; 创建链接关系
          _ (db/insert-note-link! vault-id source-client-id target-client-id
                                  "target.md" "link" "Target Doc" "[[target]]")

          ;; 访问目标文档，查看 backlinks
          request (-> (mock/request :get (str "/" target-client-id))
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path target-client-id}))
          response (app/get-note request)]

      ;; 验证返回成功
      (is (= 200 (:status response)))
      ;; 验证 backlinks 使用 client_id (数据中包含 source-client-id)
      (is (str/includes? (:body response) source-client-id))))

  (testing "堆叠路径应该使用 client_id"
    (let [;; 创建测试数据
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 5")
          vault-id (utils/generate-uuid)
          domain "stacking-test.com"
          _ (db/create-vault! vault-id tenant-id "Stacking Test" domain (utils/generate-uuid))

          ;; 创建多个文档
          client-id-1 "stack-doc-1"
          client-id-2 "stack-doc-2"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "doc1.md"
                             client-id-1 "# Doc 1" nil "hash1" "2024-01-01T00:00:00Z")
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "doc2.md"
                             client-id-2 "# Doc 2" nil "hash2" "2024-01-01T00:00:00Z")

          ;; 使用堆叠路径访问 (/{client_id}+{client_id})
          stacked-path (str client-id-1 "+" client-id-2)
          request (-> (mock/request :get (str "/" stacked-path))
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path stacked-path}))
          response (app/get-note request)]

      ;; 验证返回成功（两个文档都应该被渲染）
      (is (= 200 (:status response)))
      ;; 验证数据中包含两个 client-id
      (is (str/includes? (:body response) client-id-1))
      (is (str/includes? (:body response) client-id-2)))))

;; ============================================================
;; Asset Serving Tests
;; ============================================================

(defn- create-temp-storage []
  "Create a temporary storage directory for testing."
  (let [temp-dir (java.io.File/createTempFile "storage-test" "")]
    (.delete temp-dir)
    (.mkdirs temp-dir)
    (.deleteOnExit temp-dir)
    (.getPath temp-dir)))

(deftest test-serve-asset
  (testing "Serve asset from local storage successfully"
    (let [;; Setup temp storage
          temp-storage (create-temp-storage)
          ;; Create test data
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          vault-id (utils/generate-uuid)
          domain "asset-test.com"
          _ (db/create-vault! vault-id tenant-id "Asset Test" domain (utils/generate-uuid))

          ;; Create and store asset (object_key is now based on client_id)
          asset-content "test image content"
          client-id "test-client-123"
          object-key (str "assets/" client-id)]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        ;; Initialize storage
        (object-store/set-store! (local-store/create-local-store))

        ;; Store the asset
        (object-store/put-object! vault-id object-key asset-content "image/png")

        ;; Request the asset using object_key as path
        (let [request (-> (mock/request :get (str "/storage/" object-key))
                          (assoc :headers {"host" domain})
                          (assoc :path-params {:path object-key}))
              response (app/serve-asset request)]

          ;; Verify response
          (is (= 200 (:status response)))
          ;; Note: Content-Type falls back to application/octet-stream because
          ;; object_key is now "assets/{client_id}" without file extension.
          ;; Local storage guesses content-type from extension, but client_id has none.
          ;; In production, content_type should be looked up from DB assets table.
          (is (some? (get-in response [:headers "Content-Type"])))
          (is (bytes? (:body response)))
          (is (= asset-content (String. (:body response) "UTF-8")))))))

  (testing "Return 404 for non-existent asset"
    (let [temp-storage (create-temp-storage)
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 2")
          vault-id (utils/generate-uuid)
          domain "asset-404-test.com"
          _ (db/create-vault! vault-id tenant-id "Asset 404 Test" domain (utils/generate-uuid))]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))

        (let [request (-> (mock/request :get "/storage/assets/nonexistent")
                          (assoc :headers {"host" domain})
                          (assoc :path-params {:path "assets/nonexistent"}))
              response (app/serve-asset request)]

          (is (= 404 (:status response)))))))

  (testing "Return 403 for unknown domain (vault isolation)"
    (let [temp-storage (create-temp-storage)]
      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))

        (let [request (-> (mock/request :get "/storage/assets/some-client-id")
                          (assoc :headers {"host" "unknown-domain.com"})
                          (assoc :path-params {:path "assets/some-client-id"}))
              response (app/serve-asset request)]

          (is (= 403 (:status response)))))))

  (testing "Vault isolation - cannot access other vault's assets"
    (let [temp-storage (create-temp-storage)
          ;; Create two vaults
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 3")

          vault-id-1 (utils/generate-uuid)
          domain-1 "vault1.com"
          _ (db/create-vault! vault-id-1 tenant-id "Vault 1" domain-1 (utils/generate-uuid))

          vault-id-2 (utils/generate-uuid)
          domain-2 "vault2.com"
          _ (db/create-vault! vault-id-2 tenant-id "Vault 2" domain-2 (utils/generate-uuid))

          object-key "assets/secret-client-id"]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))

        ;; Store asset in vault-1 only
        (object-store/put-object! vault-id-1 object-key "vault1 secret" "image/png")

        ;; Access from vault-1 - should succeed
        (let [request-1 (-> (mock/request :get (str "/storage/" object-key))
                            (assoc :headers {"host" domain-1})
                            (assoc :path-params {:path object-key}))
              response-1 (app/serve-asset request-1)]
          (is (= 200 (:status response-1)))
          (is (= "vault1 secret" (String. (:body response-1) "UTF-8"))))

        ;; Access from vault-2 - should fail (404, not 403 to avoid enumeration)
        (let [request-2 (-> (mock/request :get (str "/storage/" object-key))
                            (assoc :headers {"host" domain-2})
                            (assoc :path-params {:path object-key}))
              response-2 (app/serve-asset request-2)]
          (is (= 404 (:status response-2)))))))

  (testing "Return 404 when using S3 storage"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org S3")
          vault-id (utils/generate-uuid)
          domain "any-domain.com"
          _ (db/create-vault! vault-id tenant-id "Any Domain" domain (utils/generate-uuid))]
      (with-redefs [config/storage-type (constantly :s3)]
        (let [request (-> (mock/request :get "/storage/assets/some-client-id")
                          (assoc :headers {"host" domain})
                          (assoc :path-params {:path "assets/some-client-id"}))
              response (app/serve-asset request)]
          (is (= 404 (:status response)))))))

  (testing "Return 400 for missing path"
    (let [temp-storage (create-temp-storage)
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 4")
          vault-id (utils/generate-uuid)
          domain "missing-path-test.com"
          _ (db/create-vault! vault-id tenant-id "Missing Path Test" domain (utils/generate-uuid))]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))

        (let [request (-> (mock/request :get "/storage/")
                          (assoc :headers {"host" domain})
                          (assoc :path-params {:path ""}))
              response (app/serve-asset request)]

          (is (= 400 (:status response)))))))

  (testing "Cache headers are set correctly"
    (let [temp-storage (create-temp-storage)
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org 5")
          vault-id (utils/generate-uuid)
          domain "cache-test.com"
          _ (db/create-vault! vault-id tenant-id "Cache Test" domain (utils/generate-uuid))
          object-key "assets/cached-client-id"]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))

        (object-store/put-object! vault-id object-key "cached content" "image/png")

        (let [request (-> (mock/request :get (str "/storage/" object-key))
                          (assoc :headers {"host" domain})
                          (assoc :path-params {:path object-key}))
              response (app/serve-asset request)]

          (is (= 200 (:status response)))
          (is (str/includes? (get-in response [:headers "Cache-Control"]) "max-age"))
          (is (str/includes? (get-in response [:headers "Cache-Control"]) "immutable")))))))

;; ============================================================
;; Favicon Serving Tests
;; ============================================================

(deftest test-serve-favicon
  (testing "Redirects to versioned favicon URL"
    (let [temp-storage (create-temp-storage)
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org Favicon")
          vault-id (utils/generate-uuid)
          domain "favicon-test.com"
          _ (db/create-vault! vault-id tenant-id "Favicon Test" domain (utils/generate-uuid))
          logo-hash "fav123"
          logo-object-key (str "site/logo/" logo-hash ".png")
          favicon-object-key (object-store/favicon-object-key logo-object-key)
          _ (db/update-vault-logo! vault-id logo-object-key)]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))
        (object-store/put-object! vault-id favicon-object-key "favicon-content" "image/png")

        (let [request (-> (mock/request :get "/favicon.ico?v=site%2Flogo%2Ffav123.png")
                          (assoc :headers {"host" domain}))
              response (app/serve-favicon request)]
          (is (= 302 (:status response)))
          (is (= (str "/storage/" favicon-object-key) (get-in response [:headers "Location"])))
          (is (str/includes? (get-in response [:headers "Cache-Control"]) "immutable"))
          (is (= (str "\"" logo-hash "\"") (get-in response [:headers "ETag"])))))))

  (testing "Uses short cache when unversioned"
    (let [temp-storage (create-temp-storage)
          tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org Favicon Unversioned")
          vault-id (utils/generate-uuid)
          domain "favicon-unversioned-test.com"
          _ (db/create-vault! vault-id tenant-id "Favicon Unversioned Test" domain (utils/generate-uuid))
          logo-hash "favunv"
          logo-object-key (str "site/logo/" logo-hash ".png")
          _ (db/update-vault-logo! vault-id logo-object-key)]

      (with-redefs [config/storage-config (constantly {:type :local :local-path temp-storage})
                    config/storage-type (constantly :local)]
        (object-store/set-store! (local-store/create-local-store))
        (object-store/put-object! vault-id logo-object-key "original-logo" "image/png")

        (let [request (-> (mock/request :get "/favicon.ico")
                          (assoc :headers {"host" domain}))
              response (app/serve-favicon request)]
          (is (= 302 (:status response)))
          (is (str/includes? (get-in response [:headers "Cache-Control"]) "max-age=300"))))))

  (testing "Return 404 for vault without logo"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org No Favicon")
          vault-id (utils/generate-uuid)
          domain "no-favicon-test.com"
          _ (db/create-vault! vault-id tenant-id "No Favicon Test" domain (utils/generate-uuid))]

      (let [request (-> (mock/request :get "/favicon.ico?v=1")
                        (assoc :headers {"host" domain}))
            response (app/serve-favicon request)]

        (is (= 404 (:status response))))))

  (testing "Return 403 for unknown domain"
    (let [request (-> (mock/request :get "/favicon.ico?v=1")
                      (assoc :headers {"host" "unknown-favicon-domain.com"}))
          response (app/serve-favicon request)]

      (is (= 403 (:status response))))))

;; ============================================================
;; HTMX Response Contract Tests
;; ============================================================

(deftest test-get-note-htmx-response-contract
  (testing "HTMX request returns note fragment with HX-Push-Url"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "HTMX Org")
          vault-id (utils/generate-uuid)
          domain "htmx-test.com"
          _ (db/create-vault! vault-id tenant-id "HTMX Vault" domain (utils/generate-uuid))
          note-a "note-a"
          note-b "note-b"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "a.md" note-a
                             "# Note A" nil "hash-a" "2024-01-01T00:00:00Z")
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "b.md" note-b
                             "# Note B" nil "hash-b" "2024-01-01T00:00:00Z")
          request (-> (mock/request :get (str "/" note-b))
                      (assoc :headers {"host" domain
                                       "hx-request" "true"
                                       "x-from-note-id" note-a
                                       "hx-current-url" (str "http://" domain "/" note-a)})
                      (assoc :path-params {:path note-b}))
          response (app/get-note request)]
      (is (= 200 (:status response)))
      (is (= "/note-a+note-b" (get-in response [:headers "HX-Push-Url"])))
      (is (str/includes? (get-in response [:headers "Content-Type"]) "text/html"))
      (is (str/includes? (:body response) note-b))))

  (testing "Non-HTMX request returns full page without HX headers"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Page Org")
          vault-id (utils/generate-uuid)
          domain "page-test.com"
          _ (db/create-vault! vault-id tenant-id "Page Vault" domain (utils/generate-uuid))
          note-a "note-a"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "a.md" note-a
                             "# Note A" nil "hash-a2" "2024-01-01T00:00:00Z")
          request (-> (mock/request :get (str "/" note-a))
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path note-a}))
          response (app/get-note request)]
      (is (= 200 (:status response)))
      (is (nil? (get-in response [:headers "HX-Push-Url"])))
      (is (str/includes? (get-in response [:headers "Content-Type"]) "text/html"))
      (is (str/includes? (:body response) note-a))))

  (testing "Invalid stacked path returns corrected HX-Replace-Url on full page response"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Correction Org")
          vault-id (utils/generate-uuid)
          domain "correction-test.com"
          _ (db/create-vault! vault-id tenant-id "Correction Vault" domain (utils/generate-uuid))
          note-a "note-a"
          _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "a.md" note-a
                             "# Note A" nil "hash-a3" "2024-01-01T00:00:00Z")
          request (-> (mock/request :get "/note-a+missing")
                      (assoc :headers {"host" domain})
                      (assoc :path-params {:path "note-a+missing"}))
          response (app/get-note request)]
      (is (= 200 (:status response)))
      (is (= "/note-a" (get-in response [:headers "HX-Replace-Url"])))
      (is (str/includes? (:body response) note-a)))))
