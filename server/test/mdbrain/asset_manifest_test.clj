(ns mdbrain.asset-manifest-test
  (:require
   [clojure.test :refer [deftest is testing]]
   [mdbrain.asset-manifest :as asset-manifest]))

(deftest test-fingerprinted-path
  (testing "Adds hash before extension"
    (is (= "/publics/app/css/app.abc123def456.css"
           (asset-manifest/fingerprinted-path "/publics/app/css/app.css" "abc123def456"))))

  (testing "Handles file names with multiple dots"
    (is (= "/publics/app/js/highlight.min.abc123def456.js"
           (asset-manifest/fingerprinted-path "/publics/app/js/highlight.min.js" "abc123def456"))))

  (testing "Handles file names without extension"
    (is (= "/publics/shared/robots.abc123def456"
           (asset-manifest/fingerprinted-path "/publics/shared/robots" "abc123def456"))))

  (testing "Handles leading-dot filenames"
    (is (= "/publics/shared/.well-known.abc123def456"
           (asset-manifest/fingerprinted-path "/publics/shared/.well-known" "abc123def456")))))

(deftest test-extract-asset-paths
  (let [template "<link href='{{ \"/publics/app/css/app.css\"|asset_url }}'>\n<script src='{{ '/publics/shared/htmx.min.js'|asset_url }}'></script>\n<link href='{{ \"/publics/app/css/app.css\"|asset_url }}'>\n{{ dynamic_path|asset_url }}\n{{ \"/publics/ignored/by-other-filter\"|safe }}\n{{ \"/publics/app/js/app.js\"  |  asset_url  }}"]
    (testing "Extracts asset_url literal paths from template content"
      (is (= ["/publics/app/css/app.css"
              "/publics/shared/htmx.min.js"
              "/publics/app/css/app.css"
              "/publics/app/js/app.js"]
             (asset-manifest/extract-asset-paths template))))))
