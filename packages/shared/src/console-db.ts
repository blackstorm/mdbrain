import { Database } from "bun:sqlite";
import { mkdirSync, readFileSync } from "node:fs";
import { dirname } from "node:path";

export interface TenantRecord {
  id: string;
  name: string;
  created_at: string;
}

export interface UserRecord {
  id: string;
  tenant_id: string;
  username: string;
  password_hash: string;
  role: string;
  created_at: string;
}

export interface VaultRecord {
  id: string;
  tenant_id: string;
  name: string;
  domain: string | null;
  sync_key: string;
  client_type: string;
  root_note_id: string | null;
  logo_object_key: string | null;
  custom_head_html: string | null;
  last_publish_at: string | null;
  last_publish_status: string;
  last_publish_error_code: string | null;
  last_publish_error_message: string | null;
  created_at: string;
}

export interface NoteRecord {
  id: string;
  tenant_id: string;
  vault_id: string;
  path: string;
  client_id: string;
  content: string | null;
  metadata: string | null;
  hash: string | null;
  mtime: string | null;
  deleted_at: number | null;
  created_at: string;
  updated_at: string;
}

export interface AssetRecord {
  id: string;
  tenant_id: string;
  vault_id: string;
  client_id: string;
  path: string;
  object_key: string;
  size_bytes: number;
  content_type: string;
  md5: string;
  deleted_at: number | null;
  created_at: string;
  updated_at: string;
}

export interface NoteLinkRecord {
  id: string;
  vault_id: string;
  source_client_id: string;
  target_client_id: string;
  target_path: string | null;
  link_type: string;
  display_text: string | null;
  original: string | null;
  created_at: string;
  updated_at: string;
}

export interface NoteAssetRefRecord {
  id: string;
  vault_id: string;
  note_client_id: string;
  asset_client_id: string;
  created_at: string;
}

export interface NoteLookupRecord {
  client_id: string;
  path: string;
  content: string | null;
  updated_at: string;
}

export interface AssetLookupRecord {
  client_id: string;
  path: string;
  object_key: string;
  content_type: string;
  public_url?: string;
}

interface DatabaseOptions {
  databasePath: string;
  migrationFile: string;
}

function normalizeSql(sql: string) {
  return sql
    .split(/^--;;\s*$/m)
    .map((statement) => statement.trim())
    .filter(Boolean);
}

export class ConsoleDatabase {
  readonly sqlite: Database;

  constructor(options: DatabaseOptions) {
    mkdirSync(dirname(options.databasePath), { recursive: true });
    this.sqlite = new Database(options.databasePath, { create: true });
    this.sqlite.exec("PRAGMA journal_mode = WAL;");
    this.sqlite.exec("PRAGMA foreign_keys = ON;");

    const migrationSql = readFileSync(options.migrationFile, "utf8");
    for (const statement of normalizeSql(migrationSql)) {
      this.sqlite.exec(statement);
    }
  }

  close() {
    this.sqlite.close(false);
  }

  hasAnyUser() {
    const row = this.sqlite
      .query("SELECT COUNT(*) AS count FROM users")
      .get() as { count: number };
    return row.count > 0;
  }

