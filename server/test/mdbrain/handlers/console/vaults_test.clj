(ns mdbrain.handlers.console.vaults-test
  "Tests for vault CRUD handlers."
  (:require
   [clojure.string :as str]
   [clojure.test :refer [deftest is testing use-fixtures]]
   [mdbrain.db :as db]
   [mdbrain.handlers.console.test-utils :as test-utils :refer [authenticated-request]]
   [mdbrain.handlers.console.vaults :as vaults]
   [mdbrain.object-store :as object-store]
   [mdbrain.utils :as utils]))

(use-fixtures :each test-utils/setup-test-db)

(deftest test-list-vaults-with-data
  (testing "List vaults for authenticated user returns HTML"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "list-vaults-console" "hash")
          vault-id-1 (utils/generate-uuid)
          vault-id-2 (utils/generate-uuid)
          _ (db/create-vault! vault-id-1 tenant-id "Blog 1" "blog1.com" (utils/generate-uuid))
          _ (db/create-vault! vault-id-2 tenant-id "Blog 2" "blog2.com" (utils/generate-uuid))
          request (authenticated-request :get "/api/console/vaults" tenant-id user-id)
          response (vaults/list-vaults request)]
      (is (= 200 (:status response)))
      (is (string? (:body response)))
      (is (str/includes? (:body response) "Blog 1"))
      (is (str/includes? (:body response) "Blog 2")))))

(deftest test-list-vaults-empty
  (testing "Empty vault list returns HTML with empty state"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "empty-vaults-console" "hash")
          request (authenticated-request :get "/api/console/vaults" tenant-id user-id)
          response (vaults/list-vaults request)]
      (is (= 200 (:status response)))
      (is (string? (:body response)))
      (is (str/includes? (:body response) "No sites yet")))))

(deftest test-list-vaults-uses-dashboard-data-batch-query
  (testing "List vaults uses batch dashboard data and avoids per-vault queries"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "batch-query-console" "hash")
          request (authenticated-request :get "/api/console/vaults" tenant-id user-id)
          dashboard-calls (atom 0)
          legacy-note-search-calls (atom 0)
          legacy-storage-calls (atom 0)]
      (with-redefs [db/list-vaults-dashboard-data (fn [_]
                                                    (swap! dashboard-calls inc)
                                                    [{:id (utils/generate-uuid)
                                                      :name "Batch Blog"
                                                      :domain "batch.com"
                                                      :sync-key "12345678-1234-1234-1234-123456789012"
                                                      :last-publish-status "never"
                                                      :notes []
                                                      :storage-bytes 0}])
                    db/search-notes-by-vault (fn [& _]
                                               (swap! legacy-note-search-calls inc)
                                               [])
                    db/get-vault-storage-size (fn [& _]
                                                (swap! legacy-storage-calls inc)
                                                0)]
        (let [response (vaults/list-vaults request)]
          (is (= 200 (:status response)))
          (is (string? (:body response)))
          (is (str/includes? (:body response) "Batch Blog"))
          (is (= 1 @dashboard-calls))
          (is (= 0 @legacy-note-search-calls))
          (is (= 0 @legacy-storage-calls)))))))

(deftest test-create-vault-success
  (testing "Create vault successfully"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "create-vault-console" "hash")
          request (authenticated-request :post "/api/console/vaults"
                                         tenant-id user-id
                                         :body {:name "My Blog"
                                                :domain "myblog.com"})
          response (vaults/create-vault request)]
      (is (= 200 (:status response)))
      (is (get-in response [:body :success]))
      (is (string? (get-in response [:body :vault :id])))
      (is (= "My Blog" (get-in response [:body :vault :name])))
      (is (= "myblog.com" (get-in response [:body :vault :domain])))
      (is (string? (get-in response [:body :vault :sync-key]))))))

(deftest test-create-vault-missing-fields
  (testing "Create vault with missing fields"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "missing-fields-console" "hash")
          request (authenticated-request :post "/api/console/vaults"
                                         tenant-id user-id
                                         :body {:name "Blog Only"})
          response (vaults/create-vault request)]
      (is (= 200 (:status response)))
      (is (false? (get-in response [:body :success]))))))

(deftest test-delete-vault-success
  (testing "Delete vault successfully with cascade"
    (with-redefs [object-store/delete-vault-objects! (fn [_] nil)]
      (let [tenant-id (utils/generate-uuid)
            _ (db/create-tenant! tenant-id "Test Org")
            user-id (utils/generate-uuid)
            _ (db/create-user! user-id tenant-id "delete-vault-console" "hash")
            vault-id (utils/generate-uuid)
            sync-key (utils/generate-uuid)
            _ (db/create-vault! vault-id tenant-id "My Blog" "delete-test.com" sync-key)
            _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "note1.md" "c1" "# Note 1" "{}" "h1" "2024-01-01T00:00:00Z")
            _ (db/upsert-note! (utils/generate-uuid) tenant-id vault-id "note2.md" "c2" "# Note 2" "{}" "h2" "2024-01-01T00:00:00Z")
            notes-before (db/list-notes-by-vault vault-id)
            _ (is (= 2 (count notes-before)))
            request (-> (authenticated-request :delete (str "/api/console/vaults/" vault-id)
                                               tenant-id user-id)
                        (assoc :path-params {:id vault-id}))
            response (vaults/delete-vault request)]
        (is (= 200 (:status response)))
        (is (get-in response [:body :success]))
        (is (nil? (db/get-vault-by-id vault-id)))
        (let [notes-after (db/list-notes-by-vault vault-id)]
          (is (= 0 (count notes-after))))))))

