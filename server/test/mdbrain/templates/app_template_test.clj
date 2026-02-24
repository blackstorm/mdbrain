(ns mdbrain.templates.app-template-test
  (:require [clojure.test :refer [deftest is testing]]
            [clojure.java.io :as io]
            [clojure.string :as str]))

(deftest test-app-base-template-highlight-css
  (testing "App base template includes highlight.js CSS"
    (let [resource (io/resource "templates/app/base.html")
          html (slurp resource)]
      (is (some? resource))
      (is (str/includes? html "/publics/app/css/highlight-github-dark.min.css")))))