  getTenant(id: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM tenants WHERE id = ?")
        .get(id) as TenantRecord | null) ?? null
    );
  }

  getUserByUsername(username: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM users WHERE username = ?")
        .get(username) as UserRecord | null) ?? null
    );
  }

  getUserById(id: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM users WHERE id = ?")
        .get(id) as UserRecord | null) ?? null
    );
  }

  createInitialConsoleUser(params: {
    tenantId: string;
    tenantName: string;
    userId: string;
    username: string;
    passwordHash: string;
  }) {
    const transaction = this.sqlite.transaction(
      ({
        tenantId,
        tenantName,
        userId,
        username,
        passwordHash,
      }: typeof params) => {
        this.sqlite
          .query("INSERT INTO tenants (id, name) VALUES (?, ?)")
          .run(tenantId, tenantName);
        this.sqlite
          .query(
            "INSERT INTO users (id, tenant_id, username, password_hash) VALUES (?, ?, ?, ?)",
          )
          .run(userId, tenantId, username, passwordHash);
      },
    );

    transaction(params);
  }

  updateUserPassword(userId: string, passwordHash: string) {
    this.sqlite
      .query("UPDATE users SET password_hash = ? WHERE id = ?")
      .run(passwordHash, userId);
  }

  listVaultsByTenant(tenantId: string) {
    return this.sqlite
      .query("SELECT * FROM vaults WHERE tenant_id = ? ORDER BY created_at")
      .all(tenantId) as VaultRecord[];
  }

  getVaultById(id: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM vaults WHERE id = ?")
        .get(id) as VaultRecord | null) ?? null
    );
  }

  getVaultByDomain(domain: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM vaults WHERE domain = ?")
        .get(domain) as VaultRecord | null) ?? null
    );
  }

  getVaultBySyncKey(syncKey: string) {
    return (
      (this.sqlite
        .query("SELECT * FROM vaults WHERE sync_key = ?")
        .get(syncKey) as VaultRecord | null) ?? null
    );
  }

  createVault(params: {
    id: string;
    tenantId: string;
    name: string;
    domain: string;
    syncKey: string;
  }) {
    this.sqlite
      .query(
        "INSERT INTO vaults (id, tenant_id, name, domain, sync_key) VALUES (?, ?, ?, ?, ?)",
      )
      .run(params.id, params.tenantId, params.name, params.domain, params.syncKey);
  }

  updateVault(vaultId: string, name: string, domain: string) {
    this.sqlite
      .query("UPDATE vaults SET name = ?, domain = ? WHERE id = ?")
      .run(name, domain, vaultId);
  }

  deleteVault(vaultId: string) {
    this.sqlite.query("DELETE FROM vaults WHERE id = ?").run(vaultId);
  }

  updateVaultSyncKey(vaultId: string, syncKey: string) {
    this.sqlite
      .query("UPDATE vaults SET sync_key = ? WHERE id = ?")
      .run(syncKey, vaultId);
  }

  updateVaultRootNote(vaultId: string, rootNoteId: string | null) {
    this.sqlite
      .query("UPDATE vaults SET root_note_id = ? WHERE id = ?")
      .run(rootNoteId, vaultId);
  }

  updateVaultLogo(vaultId: string, logoObjectKey: string | null) {
    this.sqlite
      .query("UPDATE vaults SET logo_object_key = ? WHERE id = ?")
      .run(logoObjectKey, vaultId);
  }

  updateVaultCustomHeadHtml(vaultId: string, customHeadHtml: string | null) {
    this.sqlite
      .query("UPDATE vaults SET custom_head_html = ? WHERE id = ?")
      .run(customHeadHtml, vaultId);
  }

  recordVaultPublishSuccess(vaultId: string) {
    this.sqlite
      .query(
        `UPDATE vaults
         SET last_publish_status = 'ok',
             last_publish_at = CURRENT_TIMESTAMP,
             last_publish_error_code = NULL,
             last_publish_error_message = NULL
         WHERE id = ?`,
      )
      .run(vaultId);
  }

  recordVaultPublishError(vaultId: string, code: string, message: string) {
    this.sqlite
      .query(
        `UPDATE vaults
         SET last_publish_status = 'error',
             last_publish_at = CURRENT_TIMESTAMP,
             last_publish_error_code = ?,
             last_publish_error_message = ?
         WHERE id = ?`,
      )
      .run(code, message, vaultId);
  }

  listNotesByVault(vaultId: string) {
    return this.sqlite
      .query(
        `SELECT client_id, path, hash, mtime
         FROM notes
         WHERE vault_id = ? AND deleted_at IS NULL
         ORDER BY path`,
      )
      .all(vaultId) as Array<Pick<NoteRecord, "client_id" | "path" | "hash" | "mtime">>;
  }

  getNoteByClientId(vaultId: string, clientId: string) {
    return (
      (this.sqlite
        .query(
          `SELECT *
           FROM notes
           WHERE vault_id = ? AND client_id = ? AND deleted_at IS NULL`,
        )
        .get(vaultId, clientId) as NoteRecord | null) ?? null
    );
  }

  listNotesForLookup(vaultId: string) {
    return this.sqlite
      .query(
        `SELECT client_id, path, content, updated_at
         FROM notes
         WHERE vault_id = ? AND deleted_at IS NULL
         ORDER BY path`,
      )
      .all(vaultId) as NoteLookupRecord[];
  }

  searchNotesByVault(vaultId: string, query: string) {
    const normalizedQuery = query.trim();
    const searchPattern = normalizedQuery ? `%${normalizedQuery}%` : "%";

    return this.sqlite
      .query(
        `SELECT client_id, path, hash, mtime
         FROM notes
         WHERE vault_id = ?
           AND deleted_at IS NULL
           AND path LIKE ?
         ORDER BY path`,
      )
      .all(vaultId, searchPattern) as Array<Pick<NoteRecord, "client_id" | "path" | "hash" | "mtime">>;
  }

  upsertNote(params: {
    id: string;
    tenantId: string;
    vaultId: string;
    path: string;
    clientId: string;
    content: string;
    metadata: string | null;
    hash: string;
    mtime?: string | null;
  }) {
    this.sqlite
      .query(
        `INSERT INTO notes (id, tenant_id, vault_id, path, client_id, content, metadata, hash, mtime)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(vault_id, client_id) DO UPDATE SET
           path = excluded.path,
           content = excluded.content,
           metadata = excluded.metadata,
           hash = excluded.hash,
           mtime = excluded.mtime,
           deleted_at = NULL,
           updated_at = CURRENT_TIMESTAMP`,
      )
      .run(
        params.id,
        params.tenantId,
        params.vaultId,
        params.path,
        params.clientId,
        params.content,
        params.metadata,
        params.hash,
        params.mtime ?? null,
      );
  }

  deleteNoteByClientId(vaultId: string, clientId: string) {
    this.sqlite
      .query("DELETE FROM notes WHERE vault_id = ? AND client_id = ?")
      .run(vaultId, clientId);
  }

  deleteNoteLinksBySource(vaultId: string, clientId: string) {
    this.sqlite
      .query("DELETE FROM note_links WHERE vault_id = ? AND source_client_id = ?")
      .run(vaultId, clientId);
  }

  deleteNoteLinksByTarget(vaultId: string, clientId: string) {
    this.sqlite
      .query("DELETE FROM note_links WHERE vault_id = ? AND target_client_id = ?")
      .run(vaultId, clientId);
  }

  replaceNoteLinks(
    vaultId: string,
    sourceClientId: string,
    links: Array<{ id: string; targetClientId: string; targetPath?: string | null }>,
  ) {
    const insertStatement = this.sqlite.prepare(
      `INSERT INTO note_links (
         id,
         vault_id,
         source_client_id,
         target_client_id,
         target_path,
         link_type,
         display_text,
         original
       ) VALUES (?, ?, ?, ?, ?, 'internal', NULL, NULL)`,
    );

    const transaction = this.sqlite.transaction(
      (
        currentVaultId: string,
        currentSourceClientId: string,
        currentLinks: Array<{ id: string; targetClientId: string; targetPath?: string | null }>,
      ) => {
        this.deleteNoteLinksBySource(currentVaultId, currentSourceClientId);
        for (const link of currentLinks) {
          insertStatement.run(
            link.id,
            currentVaultId,
            currentSourceClientId,
            link.targetClientId,
            link.targetPath ?? null,
          );
        }
      },
    );

    transaction(vaultId, sourceClientId, links);
  }

  listNoteLinksBySource(vaultId: string, clientId: string) {
    return this.sqlite
      .query(
        `SELECT *
         FROM note_links
         WHERE vault_id = ? AND source_client_id = ?
         ORDER BY target_client_id`,
      )
      .all(vaultId, clientId) as NoteLinkRecord[];
  }

  listBacklinksWithNotes(vaultId: string, clientId: string) {
    return this.sqlite
      .query(
        `SELECT
           n.client_id,
           n.path,
           n.content,
           n.updated_at
         FROM note_links nl
         JOIN notes n
           ON n.vault_id = nl.vault_id
          AND n.client_id = nl.source_client_id
         WHERE nl.vault_id = ?
           AND nl.target_client_id = ?
           AND n.deleted_at IS NULL
         ORDER BY n.path`,
      )
      .all(vaultId, clientId) as NoteLookupRecord[];
  }

  listAssetsByVault(vaultId: string) {
    return this.sqlite
      .query(
        `SELECT client_id, path, md5, size_bytes
         FROM assets
         WHERE vault_id = ? AND deleted_at IS NULL
         ORDER BY path`,
      )
      .all(vaultId) as Array<Pick<AssetRecord, "client_id" | "path" | "md5" | "size_bytes">>;
  }

  getVaultStorageSize(vaultId: string) {
    const row = this.sqlite
      .query(
        `SELECT COALESCE(SUM(size_bytes), 0) AS total_bytes
         FROM assets
         WHERE vault_id = ? AND deleted_at IS NULL`,
      )
      .get(vaultId) as { total_bytes: number };

    return row.total_bytes;
  }

  listAssetsForLookup(vaultId: string) {
    return this.sqlite
      .query(
        `SELECT client_id, path, object_key, content_type
         FROM assets
         WHERE vault_id = ? AND deleted_at IS NULL
         ORDER BY path`,
      )
      .all(vaultId) as AssetLookupRecord[];
  }

  getAssetByClientId(vaultId: string, clientId: string) {
    return (
      (this.sqlite
        .query(
          `SELECT *
           FROM assets
           WHERE vault_id = ? AND client_id = ? AND deleted_at IS NULL`,
        )
        .get(vaultId, clientId) as AssetRecord | null) ?? null
    );
  }

  upsertAsset(params: {
    id: string;
    tenantId: string;
    vaultId: string;
    clientId: string;
    path: string;
    objectKey: string;
    sizeBytes: number;
    contentType: string;
    md5: string;
  }) {
    this.sqlite
      .query(
        `INSERT INTO assets (
           id,
           tenant_id,
           vault_id,
           client_id,
           path,
           object_key,
           size_bytes,
           content_type,
           md5,
           deleted_at
         )
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
         ON CONFLICT(vault_id, client_id) DO UPDATE SET
           path = excluded.path,
           object_key = excluded.object_key,
           size_bytes = excluded.size_bytes,
           content_type = excluded.content_type,
           md5 = excluded.md5,
           deleted_at = NULL,
           updated_at = CURRENT_TIMESTAMP`,
      )
      .run(
        params.id,
        params.tenantId,
        params.vaultId,
        params.clientId,
        params.path,
        params.objectKey,
        params.sizeBytes,
        params.contentType,
        params.md5,
      );
  }

  deleteAssetByClientId(vaultId: string, clientId: string) {
    this.sqlite
      .query("DELETE FROM assets WHERE vault_id = ? AND client_id = ?")
      .run(vaultId, clientId);
  }

  listAssetRefsByNote(vaultId: string, noteClientId: string) {
    return this.sqlite
      .query(
        `SELECT *
         FROM note_asset_refs
         WHERE vault_id = ? AND note_client_id = ?
         ORDER BY asset_client_id`,
      )
      .all(vaultId, noteClientId) as NoteAssetRefRecord[];
  }

  deleteNoteAssetRefsByNote(vaultId: string, noteClientId: string) {
    this.sqlite
      .query("DELETE FROM note_asset_refs WHERE vault_id = ? AND note_client_id = ?")
      .run(vaultId, noteClientId);
  }

  deleteNoteAssetRefsByAsset(vaultId: string, assetClientId: string) {
    this.sqlite
      .query("DELETE FROM note_asset_refs WHERE vault_id = ? AND asset_client_id = ?")
      .run(vaultId, assetClientId);
  }

  countAssetRefs(vaultId: string, assetClientId: string) {
    const row = this.sqlite
      .query(
        `SELECT COUNT(*) AS count
         FROM note_asset_refs
         WHERE vault_id = ? AND asset_client_id = ?`,
      )
      .get(vaultId, assetClientId) as { count: number };

    return row.count;
  }

  replaceNoteAssetRefs(vaultId: string, noteClientId: string, assetClientIds: string[]) {
    const insertStatement = this.sqlite.prepare(
      `INSERT INTO note_asset_refs (id, vault_id, note_client_id, asset_client_id)
       VALUES (?, ?, ?, ?)
       ON CONFLICT(vault_id, note_client_id, asset_client_id) DO NOTHING`,
    );

    const transaction = this.sqlite.transaction(
      (currentVaultId: string, currentNoteClientId: string, currentAssetClientIds: string[]) => {
        this.deleteNoteAssetRefsByNote(currentVaultId, currentNoteClientId);
        for (const assetClientId of currentAssetClientIds) {
          insertStatement.run(crypto.randomUUID(), currentVaultId, currentNoteClientId, assetClientId);
        }
      },
    );

    transaction(vaultId, noteClientId, assetClientIds);
  }

  deleteNoteAndRelations(vaultId: string, clientId: string) {
    const transaction = this.sqlite.transaction((currentVaultId: string, currentClientId: string) => {
      this.deleteNoteLinksBySource(currentVaultId, currentClientId);
      this.deleteNoteLinksByTarget(currentVaultId, currentClientId);
      this.deleteNoteAssetRefsByNote(currentVaultId, currentClientId);
      this.deleteNoteByClientId(currentVaultId, currentClientId);
    });

    transaction(vaultId, clientId);
  }

  deleteAssetAndRelations(vaultId: string, clientId: string) {
    const transaction = this.sqlite.transaction((currentVaultId: string, currentClientId: string) => {
      this.deleteNoteAssetRefsByAsset(currentVaultId, currentClientId);
      this.deleteAssetByClientId(currentVaultId, currentClientId);
    });

    transaction(vaultId, clientId);
  }
}
