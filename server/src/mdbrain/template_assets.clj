(ns mdbrain.template-assets
  "Selmer asset URL filter backed by build-generated asset manifest."
  (:require
   [clojure.data.json :as json]
   [clojure.java.io :as io]
   [clojure.string :as str]
   [clojure.tools.logging :as log]
   [selmer.filters :as filters]))

(def ^:private manifest-resource-path "publics/asset-manifest.json")

(defonce ^:private filter-registered? (atom false))

(defn- normalize-manifest
  [manifest]
  (into {}
        (map (fn [[k v]] [(str k) (str v)]))
        manifest))

(defn load-manifest
  "Load manifest map from classpath resource publics/asset-manifest.json.
   Returns an empty map when missing or malformed."
  []
  (if-let [resource (io/resource manifest-resource-path)]
    (try
      (-> (slurp resource)
          (json/read-str :key-fn identity)
          normalize-manifest)
      (catch Exception e
        (log/warn e "Failed to load asset manifest, falling back to original asset paths")
        {}))
    {}))

(defonce manifest-cache (atom (delay (load-manifest))))

(defn reset-manifest!
  "Force reload of asset manifest. Useful in REPL development."
  []
  (reset! manifest-cache (delay (load-manifest))))

(defn current-manifest
  "Return cached manifest for template filter resolution."
  []
  @@manifest-cache)

(defn- split-base-and-suffix
  [path]
  (let [[_ base query fragment] (re-matches #"^([^?#]*)(\?[^#]*)?(#.*)?$" path)]
    [base (str (or query "") (or fragment ""))]))

(defn resolve-asset-url
  "Resolve logical path using manifest map.
   - Only /publics/* assets are mapped
   - Query string and fragment are preserved"
  [asset-path manifest]
  (let [path (some-> asset-path str)]
    (cond
      (str/blank? path) ""
      (not (str/starts-with? path "/publics/")) path
      :else
      (let [[base suffix] (split-base-and-suffix path)
            resolved (get manifest base base)]
        (str resolved suffix)))))

(defn asset-url
  "Selmer filter for static assets.
   Looks up fingerprinted path in asset manifest."
  [asset-path]
  (resolve-asset-url asset-path (current-manifest)))

(defn register-filter!
  []
  (when (compare-and-set! filter-registered? false true)
    (filters/add-filter! :asset_url asset-url))
  true)
