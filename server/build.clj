(ns build
  (:require [clojure.java.io :as io]
            [clojure.java.shell :as shell]
            [clojure.string :as str]
            [clojure.tools.build.api :as b]))

(def lib 'com.mdbrain/server)
(def version "0.2.0")
(def class-dir "target/classes")
(def basis (b/create-basis {:project "deps.edn"}))
(def uber-file (format "target/%s-standalone.jar" (name lib)))

(def generated-resources-dir "target/generated-resources")
(def generated-publics-dir (str generated-resources-dir "/publics"))
(def generated-lucide-dir (str generated-resources-dir "/templates/lucide"))
(def generated-asset-manifest-file (str generated-publics-dir "/asset-manifest.json"))

(defn clean [_]
  (b/delete {:path "target"}))

(defn- run-command!
  [command args]
  (let [{:keys [exit out err]} (apply shell/sh command args)]
    (when (not (str/blank? out))
      (print out))
    (when (not (str/blank? err))
      (binding [*out* *err*]
        (print err)))
    (when-not (zero? exit)
      (throw (ex-info "Command failed"
                      {:command (str/join " " (cons command args))
                       :exit exit
                       :out out
                       :err err}))))
  true)

(defn- build-frontend-resources!
  []
  (run-command! "npm" ["run" "build:template-assets"])
  (doseq [required-path [generated-asset-manifest-file generated-lucide-dir]]
    (when-not (.exists (io/file required-path))
      (throw (ex-info "Missing generated frontend resources"
                      {:path required-path}))))
  true)

(defn uberjar [_]
  (clean nil)
  (build-frontend-resources!)
  (b/copy-dir {:src-dirs ["src" "resources"]
               :target-dir class-dir})
  (b/delete {:path (str class-dir "/templates/lucide")})
  (b/copy-dir {:src-dirs [generated-resources-dir]
               :target-dir class-dir})
  (b/compile-clj {:basis basis
                  :ns-compile '[mdbrain.core]
                  :class-dir class-dir})
  (b/uber {:class-dir class-dir
           :uber-file uber-file
           :basis basis
           :main 'mdbrain.core}))
