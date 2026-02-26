(ns mdbrain.template-assets-test
  (:require
   [clojure.test :refer [deftest is testing]]
   [mdbrain.template-assets :as template-assets]))

(deftest test-resolve-asset-url
  (let [manifest {"/publics/app/css/app.css" "/publics/app/css/app.abc123def456.css"
                  "/publics/shared/htmx.min.js" "/publics/shared/htmx.min.111122223333.js"}]
    (testing "Resolves mapped publics path to fingerprinted path"
      (is (= "/publics/app/css/app.abc123def456.css"
             (template-assets/resolve-asset-url "/publics/app/css/app.css" manifest))))

    (testing "Preserves query string while resolving"
      (is (= "/publics/shared/htmx.min.111122223333.js?debug=1"
             (template-assets/resolve-asset-url "/publics/shared/htmx.min.js?debug=1" manifest))))

    (testing "Preserves URL fragment while resolving"
      (is (= "/publics/shared/htmx.min.111122223333.js#section"
             (template-assets/resolve-asset-url "/publics/shared/htmx.min.js#section" manifest))))

    (testing "Preserves query string and fragment while resolving"
      (is (= "/publics/shared/htmx.min.111122223333.js?debug=1#section"
             (template-assets/resolve-asset-url "/publics/shared/htmx.min.js?debug=1#section" manifest))))

    (testing "Falls back to original path when asset is not in manifest"
      (is (= "/publics/app/js/app.js"
             (template-assets/resolve-asset-url "/publics/app/js/app.js" manifest))))

    (testing "Falls back to original path while preserving query and fragment"
      (is (= "/publics/app/js/app.js?lang=en#demo"
             (template-assets/resolve-asset-url "/publics/app/js/app.js?lang=en#demo" manifest))))

    (testing "Non-public paths are unchanged"
      (is (= "/storage/assets/x.png"
             (template-assets/resolve-asset-url "/storage/assets/x.png" manifest))))

    (testing "Nil path returns blank"
      (is (= ""
             (template-assets/resolve-asset-url nil manifest))))

    (testing "Blank path returns blank"
      (is (= ""
             (template-assets/resolve-asset-url "" manifest))))))

(deftest test-asset-url-filter-uses-current-manifest
  (testing "asset-url resolves path using current manifest provider"
    (with-redefs [template-assets/current-manifest (fn []
                                                     {"/publics/app/css/app.css" "/publics/app/css/app.cachedhash.css"})]
      (is (= "/publics/app/css/app.cachedhash.css"
             (template-assets/asset-url "/publics/app/css/app.css"))))))
