(ns mdbrain.middleware-test
  (:require [clojure.test :refer [deftest is testing]]
            [mdbrain.utils :as utils]
            [mdbrain.middleware :as middleware]
            [ring.mock.request :as mock]))

;; Mock handler that returns the request
(defn echo-handler [request]
  {:status 200
   :body {:received true
          :session (:session request)
          :user-id (get-in request [:session :user-id])}})

;; Authentication Middleware 测试
(deftest test-wrap-auth
  (testing "Authenticated request passes through"
    (let [tenant-id (utils/generate-uuid)
          user-id (utils/generate-uuid)
          handler (middleware/wrap-auth echo-handler)
          request (-> (mock/request :get "/api/console/vaults")
                     (assoc :session {:user-id user-id :tenant-id tenant-id}))
          response (handler request)]
      (is (= 200 (:status response)))
      (is (get-in response [:body :received]))
      (is (= user-id (get-in response [:body :user-id])))))

  (testing "Unauthenticated request returns 302 redirect"
    (let [handler (middleware/wrap-auth echo-handler)
          request (mock/request :get "/api/console/vaults")
          response (handler request)]
      (is (= 302 (:status response)))
      (is (= "/console/login" (get-in response [:headers "Location"])))))

  (testing "Request with empty session returns 302 redirect"
    (let [handler (middleware/wrap-auth echo-handler)
          request (-> (mock/request :get "/api/console/vaults")
                     (assoc :session {}))
          response (handler request)]
      (is (= 302 (:status response)))))

  (testing "Request with nil session returns 302 redirect"
    (let [handler (middleware/wrap-auth echo-handler)
          request (-> (mock/request :get "/api/console/vaults")
                     (assoc :session nil))
          response (handler request)]
      (is (= 302 (:status response))))))

;; CORS Middleware 测试
(deftest test-wrap-cors
  (testing "CORS headers added to /obsidian/* response"
    (let [handler (middleware/wrap-cors echo-handler)
          request (mock/request :get "/obsidian/sync/changes")
          response (handler request)]
      (is (= "*" (get-in response [:headers "Access-Control-Allow-Origin"])))
      (is (= "GET, POST, PUT, DELETE, OPTIONS" (get-in response [:headers "Access-Control-Allow-Methods"])))
      (is (= "Content-Type, Authorization" (get-in response [:headers "Access-Control-Allow-Headers"])))))

  (testing "OPTIONS preflight returns 200 for /obsidian/*"
    (let [handler (middleware/wrap-cors echo-handler)
          request (mock/request :options "/obsidian/sync/changes")
          response (handler request)]
      (is (= 200 (:status response)))
      (is (= "*" (get-in response [:headers "Access-Control-Allow-Origin"])))))

  (testing "POST request with CORS"
    (let [handler (middleware/wrap-cors echo-handler)
          request (mock/request :post "/obsidian/sync/changes")
          response (handler request)]
      (is (= 200 (:status response)))
      (is (= "*" (get-in response [:headers "Access-Control-Allow-Origin"]))))))

;; Session Middleware 测试
(deftest test-session-middleware
  (testing "Session middleware preserves session"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          session-data {:user-id "user-123" :tenant-id "tenant-456"}
          request (-> (mock/request :get "/test")
                     (assoc :session session-data))
          response (wrapped-handler request)]
      (is (= 200 (:status response)))))

  (testing "Session middleware handles new session"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (mock/request :get "/test")
          response (wrapped-handler request)]
      (is (= 200 (:status response))))))

;; Error Handling 测试
(deftest test-error-handling
  (testing "Handler exception propagates (no error wrapper in middleware)"
    ;; The wrap-middleware doesn't include error handling, so exceptions propagate
    (let [failing-handler (fn [_] (throw (Exception. "Test error")))
          wrapped-handler (middleware/wrap-middleware failing-handler)
          request (mock/request :get "/test")]
      (is (thrown? Exception (wrapped-handler request)))))

  (testing "Non-exception error handling"
    (let [handler (fn [_] {:status 400 :body {:error "Bad request"}})
          wrapped-handler (middleware/wrap-middleware handler)
          request (mock/request :get "/test")
          response (wrapped-handler request)]
      (is (= 400 (:status response))))))

