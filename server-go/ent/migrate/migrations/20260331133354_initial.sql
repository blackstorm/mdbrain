-- Create "assets" table
CREATE TABLE `assets` (`id` text NOT NULL, `client_id` text NOT NULL, `path` text NOT NULL, `object_key` text NOT NULL, `size_bytes` integer NOT NULL, `content_type` text NOT NULL, `md5` text NOT NULL, `deleted_at` integer NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `updated_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `tenant_id` text NOT NULL, `vault_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `assets_tenants_assets` FOREIGN KEY (`tenant_id`) REFERENCES `tenants` (`id`) ON DELETE NO ACTION, CONSTRAINT `assets_vaults_assets` FOREIGN KEY (`vault_id`) REFERENCES `vaults` (`id`) ON DELETE CASCADE);
-- Create index "asset_vault_id_client_id" to table: "assets"
CREATE UNIQUE INDEX `asset_vault_id_client_id` ON `assets` (`vault_id`, `client_id`);
-- Create index "asset_vault_id" to table: "assets"
CREATE INDEX `asset_vault_id` ON `assets` (`vault_id`);
-- Create index "asset_vault_id_path" to table: "assets"
CREATE INDEX `asset_vault_id_path` ON `assets` (`vault_id`, `path`);
-- Create index "asset_vault_id_deleted_at" to table: "assets"
CREATE INDEX `asset_vault_id_deleted_at` ON `assets` (`vault_id`, `deleted_at`);
-- Create index "asset_vault_id_md5" to table: "assets"
CREATE INDEX `asset_vault_id_md5` ON `assets` (`vault_id`, `md5`);
-- Create "notes" table
CREATE TABLE `notes` (`id` text NOT NULL, `path` text NOT NULL, `client_id` text NOT NULL, `content` text NULL, `metadata` text NULL, `hash` text NULL, `mtime` text NULL, `deleted_at` integer NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `updated_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `tenant_id` text NOT NULL, `vault_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `notes_tenants_notes` FOREIGN KEY (`tenant_id`) REFERENCES `tenants` (`id`) ON DELETE NO ACTION, CONSTRAINT `notes_vaults_notes` FOREIGN KEY (`vault_id`) REFERENCES `vaults` (`id`) ON DELETE CASCADE);
-- Create index "note_vault_id_client_id" to table: "notes"
CREATE UNIQUE INDEX `note_vault_id_client_id` ON `notes` (`vault_id`, `client_id`);
-- Create index "note_vault_id" to table: "notes"
CREATE INDEX `note_vault_id` ON `notes` (`vault_id`);
-- Create index "note_vault_id_path" to table: "notes"
CREATE INDEX `note_vault_id_path` ON `notes` (`vault_id`, `path`);
-- Create index "note_mtime" to table: "notes"
CREATE INDEX `note_mtime` ON `notes` (`mtime`);
-- Create "note_asset_refs" table
CREATE TABLE `note_asset_refs` (`id` text NOT NULL, `note_client_id` text NOT NULL, `asset_client_id` text NOT NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `vault_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `note_asset_refs_vaults_note_asset_refs` FOREIGN KEY (`vault_id`) REFERENCES `vaults` (`id`) ON DELETE CASCADE);
-- Create index "noteassetref_vault_id_note_client_id_asset_client_id" to table: "note_asset_refs"
CREATE UNIQUE INDEX `noteassetref_vault_id_note_client_id_asset_client_id` ON `note_asset_refs` (`vault_id`, `note_client_id`, `asset_client_id`);
-- Create index "noteassetref_vault_id_note_client_id" to table: "note_asset_refs"
CREATE INDEX `noteassetref_vault_id_note_client_id` ON `note_asset_refs` (`vault_id`, `note_client_id`);
-- Create index "noteassetref_vault_id_asset_client_id" to table: "note_asset_refs"
CREATE INDEX `noteassetref_vault_id_asset_client_id` ON `note_asset_refs` (`vault_id`, `asset_client_id`);
-- Create "note_links" table
CREATE TABLE `note_links` (`id` text NOT NULL, `source_client_id` text NOT NULL, `target_client_id` text NOT NULL, `target_path` text NULL, `link_type` text NOT NULL, `display_text` text NULL, `original` text NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `updated_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `vault_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `note_links_vaults_note_links` FOREIGN KEY (`vault_id`) REFERENCES `vaults` (`id`) ON DELETE CASCADE);
-- Create index "notelink_vault_id_source_client_id" to table: "note_links"
CREATE INDEX `notelink_vault_id_source_client_id` ON `note_links` (`vault_id`, `source_client_id`);
-- Create index "notelink_vault_id_target_client_id" to table: "note_links"
CREATE INDEX `notelink_vault_id_target_client_id` ON `note_links` (`vault_id`, `target_client_id`);
-- Create index "notelink_vault_id_source_client_id_target_client_id" to table: "note_links"
CREATE INDEX `notelink_vault_id_source_client_id_target_client_id` ON `note_links` (`vault_id`, `source_client_id`, `target_client_id`);
-- Create "tenants" table
CREATE TABLE `tenants` (`id` text NOT NULL, `name` text NOT NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), PRIMARY KEY (`id`));
-- Create "users" table
CREATE TABLE `users` (`id` text NOT NULL, `username` text NOT NULL, `password_hash` text NOT NULL, `role` text NOT NULL DEFAULT ('admin'), `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `tenant_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `users_tenants_users` FOREIGN KEY (`tenant_id`) REFERENCES `tenants` (`id`) ON DELETE NO ACTION);
-- Create index "users_username_key" to table: "users"
CREATE UNIQUE INDEX `users_username_key` ON `users` (`username`);
-- Create "vaults" table
CREATE TABLE `vaults` (`id` text NOT NULL, `name` text NOT NULL, `domain` text NULL, `sync_key` text NOT NULL, `client_type` text NOT NULL DEFAULT ('obsidian'), `root_note_id` text NULL, `logo_object_key` text NULL, `custom_head_html` text NULL, `last_publish_at` datetime NULL, `last_publish_status` text NOT NULL DEFAULT ('never'), `last_publish_error_code` text NULL, `last_publish_error_message` text NULL, `created_at` datetime NOT NULL DEFAULT (CURRENT_TIMESTAMP), `tenant_id` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `vaults_tenants_vaults` FOREIGN KEY (`tenant_id`) REFERENCES `tenants` (`id`) ON DELETE NO ACTION);
-- Create index "vault_domain" to table: "vaults"
CREATE UNIQUE INDEX `vault_domain` ON `vaults` (`domain`);
-- Create index "vault_sync_key" to table: "vaults"
CREATE UNIQUE INDEX `vault_sync_key` ON `vaults` (`sync_key`);
