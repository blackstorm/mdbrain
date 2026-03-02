(ns mdbrain.templates.console-template-test
  (:require [clojure.test :refer [deftest is testing]]
            [clojure.java.io :as io]
            [clojure.string :as str]))

(deftest test-console-base-template-noindex
  (testing "Console base template includes noindex meta tag"
    (let [resource (io/resource "templates/console/base.html")
          html (slurp resource)]
      (is (some? resource))
      (is (str/includes? html "<meta name=\"robots\" content=\"noindex, nofollow\">")))))

(deftest test-console-page-templates-extend-base
  (testing "All full-page console templates extend console base template"
    (doseq [template ["templates/console/login.html"
                      "templates/console/init.html"
                      "templates/console/vaults.html"]]
      (let [resource (io/resource template)
            html (slurp resource)]
        (is (some? resource))
        (is (str/includes? html "{% extends \"templates/console/base.html\" %}"))))))