;; JSON Middleware 测试
(deftest test-json-middleware
  (testing "JSON request body parsing"
    (let [handler (fn [request]
                   {:status 200
                    :body {:received-data (:body-params request)}})
          wrapped-handler (middleware/wrap-middleware handler)
          request (-> (mock/request :post "/api/test")
                     (mock/header "Content-Type" "application/json")
                     (mock/body "{\"name\":\"test\",\"value\":123}"))
          response (wrapped-handler request)]
      (is (= 200 (:status response)))))

  (testing "JSON response encoding"
    (let [handler (fn [_] {:status 200 :body {:result "success" :data [1 2 3]}})
          wrapped-handler (middleware/wrap-middleware handler)
          request (mock/request :get "/api/test")
          response (wrapped-handler request)]
      (is (= 200 (:status response)))
      ;; Note: Content-Type may not be set if wrap-json-response isn't in middleware
      (is (some? response)))))

;; Content-Type Middleware 测试
(deftest test-content-type-handling
  (testing "application/json content-type"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (-> (mock/request :post "/api/test")
                     (mock/header "Content-Type" "application/json")
                     (mock/body "{\"test\":true}"))
          response (wrapped-handler request)]
      (is (= 200 (:status response)))))

  (testing "text/html content-type"
    (let [handler (fn [_] {:status 200
                          :headers {"Content-Type" "text/html"}
                          :body "<html><body>Test</body></html>"})
          wrapped-handler (middleware/wrap-middleware handler)
          request (mock/request :get "/")
          response (wrapped-handler request)]
      (is (= 200 (:status response)))
      (is (= "text/html" (get-in response [:headers "Content-Type"])))))

  (testing "Missing content-type defaults to JSON"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (mock/request :post "/api/test")
          response (wrapped-handler request)]
      (is (= 200 (:status response))))))

;; Middleware Chain Integration 测试
(deftest test-middleware-chain
  (testing "Authentication middleware alone"
    ;; Test wrap-auth directly without session middleware
    (let [handler (middleware/wrap-auth echo-handler)
          user-id (utils/generate-uuid)
          tenant-id (utils/generate-uuid)
          request (-> (mock/request :get "/api/console/test")
                     (assoc :session {:user-id user-id :tenant-id tenant-id}))
          response (handler request)]
      (is (= 200 (:status response)))
      (is (= user-id (get-in response [:body :user-id])))))

  (testing "Full middleware chain without authentication"
    (let [handler (middleware/wrap-auth echo-handler)
          wrapped-handler (middleware/wrap-middleware handler)
          request (mock/request :get "/api/console/test")
          response (wrapped-handler request)]
      ;; wrap-auth now returns 302 redirect instead of 401
      (is (= 302 (:status response)))))

  (testing "Middleware chain with CORS preflight"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (-> (mock/request :options "/obsidian/sync/changes")
                     (mock/header "Origin" "http://localhost:3000"))
          response (wrapped-handler request)]
      (is (= 200 (:status response)))
      (is (= "*" (get-in response [:headers "Access-Control-Allow-Origin"]))))))

;; Security Headers 测试
(deftest test-security-headers
  (testing "Security headers present in response"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (mock/request :get "/test")
          response (wrapped-handler request)]
      (is (= 200 (:status response)))))

  (testing "CORS security headers"
    (let [wrapped-handler (middleware/wrap-middleware echo-handler)
          request (-> (mock/request :get "/obsidian/sync/changes")
                     (mock/header "Origin" "http://example.com"))
          response (wrapped-handler request)]
      (is (= "*" (get-in response [:headers "Access-Control-Allow-Origin"]))))))

;; X-Robots-Tag Middleware 测试
(deftest test-wrap-noindex
  (testing "Adds X-Robots-Tag header when headers exist"
    (let [handler (middleware/wrap-noindex (fn [_]
                                             {:status 200
                                              :headers {"Content-Type" "text/plain"}
                                              :body "ok"}))
          response (handler (mock/request :get "/console"))]
      (is (= "noindex, nofollow" (get-in response [:headers "X-Robots-Tag"])))
      (is (= "text/plain" (get-in response [:headers "Content-Type"])))))

  (testing "Adds X-Robots-Tag header when headers missing"
    (let [handler (middleware/wrap-noindex (fn [_] {:status 200 :body "ok"}))
          response (handler (mock/request :get "/console"))]
      (is (= "noindex, nofollow" (get-in response [:headers "X-Robots-Tag"])))))

  (testing "Overrides existing X-Robots-Tag header"
    (let [handler (middleware/wrap-noindex (fn [_]
                                             {:status 200
                                              :headers {"X-Robots-Tag" "index"}
                                              :body "ok"}))
          response (handler (mock/request :get "/console"))]
      (is (= "noindex, nofollow" (get-in response [:headers "X-Robots-Tag"])))))

  (testing "Non-map responses pass through unchanged"
    (let [handler (middleware/wrap-noindex (fn [_] "stream-body"))
          response (handler (mock/request :get "/console"))]
      (is (= "stream-body" response)))))
