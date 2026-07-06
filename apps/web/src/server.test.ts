import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createMdbrainWebApp } from "./server";

function readCookie(response: Response, currentCookie = "") {
  const setCookie = response.headers.get("set-cookie");
  if (!setCookie) {
    return currentCookie;
  }

  return setCookie.split(";", 1)[0] ?? currentCookie;
}

function extractCsrfToken(html: string) {
  const match = html.match(/name="__anti-forgery-token" value="([^"]+)"/);
  if (!match?.[1]) {
    throw new Error("CSRF token not found");
  }

  return match[1];
}

async function initializeConsoleAndCreateVault(app: ReturnType<typeof createMdbrainWebApp>) {
  let cookie = "";

  const initPage = await app.fetch(new Request("http://localhost:9090/console/init"));
  cookie = readCookie(initPage, cookie);
  const initHtml = await initPage.text();
  const initCsrf = extractCsrfToken(initHtml);

  const initForm = new FormData();
  initForm.set("__anti-forgery-token", initCsrf);
  initForm.set("tenant-name", "Acme Notes");
  initForm.set("username", "console@example.com");
  initForm.set("password", "password-123");

  const initResponse = await app.fetch(
    new Request("http://localhost:9090/console/init", {
      method: "POST",
      headers: { cookie },
      body: initForm,
    }),
  );
  cookie = readCookie(initResponse, cookie);

  const loginPage = await app.fetch(
    new Request("http://localhost:9090/console/login", {
      headers: { cookie },
    }),
  );
  cookie = readCookie(loginPage, cookie);
  const loginHtml = await loginPage.text();
  const loginCsrf = extractCsrfToken(loginHtml);

  const loginForm = new FormData();
  loginForm.set("__anti-forgery-token", loginCsrf);
  loginForm.set("username", "console@example.com");
  loginForm.set("password", "password-123");

  const loginResponse = await app.fetch(
    new Request("http://localhost:9090/console/login", {
      method: "POST",
      headers: { cookie },
      body: loginForm,
    }),
  );
  cookie = readCookie(loginResponse, cookie);

  const dashboardPage = await app.fetch(
    new Request("http://localhost:9090/console", {
      headers: { cookie },
    }),
  );
  cookie = readCookie(dashboardPage, cookie);
  const dashboardHtml = await dashboardPage.text();
  const dashboardCsrf = extractCsrfToken(dashboardHtml);

  const createVaultForm = new FormData();
  createVaultForm.set("__anti-forgery-token", dashboardCsrf);
  createVaultForm.set("name", "Docs");
  createVaultForm.set("domain", "https://notes.example.com/");

  await app.fetch(
    new Request("http://localhost:9090/console/vaults", {
      method: "POST",
      headers: {
        cookie,
        "HX-Request": "true",
      },
      body: createVaultForm,
    }),
  );

  const user = app.db.getUserByUsername("console@example.com");
  const vaults = app.db.listVaultsByTenant(user!.tenant_id);
  return {
    cookie,
    csrfToken: dashboardCsrf,
    user: user!,
    vault: vaults[0]!,
  };
}

