/**
 * Mdbrain Obsidian Plugin - Snapshot Sync
 */

import { type App, Notice, Plugin, type PluginManifest, TFile } from "obsidian";
import { ObsidianHttpClient } from "./api";
import { SyncApiClient, type SyncSnapshotEntry } from "./api/sync-api";
import { ensureClientId, getClientId } from "./core/client-id";
import { DEFAULT_SETTINGS, type MdbrainSettings } from "./domain/types";
import { MdbrainSettingTab, registerFileEvents } from "./plugin";
import {
  type CachedMetadataLike,
  DebounceService,
  extractInternalLinkpathsFromCache,
  extractNoteMetadata,
  ReferenceIndex,
} from "./services";
import { getContentType, hashString, isAssetFile, md5Hash } from "./utils";
import { extractAssetPaths, extractNotePaths } from "./utils/asset-links";

export default class MdbrainPlugin extends Plugin {
  settings!: MdbrainSettings;
  syncClient!: SyncApiClient;
  private debounceService: DebounceService;
  private referenceIndex: ReferenceIndex;
  private referenceIndexReady: boolean;

  constructor(app: App, manifest: PluginManifest) {
    super(app, manifest);
    this.debounceService = new DebounceService();
    this.referenceIndex = this.createReferenceIndex();
    this.referenceIndexReady = false;
  }

  private isSyncConfigured(): boolean {
    const serverUrl = this.settings.serverUrl?.trim();
    const publishKey = this.settings.publishKey?.trim();
    return Boolean(serverUrl) && Boolean(publishKey);
  }

  private ensureSyncConfigured(notify = false): boolean {
    if (this.isSyncConfigured()) return true;
    if (notify) {
      new Notice("Publish is not configured. Set Publish URL and Key in Mdbrain settings.");
    }
    return false;
  }

  async onload() {
    console.log("[Mdbrain] Plugin loading (snapshot publish)...");
    await this.loadSettings();

    const httpClient = new ObsidianHttpClient();
    this.syncClient = new SyncApiClient(
      {
        serverUrl: this.settings.serverUrl,
        publishKey: this.settings.publishKey,
      },
      httpClient,
    );

    this.addSettingTab(new MdbrainSettingTab(this.app, this));

    this.addCommand({
      id: "sync-current-file",
      name: "Publish current file",
      callback: () => {
        const file = this.app.workspace.getActiveFile();
        if (file && file.extension === "md") {
          void this.syncCurrentFile(file);
        }
      },
    });

    this.addCommand({
      id: "sync-all-files",
      name: "Publish all files (full publish)",
      callback: () => this.fullSync(),
    });

    this.app.workspace.onLayoutReady(() => {
      void (async () => {
        await this.ensureAllNotesHaveClientIds();
        await this.buildReferenceIndexFromCurrentCache();

        registerFileEvents(
          this.app,
          {
            onMarkdownCacheChanged: (file, data, cache) =>
              this.handleMarkdownCacheChanged(file, data, cache),
            onFileDelete: (file) => this.handleFileDelete(file),
            onFileRename: (file, oldPath) => this.handleFileRename(file, oldPath),
            onAssetChange: (file) => this.handleAssetChange(file),
            onAssetDelete: () => this.fullSync(),
            onAssetRename: () => this.fullSync(),
            onMarkdownCreated: (file) => this.handleMarkdownCreated(file),
          },
          (event) => this.registerEvent(event),
        );

        console.log("[Mdbrain] ✓ Plugin loaded");
      })();
    });
  }

  private createReferenceIndex(): ReferenceIndex {
    return new ReferenceIndex((linkpath, sourcePath) => {
      const resolved = this.app.metadataCache.getFirstLinkpathDest(linkpath, sourcePath);
      if (!(resolved instanceof TFile)) return null;
      if (resolved.extension === "md") return { path: resolved.path, kind: "note" };
      if (isAssetFile(resolved)) return { path: resolved.path, kind: "asset" };
      return null;
    });
  }

