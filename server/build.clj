(ns build
  (:require [clojure.data.json :as json]
            [clojure.java.io :as io]
            [clojure.string :as str]
            [clojure.tools.build.api :as b]
            [mdbrain.asset-manifest :as asset-manifest])
  (:import [java.io File InputStream]
           [java.security MessageDigest]))

(def lib 'com.mdbrain/server)
(def version "0.2.0")
(def class-dir "target/classes")
(def basis (b/create-basis {:project "deps.edn"}))
(def uber-file (format "target/%s-standalone.jar" (name lib)))

(def generated-resources-dir "target/generated-resources")
(def generated-publics-dir (str generated-resources-dir "/publics"))
(def asset-manifest-file (str generated-publics-dir "/asset-manifest.json"))

(defn clean [_]
  (b/delete {:path "target"}))

(defn- bytes->hex
  [^bytes bytes]
  (let [hex-chars "0123456789abcdef"
        sb (StringBuilder. (* 2 (alength bytes)))]
    (dotimes [i (alength bytes)]
      (let [v (bit-and 0xff (aget bytes i))]
        (.append sb (.charAt hex-chars (bit-shift-right v 4)))
        (.append sb (.charAt hex-chars (bit-and v 0x0f)))))
    (.toString sb)))

(defn- short-sha256-file
  [^File file]
  (let [digest (MessageDigest/getInstance "SHA-256")
        buffer (byte-array 8192)]
    (with-open [^InputStream in (io/input-stream file)]
      (loop []
        (let [read-count (.read in buffer)]
          (when (pos? read-count)
            (.update digest buffer 0 read-count)
            (recur)))))
    (subs (bytes->hex (.digest digest)) 0 12)))

(defn- template-files
  []
  (let [root (io/file "resources/templates")]
    (if (.exists root)
      (->> (file-seq root)
           (filter #(.isFile ^File %))
           (filter #(str/ends-with? (.getName ^File %) ".html"))
           (sort-by #(.getPath ^File %)))
      [])))

(defn- asset-paths-from-templates
  []
  (->> (template-files)
       (mapcat (comp asset-manifest/extract-asset-paths slurp))
       distinct
       sort))

(defn- source-file-for-asset
  [asset-path]
  (io/file "resources" (subs asset-path 1)))

(defn- target-file-for-asset
  [asset-path]
  (io/file generated-resources-dir (subs asset-path 1)))

(defn- copy-fingerprinted-asset!
  [asset-path]
  (let [source-file (source-file-for-asset asset-path)]
    (when-not (.exists source-file)
      (throw (ex-info "Asset referenced by template does not exist"
                      {:asset-path asset-path
                       :source-file (.getPath source-file)})))
    (let [hash-value (short-sha256-file source-file)
          fingerprinted (asset-manifest/fingerprinted-path asset-path hash-value)
          target-file (target-file-for-asset fingerprinted)]
      (when-let [parent (.getParentFile ^File target-file)]
        (.mkdirs ^File parent))
      (io/copy source-file target-file)
      [asset-path fingerprinted])))

(defn- generate-asset-manifest!
  []
  (let [asset-paths (asset-paths-from-templates)
        manifest (->> asset-paths
                      (map copy-fingerprinted-asset!)
                      (into (sorted-map)))]
    (.mkdirs ^File (io/file generated-publics-dir))
    (spit asset-manifest-file (json/write-str manifest))
    (println "Generated asset manifest entries:" (count manifest))
    manifest))

(defn uberjar [_]
  (clean nil)
  (generate-asset-manifest!)
  (b/copy-dir {:src-dirs ["src" "resources" generated-resources-dir]
               :target-dir class-dir})
  (b/compile-clj {:basis basis
                  :ns-compile '[mdbrain.core]
                  :class-dir class-dir})
  (b/uber {:class-dir class-dir
           :uber-file uber-file
           :basis basis
           :main 'mdbrain.core}))
