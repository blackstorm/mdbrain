(ns mdbrain.view.htmx
  "HTMX request/response helpers.")

(defn is-htmx-request?
  [request]
  (boolean (get-in request [:headers "hx-request"])))

(defn with-push-url
  [response push-url]
  (update response :headers (fnil assoc {}) "HX-Push-Url" push-url))

(defn with-replace-url
  [response replace-url]
  (update response :headers (fnil assoc {}) "HX-Replace-Url" replace-url))
