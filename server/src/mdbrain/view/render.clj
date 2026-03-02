(ns mdbrain.view.render
  "Shared rendering helpers for Selmer templates."
  (:require
   [mdbrain.response :as resp]
   [mdbrain.template-assets :as template-assets]
   [selmer.parser :as selmer]))

(defn render
  "Render a Selmer template with the provided context map."
  [template context]
  (template-assets/register-filter!)
  (selmer/render-file template context))

(defn page
  "Render a full HTML page response."
  [template context]
  (resp/html (render template context)))

(defn fragment
  "Render an HTML fragment response."
  [template context]
  {:status 200
   :headers {"Content-Type" "text/html; charset=utf-8"}
   :body (render template context)})