(deftest test-delete-vault-not-found
  (testing "Delete non-existent vault returns error"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "notfound-console" "hash")
          request (-> (authenticated-request :delete "/api/console/vaults/nonexistent"
                                             tenant-id user-id)
                      (assoc :path-params {:id "nonexistent"}))
          response (vaults/delete-vault request)]
      (is (= 200 (:status response)))
      (is (false? (get-in response [:body :success])))
      (is (= "Vault not found" (get-in response [:body :error]))))))

(deftest test-delete-vault-wrong-tenant
  (testing "Delete vault from different tenant returns permission denied"
    (with-redefs [object-store/delete-vault-objects! (fn [_] nil)]
      (let [tenant-id-1 (utils/generate-uuid)
            tenant-id-2 (utils/generate-uuid)
            _ (db/create-tenant! tenant-id-1 "Org 1")
            _ (db/create-tenant! tenant-id-2 "Org 2")
            user-id-1 (utils/generate-uuid)
            user-id-2 (utils/generate-uuid)
            _ (db/create-user! user-id-1 tenant-id-1 "tenant1-console" "hash")
            _ (db/create-user! user-id-2 tenant-id-2 "tenant2-console" "hash")
            vault-id (utils/generate-uuid)
            _ (db/create-vault! vault-id tenant-id-1 "Tenant1 Vault" "tenant1.com" (utils/generate-uuid))
            request (-> (authenticated-request :delete (str "/api/console/vaults/" vault-id)
                                               tenant-id-2 user-id-2)
                        (assoc :path-params {:id vault-id}))
            response (vaults/delete-vault request)]
        (is (= 200 (:status response)))
        (is (false? (get-in response [:body :success])))
        (is (= "Permission denied" (get-in response [:body :error])))))))

(deftest test-renew-sync-key-success
  (testing "Renew sync key successfully"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "renew-console" "hash")
          vault-id (utils/generate-uuid)
          old-sync-key (utils/generate-uuid)
          _ (db/create-vault! vault-id tenant-id "My Blog" "renew-test.com" old-sync-key)
          request (-> (authenticated-request :post (str "/api/console/vaults/" vault-id "/renew-sync-key")
                                             tenant-id user-id)
                      (assoc :path-params {:id vault-id}))
          response (vaults/renew-vault-sync-key request)]
      (is (= 200 (:status response)))
      (is (get-in response [:body :success]))
      (is (string? (get-in response [:body :sync-key])))
      (is (not= old-sync-key (get-in response [:body :sync-key])))
      (let [vault-after (db/get-vault-by-id vault-id)]
        (is (= (get-in response [:body :sync-key]) (:sync-key vault-after)))
        (is (not= old-sync-key (:sync-key vault-after)))))))

(deftest test-renew-sync-key-not-found
  (testing "Renew sync key for non-existent vault"
    (let [tenant-id (utils/generate-uuid)
          _ (db/create-tenant! tenant-id "Test Org")
          user-id (utils/generate-uuid)
          _ (db/create-user! user-id tenant-id "notfound-console" "hash")
          request (-> (authenticated-request :post "/api/console/vaults/nonexistent/renew-sync-key"
                                             tenant-id user-id)
                      (assoc :path-params {:id "nonexistent"}))
          response (vaults/renew-vault-sync-key request)]
      (is (= 200 (:status response)))
      (is (false? (get-in response [:body :success])))
      (is (= "Vault not found" (get-in response [:body :error]))))))

(deftest test-renew-sync-key-wrong-tenant
  (testing "Renew sync key from different tenant returns permission denied"
    (let [tenant-id-1 (utils/generate-uuid)
          tenant-id-2 (utils/generate-uuid)
          _ (db/create-tenant! tenant-id-1 "Org 1")
          _ (db/create-tenant! tenant-id-2 "Org 2")
          user-id-1 (utils/generate-uuid)
          user-id-2 (utils/generate-uuid)
          _ (db/create-user! user-id-1 tenant-id-1 "tenant1-console" "hash")
          _ (db/create-user! user-id-2 tenant-id-2 "tenant2-console" "hash")
          vault-id (utils/generate-uuid)
          old-sync-key (utils/generate-uuid)
          _ (db/create-vault! vault-id tenant-id-1 "Tenant1 Vault" "tenant1.com" old-sync-key)
          request (-> (authenticated-request :post (str "/api/console/vaults/" vault-id "/renew-sync-key")
                                             tenant-id-2 user-id-2)
                      (assoc :path-params {:id vault-id}))
          response (vaults/renew-vault-sync-key request)]
      (is (= 200 (:status response)))
      (is (false? (get-in response [:body :success])))
      (is (= "Permission denied" (get-in response [:body :error])))
      (let [vault-after (db/get-vault-by-id vault-id)]
        (is (= old-sync-key (:sync-key vault-after)))))))