  private extractLinkpaths(content: string, cache: CachedMetadataLike | null): string[] {
    if (cache) {
      return extractInternalLinkpathsFromCache(cache);
    }

    const candidates = [...extractNotePaths(content), ...extractAssetPaths(content)];
    const seen = new Set<string>();
    const results: string[] = [];
    for (const candidate of candidates) {
      if (!candidate) continue;
      if (seen.has(candidate)) continue;
      seen.add(candidate);
      results.push(candidate);
    }
    return results;
  }

  private async ensureAllNotesHaveClientIds(): Promise<void> {
    const files = this.app.vault.getMarkdownFiles();
    let assigned = 0;
    for (const file of files) {
      const existingId = await getClientId(file, this.app);
      if (!existingId) {
        await ensureClientId(file, this.app);
        assigned++;
      }
    }
    if (assigned > 0) {
      console.log(`[Mdbrain] Assigned IDs to ${assigned} notes`);
    }
  }

  private async buildReferenceIndexFromCurrentCache(): Promise<void> {
    this.referenceIndexReady = false;
    this.referenceIndex = this.createReferenceIndex();

    const files = this.app.vault.getMarkdownFiles();
    let indexed = 0;
    for (const file of files) {
      const cache = this.app.metadataCache.getFileCache(
        file,
      ) as unknown as CachedMetadataLike | null;
      const content = cache ? "" : await this.app.vault.read(file);
      const linkpaths = this.extractLinkpaths(content, cache);
      this.referenceIndex.updateNote(file.path, linkpaths);
      indexed++;
    }
    this.referenceIndexReady = true;
    console.log(`[Mdbrain] Reference index: ${indexed}/${files.length}`);
  }

  // =========================================================================
  // File Event Handlers
  // =========================================================================

  private async getClientIdForSync(file: TFile): Promise<string | null> {
    return getClientId(file, this.app);
  }

  async handleMarkdownCreated(file: TFile): Promise<void> {
    await ensureClientId(file, this.app);
  }

  private async syncCurrentFile(file: TFile): Promise<void> {
    if (!this.ensureSyncConfigured(true)) {
      return;
    }
    const result = await this.syncNoteFile(file);
    if (!result.success) {
      new Notice("Publish failed: note upload failed");
      return;
    }
    await this.syncAssetsForNote(file, result.needUploadAssets, result.assetsById);
    await this.syncLinkedNotesForNote(file, result.needUploadNotes, result.linkedNotesById);
  }

  handleMarkdownCacheChanged(file: TFile, data: string, cache: CachedMetadataLike | null): void {
    if (!this.settings.autoSync) return;

    this.referenceIndex.updateNote(file.path, this.extractLinkpaths(data, cache));

    if (!this.isSyncConfigured()) return;

    this.debounceService.debounce(
      file.path,
      async () => {
        const clientId = await this.getClientIdForSync(file);
        if (!clientId) {
          return;
        }

        const result = await this.syncNoteFromCache(file, data, cache);
        if (!result.success) {
          new Notice("Publish failed: note upload failed");
          return;
        }

        await this.syncAssetsForNote(file, result.needUploadAssets, result.assetsById);
        await this.syncLinkedNotesForNote(file, result.needUploadNotes, result.linkedNotesById);
      },
      1200,
    );
  }

  async handleFileDelete(_file: TFile) {
    if (!this.settings.autoSync) return;
    this.referenceIndex.removeNote(_file.path);
    if (!this.isSyncConfigured()) return;
    await this.fullSync();
  }

  async handleFileRename(file: TFile, oldPath: string) {
    if (!this.settings.autoSync) return;
    this.referenceIndex.renameNote(oldPath, file.path);
    const cache = this.app.metadataCache.getFileCache(file) as unknown as CachedMetadataLike | null;
    const content = cache ? "" : await this.app.vault.read(file);
    this.referenceIndex.updateNote(file.path, this.extractLinkpaths(content, cache));

    if (!this.isSyncConfigured()) return;

    const result = await this.syncNoteFile(file);
    if (!result.success) {
      new Notice("Publish failed: note rename failed");
      return;
    }
    await this.syncAssetsForNote(file, result.needUploadAssets, result.assetsById);
    await this.syncLinkedNotesForNote(file, result.needUploadNotes, result.linkedNotesById);
  }

