(ns mdbrain.core-test
  (:require
   [clojure.test :refer [deftest is testing]]
   [mdbrain.config :as config]
   [mdbrain.core :as core]
   [mdbrain.template-assets :as template-assets]
   [ring.adapter.undertow :as undertow]))

(def ^:private test-config
  {:server {:app {:port 0 :host "127.0.0.1"}
            :console {:port 0 :host "127.0.0.1"}}
   :environment :development})

(defn- stub-config
  [& path]
  (get-in test-config path))

(deftest test-start-app-server-registers-asset-filter
  (testing "start-app-server registers asset_url filter on its own startup path"
    (let [register-count (atom 0)]
      (with-redefs [config/get-config stub-config
                    template-assets/register-filter! (fn []
                                                       (swap! register-count inc)
                                                       true)
                    undertow/run-undertow (fn [_handler _opts]
                                            (Object.))]
        (core/start-app-server)
        (is (= 1 @register-count))))))

(deftest test-start-console-server-registers-asset-filter
  (testing "start-console-server registers asset_url filter on its own startup path"
    (let [register-count (atom 0)]
      (with-redefs [config/get-config stub-config
                    template-assets/register-filter! (fn []
                                                       (swap! register-count inc)
                                                       true)
                    undertow/run-undertow (fn [_handler _opts]
                                            (Object.))]
        (core/start-console-server)
        (is (= 1 @register-count))))))
