import type { App, PluginManifest } from "obsidian";
import { TFile } from "obsidian";
import { describe, expect, test, vi } from "vitest";
import { DEFAULT_SETTINGS, type MdbrainSettings } from "./domain/types";
import MdbrainPlugin from "./main";

const createPlugin = (
  appOverrides?: Partial<App>,
  settingsOverrides?: Partial<MdbrainSettings>,
) => {
  const app = {
    vault: { read: async () => "" },
    metadataCache: { getFileCache: () => null },
    workspace: { getActiveFile: () => null },
    ...appOverrides,
  } as unknown as App;
  const manifest: PluginManifest = {
    id: "mdbrain",
    name: "Mdbrain",
    version: "0.0.0",
    minAppVersion: "0.0.0",
    description: "",
    author: "",
    isDesktopOnly: false,
  };
  const plugin = new MdbrainPlugin(app, manifest);
  plugin.settings = { ...DEFAULT_SETTINGS, ...settingsOverrides };
  return plugin;
};

describe("MdbrainPlugin.handleAssetChange", () => {
  test("does nothing when autoSync is disabled", async () => {
    const plugin = createPlugin();
    const file = new TFile("assets/image.png", "image", "png");
    const syncAssetFile = vi.fn().mockResolvedValue(true);

    const pluginAccess = plugin as unknown as {
      syncAssetFile: (file: TFile) => Promise<boolean>;
      settings: { autoSync: boolean };
    };
    pluginAccess.syncAssetFile = syncAssetFile;
    pluginAccess.settings.autoSync = false;

    await plugin.handleAssetChange(file);

    expect(syncAssetFile).not.toHaveBeenCalled();
  });

  test("skips assets that are not referenced", async () => {
    const plugin = createPlugin();
    const file = new TFile("assets/image.png", "image", "png");
    const syncAssetFile = vi.fn().mockResolvedValue(true);

    const pluginAccess = plugin as unknown as {
      syncAssetFile: (file: TFile) => Promise<boolean>;
      referenceIndexReady: boolean;
      referenceIndex: { isAssetReferenced: (path: string) => boolean };
    };
    pluginAccess.syncAssetFile = syncAssetFile;
    pluginAccess.referenceIndexReady = true;
    pluginAccess.referenceIndex = { isAssetReferenced: () => false };

    await plugin.handleAssetChange(file);

    expect(syncAssetFile).not.toHaveBeenCalled();
  });

  test("does not skip assets when reference index is not ready", async () => {
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const file = new TFile("assets/image.png", "image", "png");
    const syncAssetFile = vi.fn().mockResolvedValue(true);

    const pluginAccess = plugin as unknown as {
      syncAssetFile: (file: TFile) => Promise<boolean>;
      referenceIndexReady: boolean;
      referenceIndex: { isAssetReferenced: (path: string) => boolean };
    };
    pluginAccess.syncAssetFile = syncAssetFile;
    pluginAccess.referenceIndexReady = false;
    pluginAccess.referenceIndex = { isAssetReferenced: () => false };

    await plugin.handleAssetChange(file);

    expect(syncAssetFile).toHaveBeenCalledWith(file);
  });

  test("syncs when asset is referenced", async () => {
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const file = new TFile("assets/image.png", "image", "png");
    const syncAssetFile = vi.fn().mockResolvedValue(true);

    const pluginAccess = plugin as unknown as {
      syncAssetFile: (file: TFile) => Promise<boolean>;
      referenceIndex: { isAssetReferenced: (path: string) => boolean };
      referenceIndexReady: boolean;
    };
    pluginAccess.syncAssetFile = syncAssetFile;
    pluginAccess.referenceIndex = { isAssetReferenced: () => true };
    pluginAccess.referenceIndexReady = true;

    await plugin.handleAssetChange(file);

    expect(syncAssetFile).toHaveBeenCalledWith(file);
  });
});