  async handleAssetChange(file: TFile) {
    if (!this.settings.autoSync) return;
    if (!this.isSyncConfigured()) return;
    if (this.referenceIndexReady && !this.referenceIndex.isAssetReferenced(file.path)) {
      return;
    }
    const result = await this.syncAssetFile(file);
    if (!result) {
      new Notice("Publish failed: asset upload failed");
    }
  }

  // =========================================================================
  // Sync Operations
  // =========================================================================

  private async syncNoteFromCache(
    file: TFile,
    content: string,
    cache: CachedMetadataLike | null,
  ): Promise<{
    success: boolean;
    needUploadAssets: Array<{ id: string; hash: string }>;
    assetsById: Map<string, TFile>;
    needUploadNotes: Array<{ id: string; hash: string }>;
    linkedNotesById: Map<string, TFile>;
  }> {
    if (!this.isSyncConfigured()) {
      return {
        success: false,
        needUploadAssets: [],
        assetsById: new Map(),
        needUploadNotes: [],
        linkedNotesById: new Map(),
      };
    }
    const clientId = await getClientId(file, this.app);
    if (!clientId) {
      return {
        success: false,
        needUploadAssets: [],
        assetsById: new Map(),
        needUploadNotes: [],
        linkedNotesById: new Map(),
      };
    }

    const hash = await hashString(content);
    const metadata = extractNoteMetadata(cache);
    const assets = await this.collectReferencedAssetEntriesForNoteUsingCache(file, cache, content);
    const linkedNotes = await this.collectLinkedNoteEntriesForNoteUsingCache(file, cache, content);

    const result = await this.syncClient.syncNote(clientId, {
      path: file.path,
      content,
      hash,
      metadata: metadata as Record<string, unknown>,
      assets: assets.entries,
      linked_notes: linkedNotes.entries,
    });

    return {
      success: result.success,
      needUploadAssets: result.need_upload_assets ?? [],
      assetsById: assets.byId,
      needUploadNotes: result.need_upload_notes ?? [],
      linkedNotesById: linkedNotes.byId,
    };
  }

  private async syncNoteFile(file: TFile): Promise<{
    success: boolean;
    needUploadAssets: Array<{ id: string; hash: string }>;
    assetsById: Map<string, TFile>;
    needUploadNotes: Array<{ id: string; hash: string }>;
    linkedNotesById: Map<string, TFile>;
  }> {
    const content = await this.app.vault.read(file);
    const cache = this.app.metadataCache.getFileCache(file) as unknown as CachedMetadataLike | null;
    return this.syncNoteFromCache(file, content, cache);
  }

  private async syncAssetFile(file: TFile): Promise<boolean> {
    if (!this.isSyncConfigured()) {
      return false;
    }
    const buffer = await this.app.vault.readBinary(file);
    const assetId = await hashString(file.path);
    const hash = await md5Hash(buffer);
    const base64 = Buffer.from(new Uint8Array(buffer)).toString("base64");

    const result = await this.syncClient.syncAsset(assetId, {
      path: file.path,
      contentType: getContentType(file.extension),
      size: buffer.byteLength,
      hash,
      content: base64,
    });

    return result.success;
  }

  async fullSync(): Promise<void> {
    if (!this.ensureSyncConfigured(true)) {
      return;
    }
    const notes = await this.buildNoteSnapshot();
    const referencedAssets = await this.collectReferencedAssetFiles();
    const assets = await this.buildAssetSnapshot(referencedAssets);

    new Notice(`Starting full publish: ${notes.length} notes / ${assets.length} assets`);

    const changes = await this.syncClient.syncChanges({ notes, assets });
    if (!changes.success) {
      new Notice(`Full publish failed: ${changes.error}`);
      return;
    }

    const needNotes = changes.need_upsert?.notes ?? [];
    const needAssets = changes.need_upsert?.assets ?? [];

    if (needAssets.length > 0) {
      await this.uploadAssets(needAssets, referencedAssets);
    }

    if (needNotes.length > 0) {
      await this.uploadNotes(needNotes);
    }

    new Notice("Full publish completed");
  }

  private async uploadNotes(entries: SyncSnapshotEntry[]): Promise<void> {
    const fileMap = new Map<string, TFile>();
    for (const file of this.app.vault.getMarkdownFiles()) {
      const clientId = await getClientId(file, this.app);
      if (clientId) {
        fileMap.set(clientId, file);
      }
    }

    for (const entry of entries) {
      const file = fileMap.get(entry.id);
      if (!file) continue;
      const result = await this.syncNoteFile(file);
      if (!result.success) {
        new Notice("Publish failed: note upload failed");
      }
    }
  }

