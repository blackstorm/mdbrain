(ns mdbrain.asset-manifest
  "Helpers for extracting asset references from templates and generating
   fingerprinted asset paths.")

(def asset-url-pattern
  #"\{\{\s*(['\"])(/publics/[^'\"]+)\1\s*\|\s*asset_url\s*\}\}")

(defn extract-asset-paths
  "Extract literal /publics paths used with the `asset_url` filter from template text.
   Preserves order and duplicates."
  [template-content]
  (->> (re-seq asset-url-pattern (or template-content ""))
       (map #(nth % 2))
       vec))

(defn fingerprinted-path
  "Insert hash before file extension.
   /publics/app.css + abc -> /publics/app.abc.css"
  [asset-path hash-value]
  (let [path (str asset-path)
        hash-value (str hash-value)
        last-slash (.lastIndexOf path "/")
        dir (if (neg? last-slash) "" (subs path 0 (inc last-slash)))
        filename (if (neg? last-slash) path (subs path (inc last-slash)))
        last-dot (.lastIndexOf filename ".")]
    ;; Leading-dot filenames like `.well-known` should be treated as extensionless.
    (if (pos? last-dot)
      (str dir
           (subs filename 0 last-dot)
           "."
           hash-value
           (subs filename last-dot))
      (str dir filename "." hash-value))))
