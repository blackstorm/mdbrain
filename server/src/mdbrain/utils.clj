(ns mdbrain.utils
  (:require
   [mdbrain.utils.auth :as auth]
   [mdbrain.utils.crypto :as crypto]
   [mdbrain.view.htmx :as view.htmx]
   [mdbrain.utils.id :as id]
   [mdbrain.utils.paths :as paths]))

;; Backward-compatible facade; implementations live under mdbrain.utils.*
(def generate-uuid id/generate-uuid)

(def sha256-hex crypto/sha256-hex)

(def normalize-link-path paths/normalize-link-path)

(def hash-password auth/hash-password)
(def verify-password auth/verify-password)
(def parse-auth-header auth/parse-auth-header)

(def is-htmx? view.htmx/is-htmx-request?)