  private async uploadAssets(entries: SyncSnapshotEntry[], assetFiles?: TFile[]): Promise<void> {
    const targetAssets = assetFiles ?? this.app.vault.getFiles().filter(isAssetFile);
    const assetMap = new Map<string, TFile>();
    for (const file of targetAssets) {
      const assetId = await hashString(file.path);
      assetMap.set(assetId, file);
    }

    const tasks = entries.map((entry) => async () => {
      const file = assetMap.get(entry.id);
      if (!file) return;
      await this.syncAssetFile(file);
    });

    await this.runWithConcurrency(tasks, 3);
  }

  private async runWithConcurrency(
    tasks: Array<() => Promise<void>>,
    limit: number,
  ): Promise<void> {
    const queue = [...tasks];
    const workers = Array.from({ length: limit }, async () => {
      while (queue.length > 0) {
        const task = queue.shift();
        if (task) {
          await task();
        }
      }
    });
    await Promise.all(workers);
  }

  private async buildNoteSnapshot(): Promise<SyncSnapshotEntry[]> {
    const files = this.app.vault.getMarkdownFiles();
    const snapshot: SyncSnapshotEntry[] = [];

    for (const file of files) {
      const content = await this.app.vault.read(file);
      if (!content.trim()) continue;

      const clientId = await ensureClientId(file, this.app);
      const hash = await hashString(content);
      snapshot.push({ id: clientId, hash });
    }

    return snapshot;
  }

  private async buildAssetSnapshot(assetFiles?: TFile[]): Promise<SyncSnapshotEntry[]> {
    const assets = assetFiles ?? this.app.vault.getFiles().filter(isAssetFile);
    const snapshot: SyncSnapshotEntry[] = [];

    for (const file of assets) {
      const buffer = await this.app.vault.readBinary(file);
      const assetId = await hashString(file.path);
      const hash = await md5Hash(buffer);
      snapshot.push({ id: assetId, hash });
    }

    return snapshot;
  }

  private async collectReferencedAssetFiles(): Promise<TFile[]> {
    const referenced = new Map<string, TFile>();
    const notes = this.app.vault.getMarkdownFiles();

    for (const note of notes) {
      const cache = this.app.metadataCache.getFileCache(
        note,
      ) as unknown as CachedMetadataLike | null;
      const content = cache ? "" : await this.app.vault.read(note);
      const { assets } = this.resolveReferencesFromCache(note, cache, content);
      for (const asset of assets) referenced.set(asset.path, asset);
    }

    return Array.from(referenced.values());
  }

  private resolveReferencesFromCache(
    note: TFile,
    cache: CachedMetadataLike | null,
    content?: string,
  ): { assets: TFile[]; linkedNotes: TFile[] } {
    const linkpaths = this.extractLinkpaths(content ?? "", cache);
    const assets = new Map<string, TFile>();
    const linkedNotes = new Map<string, TFile>();

    for (const linkpath of linkpaths) {
      const resolved = this.app.metadataCache.getFirstLinkpathDest(linkpath, note.path);
      if (!(resolved instanceof TFile)) continue;
      if (resolved.extension === "md") {
        linkedNotes.set(resolved.path, resolved);
      } else if (isAssetFile(resolved)) {
        assets.set(resolved.path, resolved);
      }
    }

    return { assets: Array.from(assets.values()), linkedNotes: Array.from(linkedNotes.values()) };
  }

  private async collectReferencedAssetEntriesForNote(note: TFile): Promise<{
    entries: Array<{ id: string; hash: string }>;
    byId: Map<string, TFile>;
  }> {
    const cache = this.app.metadataCache.getFileCache(note) as unknown as CachedMetadataLike | null;
    return this.collectReferencedAssetEntriesForNoteUsingCache(note, cache);
  }