describe("MdbrainPlugin.collectReferencedAssetFiles", () => {
  test("deduplicates referenced assets across notes", async () => {
    const assets = new Map<string, TFile>();

    const vault = {
      getMarkdownFiles: () => [new TFile("notes/a.md"), new TFile("notes/b.md")],
    };
    const metadataCache = {
      getFileCache: (file: TFile) => {
        if (file.path === "notes/a.md") {
          return { embeds: [{ link: "assets/image.png", original: "![[assets/image.png]]" }] };
        }
        return {
          links: [
            { link: "assets/image.png", original: "![Alt](assets/image.png)" },
            { link: "notes/other.md", original: "[[notes/other.md]]" },
          ],
        };
      },
      getFirstLinkpathDest: (path: string) => assets.get(path) ?? null,
    };

    assets.set("assets/image.png", new TFile("assets/image.png", "image", "png"));

    const plugin = createPlugin({ vault, metadataCache });
    const pluginAccess = plugin as unknown as {
      collectReferencedAssetFiles: () => Promise<TFile[]>;
    };

    const referenced = await pluginAccess.collectReferencedAssetFiles();

    expect(referenced).toHaveLength(1);
    expect(referenced[0]?.path).toBe("assets/image.png");
  });

  test("falls back to content parsing when cache is missing", async () => {
    const assets = new Map<string, TFile>();
    assets.set("assets/image.png", new TFile("assets/image.png", "image", "png"));

    const vault = {
      getMarkdownFiles: () => [new TFile("notes/a.md")],
      read: async () => "![Alt](assets/image.png)\n[[notes/other.md]]",
    };
    const metadataCache = {
      getFileCache: () => null,
      getFirstLinkpathDest: (path: string) => assets.get(path) ?? null,
    };

    const plugin = createPlugin({ vault, metadataCache });
    const pluginAccess = plugin as unknown as {
      collectReferencedAssetFiles: () => Promise<TFile[]>;
    };

    const referenced = await pluginAccess.collectReferencedAssetFiles();

    expect(referenced).toHaveLength(1);
    expect(referenced[0]?.path).toBe("assets/image.png");
  });
});

describe("MdbrainPlugin.handleFileRename", () => {
  test("renames reference index and syncs note", async () => {
    const plugin = createPlugin(
      {
        metadataCache: { getFileCache: () => null } as never,
        vault: { read: async () => "" } as never,
      },
      { publishKey: "test-key" },
    );
    const file = new TFile("notes/new.md");
    const syncNoteFile = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [],
      assetsById: new Map<string, TFile>(),
      needUploadNotes: [],
      linkedNotesById: new Map<string, TFile>(),
    });
    const syncAssetsForNote = vi.fn().mockResolvedValue(undefined);
    const syncLinkedNotesForNote = vi.fn().mockResolvedValue(undefined);

    const pluginAccess = plugin as unknown as {
      syncNoteFile: (file: TFile) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncAssetsForNote: (file: TFile) => Promise<void>;
      syncLinkedNotesForNote: (file: TFile) => Promise<void>;
      settings: { autoSync: boolean };
      referenceIndex: {
        renameNote: (oldPath: string, newPath: string) => void;
        updateNote: () => void;
      };
    };
    pluginAccess.syncNoteFile = syncNoteFile;
    pluginAccess.syncAssetsForNote = syncAssetsForNote;
    pluginAccess.syncLinkedNotesForNote = syncLinkedNotesForNote;
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { renameNote: vi.fn(), updateNote: vi.fn() };

    await plugin.handleFileRename(file, "notes/old.md");

    expect(pluginAccess.referenceIndex.renameNote).toHaveBeenCalledWith(
      "notes/old.md",
      "notes/new.md",
    );
    expect(syncNoteFile).toHaveBeenCalledWith(file);
    expect(syncAssetsForNote).toHaveBeenCalled();
    expect(syncLinkedNotesForNote).toHaveBeenCalled();
  });
});