describe("Bun console app", () => {
  let tempDataPath: string;
  let app: ReturnType<typeof createMdbrainWebApp>;

  beforeEach(() => {
    tempDataPath = mkdtempSync(join(tmpdir(), "mdbrain-bun-"));
    app = createMdbrainWebApp({
      cwd: process.cwd(),
      env: {
        ...process.env,
        DATA_PATH: tempDataPath,
        SESSION_SECRET: "test-session-secret",
        HOST: "127.0.0.1",
        CONSOLE_PORT: "9090",
        CADDY_ON_DEMAND_TLS_ENABLED: "true",
      },
    });
  });

  afterEach(() => {
    app.close();
    rmSync(tempDataPath, { recursive: true, force: true });
  });

  test("initializes the console, creates a vault, and serves vault info", async () => {
    const redirectToInit = await app.fetch(new Request("http://localhost:9090/console"));
    expect(redirectToInit.status).toBe(302);
    expect(redirectToInit.headers.get("location")).toBe("/console/init");
    const { vault } = await initializeConsoleAndCreateVault(app);
    expect(vault.domain).toBe("notes.example.com");

    const domainCheckResponse = await app.fetch(
      new Request("http://localhost:9090/console/domain-check?domain=notes.example.com"),
    );
    expect(domainCheckResponse.status).toBe(200);
    expect(await domainCheckResponse.text()).toBe("ok");

    const missingDomainCheckResponse = await app.fetch(
      new Request("http://localhost:9090/console/domain-check?domain=missing.example.com"),
    );
    expect(missingDomainCheckResponse.status).toBe(404);

    const vaultInfoResponse = await app.fetch(
      new Request("http://localhost:9090/obsidian/vault/info", {
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
        },
      }),
    );
    expect(vaultInfoResponse.status).toBe(200);

    const vaultInfo = (await vaultInfoResponse.json()) as {
      success: boolean;
      vault?: { name: string; domain?: string };
    };
    expect(vaultInfo.success).toBe(true);
    expect(vaultInfo.vault?.name).toBe("Docs");
    expect(vaultInfo.vault?.domain).toBe("notes.example.com");
  });

  test("renders a login error when the password is incorrect", async () => {
    const passwordHash = await Bun.password.hash("password-123");
    app.db.createInitialConsoleUser({
      tenantId: "tenant-1",
      tenantName: "Acme Notes",
      userId: "user-1",
      username: "console@example.com",
      passwordHash,
    });

    let cookie = "";
    const loginPage = await app.fetch(new Request("http://localhost:9090/console/login"));
    cookie = readCookie(loginPage, cookie);
    const loginHtml = await loginPage.text();
    const loginCsrf = extractCsrfToken(loginHtml);

    const loginForm = new FormData();
    loginForm.set("__anti-forgery-token", loginCsrf);
    loginForm.set("username", "console@example.com");
    loginForm.set("password", "wrong-password");

    const loginResponse = await app.fetch(
      new Request("http://localhost:9090/console/login", {
        method: "POST",
        headers: { cookie },
        body: loginForm,
      }),
    );

    expect(loginResponse.status).toBe(422);
    expect(await loginResponse.text()).toContain("Invalid username or password");
  });

  test("publishes notes and assets through Bun sync endpoints", async () => {
    const { vault } = await initializeConsoleAndCreateVault(app);
    const assetId = "asset-1";
    const assetHash = "md5-asset-1";
    const noteId = "note-1";
    const noteHash = "hash-note-1";
    const linkedNoteId = "note-2";
    const linkedNoteHash = "hash-note-2";

    const changesResponse = await app.fetch(
      new Request("http://localhost:9090/obsidian/sync/changes", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          notes: [{ id: noteId, hash: noteHash }],
          assets: [{ id: assetId, hash: assetHash }],
        }),
      }),
    );
    expect(changesResponse.status).toBe(200);
    const changes = (await changesResponse.json()) as {
      need_upsert: { notes: Array<{ id: string }>; assets: Array<{ id: string }> };
    };
    expect(changes.need_upsert.notes).toEqual([{ id: noteId, hash: noteHash }]);
    expect(changes.need_upsert.assets).toEqual([{ id: assetId, hash: assetHash }]);

    const assetBody = Buffer.from("image-bytes").toString("base64");
    const assetResponse = await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/assets/${assetId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "images/logo.png",
          contentType: "image/png",
          size: 11,
          hash: assetHash,
          content: assetBody,
        }),
      }),
    );
    expect(assetResponse.status).toBe(200);

    const noteResponse = await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/notes/${noteId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "Welcome.md",
          content: "# Welcome",
          hash: noteHash,
          metadata: { tags: [{ tag: "#start" }] },
          assets: [{ id: assetId, hash: assetHash }],
          linked_notes: [{ id: linkedNoteId, hash: linkedNoteHash }],
        }),
      }),
    );
    expect(noteResponse.status).toBe(200);

    const notePayload = (await noteResponse.json()) as {
      noteId: string;
      need_upload_assets: Array<{ id: string }>;
      need_upload_notes: Array<{ id: string; hash: string }>;
    };
    expect(notePayload.noteId).toBe(noteId);
    expect(notePayload.need_upload_assets).toEqual([]);
    expect(notePayload.need_upload_notes).toEqual([{ id: linkedNoteId, hash: linkedNoteHash }]);

    const assetRecord = app.db.getAssetByClientId(vault.id, assetId);
    expect(assetRecord?.path).toBe("images/logo.png");
    expect(assetRecord?.object_key).toBe(`assets/${assetId}.png`);

    const noteRecord = app.db.getNoteByClientId(vault.id, noteId);
    expect(noteRecord?.path).toBe("Welcome.md");
    expect(noteRecord?.hash).toBe(noteHash);

    const linkRows = app.db.listNoteLinksBySource(vault.id, noteId);
    expect(linkRows.map((row) => row.target_client_id)).toEqual([linkedNoteId]);

    const storedAssetResponse = await app.fetch(
      new Request(`http://localhost:8080/storage/${assetRecord!.object_key}`, {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(storedAssetResponse.status).toBe(200);
    expect(await storedAssetResponse.text()).toBe("image-bytes");

    const changesAfterPublish = await app.fetch(
      new Request("http://localhost:9090/obsidian/sync/changes", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          notes: [{ id: noteId, hash: noteHash }],
          assets: [{ id: assetId, hash: assetHash }],
        }),
      }),
    );
    const changesAfterPublishPayload = (await changesAfterPublish.json()) as {
      need_upsert: { notes: unknown[]; assets: unknown[] };
    };
    expect(changesAfterPublishPayload.need_upsert.notes).toEqual([]);
    expect(changesAfterPublishPayload.need_upsert.assets).toEqual([]);

    const noteWithoutAssetsResponse = await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/notes/${noteId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "Welcome.md",
          content: "# Welcome Again",
          hash: "hash-note-1-v2",
          metadata: {},
          assets: [],
          linked_notes: [],
        }),
      }),
    );
    expect(noteWithoutAssetsResponse.status).toBe(200);
    expect(app.db.getAssetByClientId(vault.id, assetId)).toBeNull();
    expect(app.db.listNoteLinksBySource(vault.id, noteId)).toEqual([]);

    const deletedAssetResponse = await app.fetch(
      new Request(`http://localhost:8080/storage/${assetRecord!.object_key}`, {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(deletedAssetResponse.status).toBe(404);

    const updatedVault = app.db.getVaultById(vault.id);
    expect(updatedVault?.last_publish_status).toBe("ok");
  });

  test("renders the public site with HTMX note fragments via Bun", async () => {
    const { vault } = await initializeConsoleAndCreateVault(app);

    const assetId = "asset-1";
    const assetHash = "asset-hash";
    const firstNoteId = "note-1";
    const secondNoteId = "note-2";

    await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/assets/${assetId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "images/logo.png",
          contentType: "image/png",
          size: 11,
          hash: assetHash,
          content: Buffer.from("image-bytes").toString("base64"),
        }),
      }),
    );

    await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/notes/${secondNoteId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "Second Note.md",
          content: "# Second Note\nBacklink target",
          hash: "hash-note-2",
          metadata: {},
          assets: [],
          linked_notes: [],
        }),
      }),
    );

    await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/notes/${firstNoteId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "Welcome.md",
          content: "# Welcome\nSee [[Second Note]]\n\n![[images/logo.png]]\n\n$E=mc^2$",
          hash: "hash-note-1",
          metadata: {},
          assets: [{ id: assetId, hash: assetHash }],
          linked_notes: [{ id: secondNoteId, hash: "hash-note-2" }],
        }),
      }),
    );

    const homeResponse = await app.fetch(
      new Request("http://localhost:8080/", {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(homeResponse.status).toBe(200);
    const homeHtml = await homeResponse.text();
    expect(homeHtml).toContain("Docs");
    expect(homeHtml).toContain("Welcome.md");

    const firstNoteResponse = await app.fetch(
      new Request(`http://localhost:8080/${firstNoteId}`, {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(firstNoteResponse.status).toBe(200);
    const firstNoteHtml = await firstNoteResponse.text();
    expect(firstNoteHtml).toContain("Second Note");
    expect(firstNoteHtml).toContain(`href="/${secondNoteId}"`);
    expect(firstNoteHtml).toContain(`src="/storage/assets/${assetId}.png"`);
    expect(firstNoteHtml).toContain("math-inline");

    const secondNoteFragment = await app.fetch(
      new Request(`http://localhost:8080/${secondNoteId}`, {
        headers: {
          Host: "notes.example.com",
          "HX-Request": "true",
          "HX-Current-URL": `http://notes.example.com:8080/${firstNoteId}`,
          "X-From-Note-Id": firstNoteId,
        },
      }),
    );
    expect(secondNoteFragment.status).toBe(200);
    expect(secondNoteFragment.headers.get("hx-push-url")).toBe(`/${firstNoteId}+${secondNoteId}`);
    const fragmentHtml = await secondNoteFragment.text();
    expect(fragmentHtml).toContain(`id="note-${secondNoteId}"`);
    expect(fragmentHtml).toContain("Second Note");

    const secondNotePage = await app.fetch(
      new Request(`http://localhost:8080/${secondNoteId}`, {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    const secondNotePageHtml = await secondNotePage.text();
    expect(secondNotePageHtml).toContain("Links to this note");
    expect(secondNotePageHtml).toContain(`href="/${firstNoteId}"`);
  });

  test("manages logo, root note, and custom head HTML from the Bun console", async () => {
    const { vault, cookie, csrfToken } = await initializeConsoleAndCreateVault(app);
    const noteId = "root-note-1";
    const noteHash = "hash-root-note-1";
    const logoBytes = readFileSync(join(process.cwd(), "server/resources/publics/console/images/logo.png"));

    const noteResponse = await app.fetch(
      new Request(`http://localhost:9090/obsidian/sync/notes/${noteId}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${vault.sync_key}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: "Home.md",
          content: "# Home\nThis is the homepage.",
          hash: noteHash,
          metadata: {},
          assets: [],
          linked_notes: [],
        }),
      }),
    );
    expect(noteResponse.status).toBe(200);

    const logoForm = new FormData();
    logoForm.set("__anti-forgery-token", csrfToken);
    logoForm.set("logo", new File([logoBytes], "logo.png", { type: "image/png" }));

    const logoUploadResponse = await app.fetch(
      new Request(`http://localhost:9090/console/vaults/${vault.id}/logo`, {
        method: "POST",
        headers: { cookie },
        body: logoForm,
      }),
    );
    expect(logoUploadResponse.status).toBe(200);
    const logoUploadPayload = (await logoUploadResponse.json()) as {
      success: boolean;
      logoUrl: string;
    };
    expect(logoUploadPayload.success).toBe(true);

    const vaultWithLogo = app.db.getVaultById(vault.id);
    expect(vaultWithLogo?.logo_object_key).toContain("site/logo/");

    const consoleLogoResponse = await app.fetch(
      new Request(`http://localhost:9090${logoUploadPayload.logoUrl}`, {
        headers: { cookie },
      }),
    );
    expect(consoleLogoResponse.status).toBe(200);
    expect(consoleLogoResponse.headers.get("content-type")).toBe("image/png");
    expect(Buffer.from(await consoleLogoResponse.arrayBuffer())).toEqual(logoBytes);

    const rootNoteForm = new FormData();
    rootNoteForm.set("__anti-forgery-token", csrfToken);
    rootNoteForm.set("rootNoteId", noteId);

    const rootNoteResponse = await app.fetch(
      new Request(`http://localhost:9090/console/vaults/${vault.id}/root-note`, {
        method: "PUT",
        headers: { cookie },
        body: rootNoteForm,
      }),
    );
    expect(rootNoteResponse.status).toBe(204);

    const customHeadHtml = '<meta name="x-mdbrain-test" content="enabled">';
    const customHtmlForm = new FormData();
    customHtmlForm.set("__anti-forgery-token", csrfToken);
    customHtmlForm.set("customHeadHtml", customHeadHtml);

    const customHtmlResponse = await app.fetch(
      new Request(`http://localhost:9090/console/vaults/${vault.id}/custom-head-html`, {
        method: "PUT",
        headers: { cookie },
        body: customHtmlForm,
      }),
    );
    expect(customHtmlResponse.status).toBe(200);
    expect(await customHtmlResponse.json()).toEqual({ success: true });

    const notesResponse = await app.fetch(
      new Request(`http://localhost:9090/console/vaults/${vault.id}/notes?q=Home`, {
        headers: { cookie },
      }),
    );
    expect(notesResponse.status).toBe(200);
    const notesPayload = (await notesResponse.json()) as {
      success: boolean;
      notes: Array<{ clientId: string; path: string }>;
    };
    expect(notesPayload.success).toBe(true);
    expect(notesPayload.notes).toEqual([{ clientId: noteId, path: "Home.md", hash: noteHash, mtime: null }]);

    const rootNoteSelectorResponse = await app.fetch(
      new Request(`http://localhost:9090/console/vaults/${vault.id}/root-note-selector`, {
        headers: { cookie },
      }),
    );
    expect(rootNoteSelectorResponse.status).toBe(200);
    const rootNoteSelectorHtml = await rootNoteSelectorResponse.text();
    expect(rootNoteSelectorHtml).toContain(`note-selector-${vault.id}`);
    expect(rootNoteSelectorHtml).toContain("Home.md");

    const publicHomeResponse = await app.fetch(
      new Request("http://localhost:8080/", {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(publicHomeResponse.status).toBe(200);
    const publicHomeHtml = await publicHomeResponse.text();
    expect(publicHomeHtml).toContain("This is the homepage.");
    expect(publicHomeHtml).toContain(customHeadHtml);
    expect(publicHomeHtml).toContain(`/storage/${vaultWithLogo!.logo_object_key}`);

    const faviconResponse = await app.fetch(
      new Request("http://localhost:8080/favicon.ico", {
        headers: {
          Host: "notes.example.com",
        },
      }),
    );
    expect(faviconResponse.status).toBe(302);
    expect(faviconResponse.headers.get("location")).toBe(`/storage/${vaultWithLogo!.logo_object_key}`);
  });

  test("renders S3-backed public asset URLs and redirects storage requests", async () => {
    const s3DataPath = mkdtempSync(join(tmpdir(), "mdbrain-bun-s3-"));
    const s3App = createMdbrainWebApp({
      cwd: process.cwd(),
      env: {
        ...process.env,
        DATA_PATH: s3DataPath,
        SESSION_SECRET: "test-session-secret",
        HOST: "127.0.0.1",
        APP_PORT: "8080",
        CONSOLE_PORT: "9090",
        STORAGE_TYPE: "s3",
        S3_ENDPOINT: "http://localhost:9000",
        S3_ACCESS_KEY: "rustfsadmin",
        S3_SECRET_KEY: "rustfsadmin",
        S3_REGION: "us-east-1",
        S3_BUCKET: "mdbrain",
        S3_PUBLIC_URL: "https://cdn.example.com",
      },
    });

    try {
      s3App.db.createInitialConsoleUser({
        tenantId: "tenant-1",
        tenantName: "Acme Notes",
        userId: "user-1",
        username: "console@example.com",
        passwordHash: "not-used",
      });
      s3App.db.createVault({
        id: "vault-1",
        tenantId: "tenant-1",
        name: "Docs",
        domain: "notes.example.com",
        syncKey: "sync-key-1",
      });
      s3App.db.upsertAsset({
        id: "asset-row-1",
        tenantId: "tenant-1",
        vaultId: "vault-1",
        clientId: "asset-1",
        path: "images/logo.png",
        objectKey: "assets/asset-1.png",
        sizeBytes: 11,
        contentType: "image/png",
        md5: "asset-hash-1",
      });
      s3App.db.upsertNote({
        id: "note-row-1",
        tenantId: "tenant-1",
        vaultId: "vault-1",
        clientId: "note-1",
        path: "Welcome.md",
        content: "# Welcome\n\n![[images/logo.png]]",
        metadata: null,
        hash: "note-hash-1",
      });
      s3App.db.updateVaultRootNote("vault-1", "note-1");
      s3App.db.updateVaultLogo("vault-1", "site/logo/logo-hash.png");

      const noteResponse = await s3App.fetch(
        new Request("http://localhost:8080/note-1", {
          headers: {
            Host: "notes.example.com",
          },
        }),
      );
      expect(noteResponse.status).toBe(200);
      const noteHtml = await noteResponse.text();
      expect(noteHtml).toContain("https://cdn.example.com/mdbrain/vault1/assets/asset-1.png");

      const faviconResponse = await s3App.fetch(
        new Request("http://localhost:8080/favicon.ico", {
          headers: {
            Host: "notes.example.com",
          },
        }),
      );
      expect(faviconResponse.status).toBe(302);
      expect(faviconResponse.headers.get("location")).toBe(
        "https://cdn.example.com/mdbrain/vault1/site/logo/logo-hash.png",
      );

      const storageRedirectResponse = await s3App.fetch(
        new Request("http://localhost:8080/storage/assets/asset-1.png", {
          headers: {
            Host: "notes.example.com",
          },
        }),
      );
      expect(storageRedirectResponse.status).toBe(302);
      expect(storageRedirectResponse.headers.get("location")).toBe(
        "https://cdn.example.com/mdbrain/vault1/assets/asset-1.png",
      );
    } finally {
      s3App.close();
      rmSync(s3DataPath, { recursive: true, force: true });
    }
  });
});