  private async collectReferencedAssetEntriesForNoteUsingCache(
    note: TFile,
    cache: CachedMetadataLike | null,
    content?: string,
  ): Promise<{
    entries: Array<{ id: string; hash: string }>;
    byId: Map<string, TFile>;
  }> {
    const files = this.resolveReferencesFromCache(note, cache, content).assets;
    const byId = new Map<string, TFile>();
    const entries = await Promise.all(
      files.map(async (file) => {
        const buffer = await this.app.vault.readBinary(file);
        const id = await hashString(file.path);
        const hash = await md5Hash(buffer);
        byId.set(id, file);
        return { id, hash };
      }),
    );
    return { entries, byId };
  }

  private async collectLinkedNoteEntriesForNote(note: TFile): Promise<{
    entries: Array<{ id: string; hash: string }>;
    byId: Map<string, TFile>;
  }> {
    const cache = this.app.metadataCache.getFileCache(note) as unknown as CachedMetadataLike | null;
    return this.collectLinkedNoteEntriesForNoteUsingCache(note, cache);
  }

  private async collectLinkedNoteEntriesForNoteUsingCache(
    note: TFile,
    cache: CachedMetadataLike | null,
    content?: string,
  ): Promise<{
    entries: Array<{ id: string; hash: string }>;
    byId: Map<string, TFile>;
  }> {
    const linkedFiles = new Map<string, TFile>();
    for (const linked of this.resolveReferencesFromCache(note, cache, content).linkedNotes) {
      if (linked.path !== note.path) linkedFiles.set(linked.path, linked);
    }

    const byId = new Map<string, TFile>();
    const entries: Array<{ id: string; hash: string }> = [];

    for (const file of linkedFiles.values()) {
      const id = await getClientId(file, this.app);
      if (!id) continue;

      const linkedContent = await this.app.vault.read(file);
      const hash = await hashString(linkedContent);
      byId.set(id, file);
      entries.push({ id, hash });
    }

    return { entries, byId };
  }

  private async syncAssetsForNote(
    note: TFile,
    needUploadAssets?: Array<{ id: string }>,
    assetsById?: Map<string, TFile>,
  ): Promise<void> {
    const lookup = assetsById ?? (await this.collectReferencedAssetEntriesForNote(note)).byId;
    const assetsToUpload = needUploadAssets?.length
      ? needUploadAssets
      : Array.from(lookup.keys()).map((id) => ({ id }));

    if (assetsToUpload.length === 0) {
      return;
    }

    let failed = false;
    for (const asset of assetsToUpload) {
      const file = lookup.get(asset.id);
      if (!file) continue;
      const result = await this.syncAssetFile(file);
      if (!result) {
        failed = true;
      }
    }

    if (failed) {
      new Notice("Publish failed: asset upload failed");
    }
  }

  private async syncLinkedNotesForNote(
    note: TFile,
    needUploadNotes: Array<{ id: string; hash: string }>,
    linkedNotesById: Map<string, TFile>,
  ): Promise<void> {
    if (needUploadNotes.length === 0) {
      return;
    }

    let failed = false;
    for (const entry of needUploadNotes) {
      const file = linkedNotesById.get(entry.id);
      if (!file || file.path === note.path) continue;
      const result = await this.syncNoteFile(file);
      if (!result.success) {
        failed = true;
        continue;
      }
      await this.syncAssetsForNote(file, result.needUploadAssets, result.assetsById);
    }

    if (failed) {
      new Notice("Publish failed: linked note upload failed");
    }
  }

  // extractAssetIds removed; asset uploads now use per-note asset entries with hashes.

  // =========================================================================
  // Settings Persistence
  // =========================================================================

  async loadSettings() {
    const data = await this.loadData();
    const publishKey =
      data && typeof data === "object"
        ? ((data as Record<string, unknown>).publishKey ??
          (data as Record<string, unknown>).syncKey)
        : undefined;
    this.settings = Object.assign({}, DEFAULT_SETTINGS, data, {
      publishKey: typeof publishKey === "string" ? publishKey : "",
    });
  }

  async saveSettings() {
    await this.saveData({
      ...this.settings,
    });
    this.syncClient.updateConfig({
      serverUrl: this.settings.serverUrl,
      publishKey: this.settings.publishKey,
    });
  }

  onunload() {
    console.log("[Mdbrain] Plugin unloading");
    this.debounceService.clearAll();
  }
}