describe("MdbrainPlugin.handleMarkdownCacheChanged", () => {
  test("removing the last image embed only syncs the note (does not full sync)", async () => {
    const note = new TFile("notes/a.md");
    const asset = new TFile("assets/image.png", "image", "png");

    const vault = {
      readBinary: async (file: TFile) => {
        if (file.path !== asset.path) {
          throw new Error(`Unexpected readBinary for ${file.path}`);
        }
        return new Uint8Array([1, 2, 3]).buffer;
      },
    };
    const metadataCache = {
      getFileCache: (_file: TFile) => ({ frontmatter: { "mdbrain-id": "note-a" } }),
      getFirstLinkpathDest: (linkpath: string) => {
        if (linkpath === asset.path) return asset;
        return null;
      },
    };

    const plugin = createPlugin({ vault, metadataCache }, { publishKey: "test-key" });

    const syncNote = vi.fn().mockResolvedValue({
      success: true,
      need_upload_assets: [],
      need_upload_notes: [],
    });
    const syncChanges = vi.fn();
    const fullSyncSpy = vi.spyOn(plugin, "fullSync");

    let pending: Promise<void> | null = null;

    const pluginAccess = plugin as unknown as {
      debounceService: { debounce: (key: string, callback: () => void, delay: number) => void };
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncAssetsForNote: () => Promise<void>;
      syncLinkedNotesForNote: () => Promise<void>;
      syncClient: { syncNote: typeof syncNote; syncChanges: typeof syncChanges };
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.debounceService = {
      debounce: (_key, callback, _delay) => {
        pending = callback() as Promise<void>;
      },
    };
    pluginAccess.getClientIdForSync = vi.fn().mockResolvedValue("note-a");
    pluginAccess.syncAssetsForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.syncLinkedNotesForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.syncClient = { syncNote, syncChanges };
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "before ![[assets/image.png]]", {
      embeds: [{ link: "assets/image.png", original: "![[assets/image.png]]" }],
    });
    if (pending) await pending;

    plugin.handleMarkdownCacheChanged(note, "after (no embeds)", { embeds: [] });
    if (pending) await pending;

    expect(fullSyncSpy).not.toHaveBeenCalled();
    expect(syncChanges).not.toHaveBeenCalled();

    const lastCall = syncNote.mock.calls.at(-1);
    expect(lastCall?.[0]).toBe("note-a");
    expect(lastCall?.[1]).toMatchObject({
      path: "notes/a.md",
      content: "after (no embeds)",
      assets: [],
    });
  });

  test("uploads referenced assets after note sync", async () => {
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const note = new TFile("notes/a.md");
    const assetA = new TFile("assets/a.png", "a", "png");
    const assetB = new TFile("assets/b.png", "b", "png");

    const assetMap = new Map<string, TFile>([
      ["asset-a", assetA],
      ["asset-b", assetB],
    ]);
    const syncNoteFromCache = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [
        { id: "asset-a", hash: "md5-a" },
        { id: "asset-b", hash: "md5-b" },
      ],
      assetsById: assetMap,
      needUploadNotes: [],
      linkedNotesById: new Map<string, TFile>(),
    });
    const syncAssetFile = vi.fn().mockResolvedValue(true);
    let pending: Promise<void> | null = null;

    const pluginAccess = plugin as unknown as {
      debounceService: { debounce: (key: string, callback: () => void, delay: number) => void };
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncNoteFromCache: (
        file: TFile,
        data: string,
        cache: unknown,
      ) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncAssetFile: (file: TFile) => Promise<boolean>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.debounceService = {
      debounce: (_key, callback, _delay) => {
        pending = callback() as Promise<void>;
      },
    };
    pluginAccess.getClientIdForSync = vi.fn().mockResolvedValue("existing-id");
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.syncAssetFile = syncAssetFile;
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "content", { links: [] });
    if (pending) {
      await pending;
    }

    expect(syncNoteFromCache).toHaveBeenCalledWith(note, "content", { links: [] });
    expect(syncAssetFile).toHaveBeenCalledWith(assetA);
    expect(syncAssetFile).toHaveBeenCalledWith(assetB);
  });

  test("debounces multiple rapid cache changes and uses the latest content", async () => {
    vi.useFakeTimers();
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const note = new TFile("notes/a.md");

    const syncNoteFromCache = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [],
      assetsById: new Map<string, TFile>(),
      needUploadNotes: [],
      linkedNotesById: new Map<string, TFile>(),
    });

    const pluginAccess = plugin as unknown as {
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncNoteFromCache: (
        file: TFile,
        data: string,
        cache: unknown,
      ) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncAssetsForNote: () => Promise<void>;
      syncLinkedNotesForNote: () => Promise<void>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.getClientIdForSync = vi.fn().mockResolvedValue("existing-id");
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.syncAssetsForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.syncLinkedNotesForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "first", null);
    plugin.handleMarkdownCacheChanged(note, "second", null);

    await vi.advanceTimersByTimeAsync(1200);

    expect(syncNoteFromCache).toHaveBeenCalledTimes(1);
    expect(syncNoteFromCache).toHaveBeenCalledWith(note, "second", null);
    vi.useRealTimers();
  });

  test("uploads linked notes after note sync", async () => {
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const note = new TFile("notes/a.md");
    const linked = new TFile("notes/b.md");
    const linkedMap = new Map<string, TFile>([["note-b", linked]]);
    const syncNoteFromCache = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [],
      assetsById: new Map<string, TFile>(),
      needUploadNotes: [{ id: "note-b", hash: "hash-b" }],
      linkedNotesById: linkedMap,
    });
    const syncNoteFile = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [],
      assetsById: new Map<string, TFile>(),
      needUploadNotes: [],
      linkedNotesById: new Map<string, TFile>(),
    });
    const syncAssetsForNote = vi.fn().mockResolvedValue(undefined);
    let pending: Promise<void> | null = null;

    const pluginAccess = plugin as unknown as {
      debounceService: { debounce: (key: string, callback: () => void, delay: number) => void };
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncNoteFromCache: (
        file: TFile,
        data: string,
        cache: unknown,
      ) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncNoteFile: (file: TFile) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncAssetsForNote: (file: TFile) => Promise<void>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.debounceService = {
      debounce: (_key, callback, _delay) => {
        pending = callback() as Promise<void>;
      },
    };
    pluginAccess.getClientIdForSync = vi.fn().mockResolvedValue("existing-id");
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.syncNoteFile = syncNoteFile;
    pluginAccess.syncAssetsForNote = syncAssetsForNote;
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "content", { links: [] });
    if (pending) {
      await pending;
    }

    expect(syncNoteFromCache).toHaveBeenCalledWith(note, "content", { links: [] });
    expect(syncNoteFile).toHaveBeenCalledWith(linked);
    expect(syncAssetsForNote).toHaveBeenCalledWith(linked, [], expect.any(Map));
  });

  test("sync does not write to file (no self-write loop)", async () => {
    vi.useFakeTimers();
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const note = new TFile("notes/a.md");
    const originalContent = "# Test content";

    const getClientId = vi.fn().mockResolvedValue("existing-id");
    const syncNoteFromCache = vi.fn().mockResolvedValue({
      success: true,
      needUploadAssets: [],
      assetsById: new Map<string, TFile>(),
      needUploadNotes: [],
      linkedNotesById: new Map<string, TFile>(),
    });

    const pluginAccess = plugin as unknown as {
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncNoteFromCache: (
        file: TFile,
        data: string,
        cache: unknown,
      ) => Promise<{
        success: boolean;
        needUploadAssets: Array<{ id: string; hash: string }>;
        assetsById: Map<string, TFile>;
        needUploadNotes: Array<{ id: string; hash: string }>;
        linkedNotesById: Map<string, TFile>;
      }>;
      syncAssetsForNote: () => Promise<void>;
      syncLinkedNotesForNote: () => Promise<void>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.getClientIdForSync = getClientId;
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.syncAssetsForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.syncLinkedNotesForNote = vi.fn().mockResolvedValue(undefined);
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, originalContent, null);
    await vi.advanceTimersByTimeAsync(1200);

    expect(getClientId).toHaveBeenCalledWith(note);
    expect(syncNoteFromCache).toHaveBeenCalledWith(note, originalContent, null);
    vi.useRealTimers();
  });

  test("skips sync when note has no client ID", async () => {
    vi.useFakeTimers();
    const plugin = createPlugin(undefined, { publishKey: "test-key" });
    const note = new TFile("notes/a.md");

    const getClientId = vi.fn().mockResolvedValue(null);
    const syncNoteFromCache = vi.fn();

    const pluginAccess = plugin as unknown as {
      getClientIdForSync: (file: TFile) => Promise<string | null>;
      syncNoteFromCache: (file: TFile, data: string, cache: unknown) => Promise<unknown>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.getClientIdForSync = getClientId;
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "content", null);
    await vi.advanceTimersByTimeAsync(1200);

    expect(getClientId).toHaveBeenCalledWith(note);
    expect(syncNoteFromCache).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  test("skips sync when publish config is missing", async () => {
    vi.useFakeTimers();
    const plugin = createPlugin();
    const note = new TFile("notes/a.md");

    const syncNoteFromCache = vi.fn();

    const pluginAccess = plugin as unknown as {
      syncNoteFromCache: (file: TFile, data: string, cache: unknown) => Promise<unknown>;
      settings: { autoSync: boolean };
      referenceIndex: { updateNote: (notePath: string) => void };
    };
    pluginAccess.syncNoteFromCache = syncNoteFromCache;
    pluginAccess.settings.autoSync = true;
    pluginAccess.referenceIndex = { updateNote: vi.fn() };

    plugin.handleMarkdownCacheChanged(note, "content", null);
    await vi.advanceTimersByTimeAsync(1200);

    expect(syncNoteFromCache).not.toHaveBeenCalled();
    vi.useRealTimers();
  });
});
