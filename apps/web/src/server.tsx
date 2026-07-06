/** @jsxImportSource ../../../packages/ui/src */
import { randomUUID } from "node:crypto";
import { normalize, resolve, sep } from "node:path";
import {
  assetObjectKey,
  type ConsoleSession,
  ConsoleDatabase,
  createAnonymousSession,
  extensionFromPath,
  loadRuntimeConfig,
  logoObjectKey,
  parseSessionCookie,
  putStorageObject,
  removeStorageObject,
  removeVaultStorage,
  sha256Bytes,
  serializeExpiredSessionCookie,
  serializeSessionCookie,
  storageObjectPath,
  storagePublicUrl,
  type SyncAssetRequest,
  type SyncChangesRequest,
  type SyncChangesResponse,
  type SyncNoteRequest,
  type VaultInfoResponse,
} from "../../../packages/shared/src/index";
import type {
  AssetLookupRecord,
  NoteLookupRecord,
  NoteRecord,
  VaultRecord,
} from "../../../packages/shared/src/console-db";
import { renderDocument, renderToString } from "../../../packages/ui/src/index";
import {
  PublicHomePage,
  PublicNoteFragment,
  PublicNotePage,
  type PublicNoteViewModel,
  type PublicVaultViewModel,
} from "./components/app-pages";
import {
  ConsolePage,
  ErrorFragment,
  InitPage,
  LoginPage,
  RootNoteSelector,
  VaultList,
} from "./components/pages";
import { createMarkdownContext, noteDescription, noteTitle, renderMarkdown } from "./markdown";

interface MdbrainWebOptions {
  cwd?: string;
  env?: Record<string, string | undefined>;
  migrationFile?: string;
  publicsDir?: string;
}

interface RequestContext {
  readonly request: Request;
  readonly url: URL;
  readonly db: ConsoleDatabase;
  readonly config: ReturnType<typeof loadRuntimeConfig>;
  session: ConsoleSession | null;
  readonly isHtmx: boolean;
  readonly isStateChanging: boolean;
  sessionDirty: boolean;
}

interface MdbrainWebApp {
  readonly config: ReturnType<typeof loadRuntimeConfig>;
  readonly db: ConsoleDatabase;
  fetch(request: Request): Promise<Response>;
  close(): void;
}

type StorageFile = ReturnType<typeof Bun.file>;

const NOINDEX_HEADER = "noindex, nofollow";
const HTML_CONTENT_TYPE = "text/html; charset=utf-8";
const JSON_CONTENT_TYPE = "application/json; charset=utf-8";
const TEXT_CONTENT_TYPE = "text/plain; charset=utf-8";
const BINARY_CONTENT_TYPE = "application/octet-stream";
const IMMUTABLE_CACHE_CONTROL = "public, max-age=31536000, immutable";
const MAX_PUBLISH_ERROR_MESSAGE_LENGTH = 400;
const MAX_CUSTOM_HEAD_HTML_SIZE = 65_536;
const MAX_LOGO_FILE_SIZE = 2 * 1024 * 1024;
const MIN_LOGO_DIMENSION = 128;
const ALLOWED_LOGO_CONTENT_TYPES = new Set(["image/png", "image/jpeg", "image/jpg"]);
const CSRF_ERROR_MESSAGE = "CSRF token missing or incorrect";
const LOGO_NOT_FOUND_MESSAGE = "Logo not found";
const MISSING_REQUIRED_FIELDS_MESSAGE = "Missing required fields";
const SESSION_EXPIRED_MESSAGE = "Session expired. Please sign in again.";

function redirect(location: string, status = 302) {
  return new Response(null, {
    status,
    headers: {
      Location: location,
    },
  });
}

function htmlResponse(html: string, status = 200) {
  return new Response(html, {
    status,
    headers: {
      "Content-Type": HTML_CONTENT_TYPE,
    },
  });
}

function htmlFragment(message: string, status = 422) {
  return htmlResponse(renderToString(<ErrorFragment message={message} />), status);
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": JSON_CONTENT_TYPE,
    },
  });
}

function jsonError(status: number, error: string) {
  return jsonResponse({ success: false, error }, status);
}

function noContentResponse(headers?: HeadersInit) {
  return new Response(null, {
    status: 204,
    headers,
  });
}

function htmxRedirectResponse(location: string) {
  return noContentResponse({
    "HX-Redirect": location,
  });
}

function finalizeConsoleResponse(
  response: Response,
  context: RequestContext,
  clearSession = false,
) {
  const headers = new Headers(response.headers);
  headers.set("X-Robots-Tag", NOINDEX_HEADER);

  if (clearSession) {
    headers.set(
      "Set-Cookie",
      serializeExpiredSessionCookie(context.config.secureCookies),
    );
  } else if (context.session && context.sessionDirty) {
    headers.set(
      "Set-Cookie",
      serializeSessionCookie(
        context.session,
        context.config.sessionSecret,
        context.config.secureCookies,
      ),
    );
  }

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

function finalizeConsoleHtmlError(
  context: RequestContext,
  message: string,
  status = 422,
) {
  return finalizeConsoleResponse(htmlFragment(message, status), context);
}

function finalizeConsoleJsonError(
  context: RequestContext,
  status: number,
  error: string,
) {
  return finalizeConsoleResponse(jsonError(status, error), context);
}

function finalizeConsoleNoContent(
  context: RequestContext,
  headers?: HeadersInit,
) {
  return finalizeConsoleResponse(noContentResponse(headers), context);
}

async function readForm(request: Request) {
  const formData = await request.formData();
  return Object.fromEntries(
    Array.from(formData.entries()).map(([key, value]) => [key, typeof value === "string" ? value : String(value)]),
  ) as Record<string, string>;
}

async function readJson<T>(request: Request) {
  return (await request.json()) as T;
}

function isAuthenticated(session: ConsoleSession | null): session is ConsoleSession & {
  userId: string;
  tenantId: string;
} {
  return Boolean(session?.userId && session.tenantId);
}

function validateCsrf(context: RequestContext, params: Record<string, string>) {
  if (!context.isStateChanging) {
    return true;
  }

  const providedToken =
    context.request.headers.get("x-csrf-token") ?? params.__anti_forgery_token ?? params["__anti-forgery-token"];

  return Boolean(context.session?.csrfToken && providedToken === context.session.csrfToken);
}

function cleanDomain(domain: string) {
  return domain.trim().replace(/^https?:\/\//i, "").replace(/\/+$/, "");
}

function truncateString(value: string | null | undefined, maxLength = MAX_PUBLISH_ERROR_MESSAGE_LENGTH) {
  const normalized = value?.trim();
  if (!normalized) {
    return "";
  }

  return normalized.slice(0, maxLength);
}

function maskSyncKey(syncKey: string) {
  return `${syncKey.slice(0, 8)}******${syncKey.slice(-8)}`;
}

function encodePathSegments(path: string) {
  return path
    .split("/")
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}

function consoleAssetUrl(vaultId: string, objectKey: string) {
  return `/console/storage/${encodeURIComponent(vaultId)}/${encodePathSegments(objectKey)}`;
}

function storageFileResponse(file: StorageFile) {
  return new Response(file, {
    headers: {
      "Content-Type": file.type || BINARY_CONTENT_TYPE,
      "Cache-Control": IMMUTABLE_CACHE_CONTROL,
    },
  });
}

function resolveConsoleAssetUrl(
  context: RequestContext,
  vaultId: string,
  objectKey: string,
) {
  return context.config.storageType === "s3"
    ? storagePublicUrl(context.config, vaultId, objectKey)
    : consoleAssetUrl(vaultId, objectKey);
}

function formatStorageSize(sizeBytes: number) {
  if (sizeBytes < 1024) {
    return `${sizeBytes} B`;
  }

  const units = ["KB", "MB", "GB", "TB"];
  let value = sizeBytes / 1024;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}

function readUint16Be(bytes: Uint8Array, offset: number) {
  return (bytes[offset]! << 8) | bytes[offset + 1]!;
}

function readUint32Be(bytes: Uint8Array, offset: number) {
  return (
    bytes[offset]! * 0x1000000 +
    bytes[offset + 1]! * 0x10000 +
    bytes[offset + 2]! * 0x100 +
    bytes[offset + 3]!
  );
}

function readImageDimensions(bytes: Uint8Array, contentType: string) {
  if ((contentType === "image/png" || contentType === "image/x-png") && bytes.length >= 24) {
    const pngSignature = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
    const signatureValid = pngSignature.every((value, index) => bytes[index] === value);
    const ihdrValid =
      bytes[12] === 0x49 &&
      bytes[13] === 0x48 &&
      bytes[14] === 0x44 &&
      bytes[15] === 0x52;

    if (signatureValid && ihdrValid) {
      return {
        width: readUint32Be(bytes, 16),
        height: readUint32Be(bytes, 20),
      };
    }
  }

  if ((contentType === "image/jpeg" || contentType === "image/jpg") && bytes.length >= 4) {
    if (bytes[0] !== 0xff || bytes[1] !== 0xd8) {
      return null;
    }

    let offset = 2;
    while (offset + 1 < bytes.length) {
      if (bytes[offset] !== 0xff) {
        offset += 1;
        continue;
      }

      let markerOffset = offset + 1;
      while (markerOffset < bytes.length && bytes[markerOffset] === 0xff) {
        markerOffset += 1;
      }

      if (markerOffset >= bytes.length) {
        return null;
      }

      const marker = bytes[markerOffset]!;
      offset = markerOffset + 1;

      if (marker === 0xd8 || marker === 0xd9 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
        continue;
      }

      if (offset + 1 >= bytes.length) {
        return null;
      }

      const segmentLength = readUint16Be(bytes, offset);
      if (segmentLength < 2 || offset + segmentLength > bytes.length) {
        return null;
      }

      const isStartOfFrame =
        (marker >= 0xc0 && marker <= 0xc3) ||
        (marker >= 0xc5 && marker <= 0xc7) ||
        (marker >= 0xc9 && marker <= 0xcb) ||
        (marker >= 0xcd && marker <= 0xcf);

      if (isStartOfFrame && segmentLength >= 7) {
        return {
          height: readUint16Be(bytes, offset + 3),
          width: readUint16Be(bytes, offset + 5),
        };
      }

      offset += segmentLength;
    }
  }

  return null;
}

function mapVaultViewModel(db: ConsoleDatabase, vault: ReturnType<ConsoleDatabase["listVaultsByTenant"]>[number]) {
  return {
    id: vault.id,
    name: vault.name,
    domain: vault.domain ?? "",
    syncKey: vault.sync_key,
    maskedKey: maskSyncKey(vault.sync_key),
    lastPublishStatus: vault.last_publish_status ?? "never",
    lastPublishAt: vault.last_publish_at,
    lastPublishErrorMessage: vault.last_publish_error_message,
    rootNoteId: vault.root_note_id,
    notes: db.searchNotesByVault(vault.id, "").map((note) => ({
      clientId: note.client_id,
      path: note.path,
    })),
    storageSize: formatStorageSize(db.getVaultStorageSize(vault.id)),
    logoUrl: vault.logo_object_key ? consoleAssetUrl(vault.id, vault.logo_object_key) : null,
    customHeadHtml: vault.custom_head_html ?? "",
    createdAt: vault.created_at,
    hasCustomHtml: Boolean(vault.custom_head_html?.trim()),
  };
}

function getHealthTokenFromRequest(request: Request, url: URL) {
  return request.headers.get("x-health-token") ?? url.searchParams.get("token");
}

function getBearerToken(request: Request) {
  const header = request.headers.get("authorization");
  if (!header) {
    return null;
  }

  const match = header.match(/^Bearer\s+(.+)$/i);
  return match?.[1] ?? null;
}

function parseHostDomain(host: string | null) {
  const normalized = host?.trim();
  if (!normalized) {
    return { error: "missing" as const };
  }

  if (/\s|\//.test(normalized)) {
    return { error: "invalid" as const };
  }

  if (normalized.startsWith("[")) {
    const closeIndex = normalized.indexOf("]");
    if (closeIndex <= 1) {
      return { error: "invalid" as const };
    }

    const domain = normalized.slice(1, closeIndex);
    const rest = normalized.slice(closeIndex + 1);
    if (!rest) {
      return { domain };
    }

    if (!rest.startsWith(":")) {
      return { error: "invalid" as const };
    }

    const port = Number.parseInt(rest.slice(1), 10);
    return Number.isFinite(port) && port >= 1 && port <= 65535
      ? { domain }
      : { error: "invalid" as const };
  }

  const parts = normalized.split(":");
  if (parts.length > 2) {
    return { error: "invalid" as const };
  }

  const [domain, port] = parts;
  if (!domain || !/^[A-Za-z0-9._-]+$/.test(domain)) {
    return { error: "invalid" as const };
  }

  if (!port) {
    return { domain };
  }

  const portNumber = Number.parseInt(port, 10);
  return Number.isFinite(portNumber) && portNumber >= 1 && portNumber <= 65535
    ? { domain }
    : { error: "invalid" as const };
}

async function servePublicFile(pathname: string, publicsDir: string) {
  const requestedPath = pathname.replace(/^\/publics\//, "");
  const absolutePath = resolve(publicsDir, requestedPath);
  const normalizedRoot = `${normalize(publicsDir)}${sep}`;
  const file = Bun.file(absolutePath);

  if (!absolutePath.startsWith(normalizedRoot) || !(await file.exists())) {
    return new Response("Not found", { status: 404 });
  }

  return new Response(file);
}

function faviconResponse(faviconFile: string) {
  return new Response(Bun.file(faviconFile));
}

function notFound() {
  return new Response("Not found", { status: 404 });
}

function missingHostResponse(error: "missing" | "invalid" | "unbound") {
  if (error === "unbound") {
    return new Response("Forbidden", {
      status: 403,
      headers: {
        "Content-Type": TEXT_CONTENT_TYPE,
        "Cache-Control": "no-store",
      },
    });
  }

  return new Response("Bad request", {
    status: 400,
    headers: {
      "Content-Type": TEXT_CONTENT_TYPE,
      "Cache-Control": "no-store",
    },
  });
}

function getVaultByHost(context: RequestContext) {
  const parsed = parseHostDomain(context.request.headers.get("host"));
  if ("error" in parsed) {
    return { response: missingHostResponse(parsed.error) };
  }

  const vault = context.db.getVaultByDomain(parsed.domain);
  if (!vault) {
    return { response: missingHostResponse("unbound") };
  }

  return { vault };
}

function getSyncVault(context: RequestContext) {
  const syncKey = getBearerToken(context.request);
  if (!syncKey) {
    return { response: jsonError(401, "Missing authorization header") };
  }

  const vault = context.db.getVaultBySyncKey(syncKey);
  if (!vault) {
    return { response: jsonError(401, "Invalid publish key") };
  }

  return { vault };
}

function recordVaultPublishResult(
  context: RequestContext,
  vaultId: string,
  response: Response,
  explicitMessage?: string,
) {
  if (response.status < 400) {
    context.db.recordVaultPublishSuccess(vaultId);
    return response;
  }

  const message =
    truncateString(explicitMessage) ||
    truncateString(response.statusText) ||
    "Request failed";

  const code = response.status >= 500 ? "server_error" : response.status >= 400 ? "bad_request" : "error";
  context.db.recordVaultPublishError(vaultId, code, message);
  return response;
}

function recordVaultPublishException(context: RequestContext, vaultId: string, error: unknown) {
  context.db.recordVaultPublishError(
    vaultId,
    "server_error",
    truncateString(error instanceof Error ? error.message : "Internal server error") || "Internal server error",
  );
}

function normalizeHashEntries(entries: Array<{ id?: string; hash?: string }> | undefined | null) {
  return (entries ?? [])
    .filter((entry) => typeof entry?.id === "string" && typeof entry?.hash === "string")
    .map((entry) => ({ id: entry.id!.trim(), hash: entry.hash!.trim() }))
    .filter((entry) => entry.id && entry.hash);
}

function trimmedString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function parseSyncNotePayload(noteId: string, body: SyncNoteRequest) {
  const normalizedNoteId = noteId.trim();
  const notePath = trimmedString(body.path);
  const noteHash = trimmedString(body.hash);

  if (!normalizedNoteId) {
    return { error: "Missing note id" as const };
  }

  if (!notePath) {
    return { error: "Missing note path" as const };
  }

  if (!noteHash) {
    return { error: "Missing note hash" as const };
  }

  if (typeof body.content !== "string") {
    return { error: "Missing note content" as const };
  }

  if (!Array.isArray(body.assets)) {
    return { error: "Missing assets" as const };
  }

  if (!Array.isArray(body.linked_notes)) {
    return { error: "Missing linked notes" as const };
  }

  return {
    noteId: normalizedNoteId,
    notePath,
    noteHash,
    content: body.content,
    metadata: body.metadata,
    assetEntries: normalizeHashEntries(body.assets),
    linkedEntries: normalizeHashEntries(body.linked_notes),
  };
}

function parseSyncAssetPayload(assetId: string, body: SyncAssetRequest) {
  const normalizedAssetId = assetId.trim();
  const assetPath = trimmedString(body.path);
  const assetHash = trimmedString(body.hash);
  const contentType = trimmedString(body.contentType);

  if (!normalizedAssetId) {
    return { error: "Missing asset id" as const };
  }

  if (!assetPath) {
    return { error: "Missing asset path" as const };
  }

  if (!assetHash) {
    return { error: "Missing asset hash" as const };
  }

  if (!contentType) {
    return { error: "Missing asset contentType" as const };
  }

  if (typeof body.content !== "string" || !body.content) {
    return { error: "Missing asset content" as const };
  }

  return {
    assetId: normalizedAssetId,
    assetPath,
    assetHash,
    contentType,
    content: body.content,
    size: body.size,
  };
}

async function deleteAsset(context: RequestContext, vaultId: string, assetId: string) {
  const asset = context.db.getAssetByClientId(vaultId, assetId);
  if (!asset) {
    return;
  }

  await removeStorageObject(context.config, vaultId, asset.object_key);
  context.db.deleteAssetAndRelations(vaultId, assetId);
}

async function deleteNote(context: RequestContext, vaultId: string, noteId: string) {
  const existingRefs = context.db.listAssetRefsByNote(vaultId, noteId);
  context.db.deleteNoteAndRelations(vaultId, noteId);

  for (const ref of existingRefs) {
    if (context.db.countAssetRefs(vaultId, ref.asset_client_id) === 0) {
      await deleteAsset(context, vaultId, ref.asset_client_id);
    }
  }
}

async function syncNoteAssetRefs(
  context: RequestContext,
  vaultId: string,
  noteId: string,
  assetIds: string[],
) {
  const existingRefs = context.db.listAssetRefsByNote(vaultId, noteId);
  const previousIds = new Set(existingRefs.map((ref) => ref.asset_client_id));
  const nextIds = Array.from(new Set(assetIds.filter(Boolean)));

  context.db.replaceNoteAssetRefs(vaultId, noteId, nextIds);

  for (const previousId of previousIds) {
    if (nextIds.includes(previousId)) {
      continue;
    }

    if (context.db.countAssetRefs(vaultId, previousId) === 0) {
      await deleteAsset(context, vaultId, previousId);
    }
  }
}

function syncNoteLinks(
  context: RequestContext,
  vaultId: string,
  noteId: string,
  linkedNotes: Array<{ id: string; hash: string }>,
) {
  context.db.replaceNoteLinks(
    vaultId,
    noteId,
    Array.from(new Set(linkedNotes.map((entry) => entry.id))).map((targetClientId) => ({
      id: randomUUID(),
      targetClientId,
    })),
  );
}

async function serveStoredAsset(context: RequestContext, objectKey: string) {
  const hostMatch = getVaultByHost(context);
  if ("response" in hostMatch) {
    return hostMatch.response;
  }

  const assetResponse = await serveVaultObject(context, hostMatch.vault.id, objectKey);
  return assetResponse ?? notFound();
}

function requestPort(url: URL) {
  if (!url.port) {
    return null;
  }

  const port = Number.parseInt(url.port, 10);
  return Number.isFinite(port) ? port : null;
}

function isConsoleRequest(context: RequestContext) {
  if (context.config.appPort === context.config.consolePort) {
    return true;
  }

  const port = requestPort(context.url);
  if (port != null) {
    return port === context.config.consolePort;
  }

  return (
    context.url.pathname.startsWith("/console") ||
    context.url.pathname.startsWith("/obsidian")
  );
}

function isAppRequest(context: RequestContext) {
  if (context.config.appPort === context.config.consolePort) {
    return true;
  }

  const port = requestPort(context.url);
  if (port != null) {
    return port === context.config.appPort;
  }

  return !isConsoleRequest(context);
}

function parsePathIds(pathname: string) {
  if (!pathname || pathname === "/") {
    return [];
  }

  return pathname
    .replace(/^\/+/, "")
    .split("+")
    .map((segment) => decodeURIComponent(segment))
    .filter(Boolean);
}

function buildPushUrl(
  currentUrl: string | null,
  fromNoteId: string | null,
  targetNoteId: string,
  rootNoteId: string | null,
) {
  const currentPath = currentUrl
    ? currentUrl.replace(/^https?:\/\/[^/]+/i, "").replace(/\?.*$/, "")
    : "/";

  if (currentPath === "/") {
    return rootNoteId ? `/${rootNoteId}+${targetNoteId}` : `/${targetNoteId}`;
  }

  const pathParts = currentPath.replace(/^\/+/, "").split("+").filter(Boolean);
  if (!fromNoteId) {
    return `/${[...pathParts, targetNoteId].join("+")}`;
  }

  const currentIndex = pathParts.indexOf(fromNoteId);
  const keptParts = currentIndex >= 0 ? pathParts.slice(0, currentIndex + 1) : pathParts;
  return `/${[...keptParts, targetNoteId].join("+")}`;
}

function mapPublicVault(
  config: ReturnType<typeof loadRuntimeConfig>,
  vault: VaultRecord,
): PublicVaultViewModel {
  return {
    id: vault.id,
    name: vault.name,
    domain: vault.domain ?? "",
    logoUrl: vault.logo_object_key ? storagePublicUrl(config, vault.id, vault.logo_object_key) : null,
    faviconVersion: vault.logo_object_key ?? null,
    customHeadHtml: vault.custom_head_html ?? null,
  };
}

function buildPublicNoteView(
  context: RequestContext,
  vaultId: string,
  note: Pick<NoteRecord, "client_id" | "path" | "content" | "updated_at">,
  notes: NoteLookupRecord[],
  assets: AssetLookupRecord[],
): PublicNoteViewModel {
  const markdownContext = createMarkdownContext(notes, assets);
  const backlinks = context.db.listBacklinksWithNotes(vaultId, note.client_id).map((backlink) => ({
    clientId: backlink.client_id,
    title: noteTitle(backlink.content, backlink.path),
    description: noteDescription(backlink.content) ?? "",
  }));

  return {
    clientId: note.client_id,
    title: noteTitle(note.content, note.path),
    htmlContent: renderMarkdown(note.content, markdownContext),
    path: note.path,
    updatedAt: note.updated_at,
    backlinks,
  };
}

function publicPageContext(context: RequestContext, vaultId: string) {
  const notes = context.db.listNotesForLookup(vaultId);
  const assets = context.db.listAssetsForLookup(vaultId).map((asset) => ({
    ...asset,
    public_url: storagePublicUrl(context.config, vaultId, asset.object_key),
  }));
  return { notes, assets };
}

function renderPublicNoteFragment(note: PublicNoteViewModel) {
  return renderToString(<PublicNoteFragment note={note} />);
}

function listConsoleVaults(context: RequestContext) {
  return context.db.listVaultsByTenant(context.session!.tenantId).map((vault) => mapVaultViewModel(context.db, vault));
}

function sessionExpired(context: RequestContext) {
  if (context.isHtmx) {
    return finalizeConsoleHtmlError(context, SESSION_EXPIRED_MESSAGE, 401);
  }

  return finalizeConsoleResponse(redirect("/console/login"), context);
}

async function handleConsoleInit(context: RequestContext) {
  if (context.request.method === "GET") {
    if (context.db.hasAnyUser()) {
      return finalizeConsoleResponse(redirect("/console/login"), context);
    }

    return finalizeConsoleResponse(
      htmlResponse(renderDocument(<InitPage csrfToken={context.session!.csrfToken} />)),
      context,
    );
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  if (context.db.hasAnyUser()) {
    return finalizeConsoleHtmlError(context, "System already initialized", 403);
  }

  const tenantName = params["tenant-name"]?.trim();
  const username = params.username?.trim();
  const password = params.password ?? "";

  if (!tenantName || !username || !password) {
    return finalizeConsoleHtmlError(context, MISSING_REQUIRED_FIELDS_MESSAGE);
  }

  if (context.db.getUserByUsername(username)) {
    return finalizeConsoleHtmlError(context, "Username already exists");
  }

  if (password.length < 8) {
    return finalizeConsoleHtmlError(context, "Password must be at least 8 characters");
  }

  const passwordHash = await Bun.password.hash(password);
  context.db.createInitialConsoleUser({
    tenantId: randomUUID(),
    tenantName,
    userId: randomUUID(),
    username,
    passwordHash,
  });

  return finalizeConsoleResponse(
    htmxRedirectResponse("/console/login"),
    context,
  );
}

async function handleConsoleLogin(context: RequestContext) {
  if (context.request.method === "GET") {
    if (!context.db.hasAnyUser()) {
      return finalizeConsoleResponse(redirect("/console/init"), context);
    }

    if (isAuthenticated(context.session)) {
      return finalizeConsoleResponse(redirect("/console"), context);
    }

    return finalizeConsoleResponse(
      htmlResponse(renderDocument(<LoginPage csrfToken={context.session!.csrfToken} />)),
      context,
    );
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  const username = params.username?.trim();
  const password = params.password ?? "";
  if (!username || !password) {
    return finalizeConsoleHtmlError(context, MISSING_REQUIRED_FIELDS_MESSAGE);
  }

  const user = context.db.getUserByUsername(username);
  if (!user || !(await Bun.password.verify(password, user.password_hash))) {
    return finalizeConsoleHtmlError(context, "Invalid username or password");
  }

  context.session = {
    userId: user.id,
    tenantId: user.tenant_id,
    csrfToken: context.session?.csrfToken ?? createAnonymousSession().csrfToken,
  };
  context.sessionDirty = true;

  return finalizeConsoleResponse(
    htmxRedirectResponse("/console"),
    context,
  );
}

async function handleConsoleHome(context: RequestContext) {
  if (!isAuthenticated(context.session)) {
    return sessionExpired(context);
  }

  const tenant = context.db.getTenant(context.session.tenantId);
  if (!tenant) {
    return sessionExpired(context);
  }

  const vaults = listConsoleVaults(context);

  return finalizeConsoleResponse(
    htmlResponse(
      renderDocument(
        <ConsolePage
          csrfToken={context.session.csrfToken}
          tenant={{ id: tenant.id, name: tenant.name }}
          vaults={vaults}
        />,
      ),
    ),
    context,
  );
}

async function handleVaultList(context: RequestContext) {
  if (!isAuthenticated(context.session)) {
    return sessionExpired(context);
  }

  const vaults = listConsoleVaults(context);
  return finalizeConsoleResponse(
    htmlResponse(renderToString(<VaultList vaults={vaults} />)),
    context,
  );
}

async function handleCreateVault(context: RequestContext) {
  if (!isAuthenticated(context.session)) {
    return sessionExpired(context);
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  const name = params.name?.trim();
  const domain = cleanDomain(params.domain ?? "");

  if (!name || !domain) {
    return finalizeConsoleHtmlError(context, "Site name and domain are required");
  }

  if (context.db.getVaultByDomain(domain)) {
    return finalizeConsoleHtmlError(context, "Domain already in use");
  }

  context.db.createVault({
    id: randomUUID(),
    tenantId: context.session.tenantId,
    name,
    domain,
    syncKey: randomUUID(),
  });

  return finalizeConsoleNoContent(context);
}

async function handleUpdateVault(context: RequestContext, vaultId: string) {
  if (!isAuthenticated(context.session)) {
    return sessionExpired(context);
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  const vault = context.db.getVaultById(vaultId);
  if (!vault || vault.tenant_id !== context.session.tenantId) {
    return finalizeConsoleHtmlError(context, "Vault not found", 404);
  }

  const name = params.name?.trim();
  const domain = cleanDomain(params.domain ?? "");
  if (!name || !domain) {
    return finalizeConsoleHtmlError(context, "Site name and domain are required");
  }

  const existingDomain = context.db.getVaultByDomain(domain);
  if (existingDomain && existingDomain.id !== vaultId) {
    return finalizeConsoleHtmlError(context, "Domain already in use");
  }

  context.db.updateVault(vaultId, name, domain);
  return finalizeConsoleNoContent(context);
}

function requireVaultOwnership(
  context: RequestContext,
  vaultId: string,
  notFoundMessage = "Vault not found",
) {
  if (!isAuthenticated(context.session)) {
    return { response: sessionExpired(context) };
  }

  const vault = context.db.getVaultById(vaultId);
  if (!vault || vault.tenant_id !== context.session.tenantId) {
    return {
      response: finalizeConsoleHtmlError(context, notFoundMessage, 404),
    };
  }

  return { vault };
}

function requireVaultOwnershipJson(context: RequestContext, vaultId: string) {
  if (!isAuthenticated(context.session)) {
    return {
      response: finalizeConsoleJsonError(context, 401, "Session expired"),
    };
  }

  const vault = context.db.getVaultById(vaultId);
  if (!vault || vault.tenant_id !== context.session.tenantId) {
    return {
      response: finalizeConsoleJsonError(context, 404, "Vault not found"),
    };
  }

  return { vault };
}

async function handleDeleteVault(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnership(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const params = await readForm(context.request).catch(() => ({}));
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  await removeVaultStorage(context.config, vaultId);
  context.db.deleteVault(vaultId);
  return finalizeConsoleNoContent(context);
}

async function handleRenewSyncKey(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnership(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const params = await readForm(context.request).catch(() => ({}));
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  context.db.updateVaultSyncKey(vaultId, randomUUID());
  return finalizeConsoleNoContent(context);
}

async function handleVaultNotes(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnershipJson(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const query = context.url.searchParams.get("q") ?? "";
  return finalizeConsoleResponse(
    jsonResponse({
      success: true,
      notes: context.db.searchNotesByVault(vaultId, query).map((note) => ({
        clientId: note.client_id,
        path: note.path,
        hash: note.hash,
        mtime: note.mtime,
      })),
    }),
    context,
  );
}

async function handleRootNoteSelector(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnership(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const notes = context.db.searchNotesByVault(vaultId, "").map((note) => ({
    clientId: note.client_id,
    path: note.path,
  }));

  return finalizeConsoleResponse(
    htmlResponse(
      renderToString(
        <RootNoteSelector
          vaultId={vaultId}
          rootNoteId={ownership.vault.root_note_id}
          notes={notes}
        />,
      ),
    ),
    context,
  );
}

async function handleUpdateRootNote(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnership(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  const rootNoteId = params.rootNoteId?.trim();
  if (!rootNoteId) {
    return finalizeConsoleHtmlError(context, "Missing root note id");
  }

  context.db.updateVaultRootNote(vaultId, rootNoteId);
  return finalizeConsoleNoContent(context);
}

async function handleUpdateCustomHeadHtml(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnershipJson(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleJsonError(context, 403, CSRF_ERROR_MESSAGE);
  }

  const customHeadHtml = params.customHeadHtml ?? "";
  if (customHeadHtml.length > MAX_CUSTOM_HEAD_HTML_SIZE) {
    return finalizeConsoleJsonError(context, 422, "Custom HTML exceeds maximum size of 64KB");
  }

  context.db.updateVaultCustomHeadHtml(vaultId, customHeadHtml);
  return finalizeConsoleResponse(jsonResponse({ success: true }), context);
}

async function serveLocalVaultObject(
  context: RequestContext,
  vaultId: string,
  objectKey: string,
) {
  try {
    const filePath = storageObjectPath(context.config.localStoragePath, vaultId, objectKey);
    const file = Bun.file(filePath);
    if (!(await file.exists())) {
      return null;
    }

    return storageFileResponse(file);
  } catch {
    return null;
  }
}

async function serveVaultObject(
  context: RequestContext,
  vaultId: string,
  objectKey: string,
) {
  if (context.config.storageType === "s3") {
    return redirect(storagePublicUrl(context.config, vaultId, objectKey), 302);
  }

  return serveLocalVaultObject(context, vaultId, objectKey);
}

async function handleConsoleStorageAsset(
  context: RequestContext,
  vaultId: string,
  objectKey: string,
) {
  const ownership = requireVaultOwnershipJson(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const assetResponse = await serveVaultObject(context, vaultId, objectKey);
  if (!assetResponse) {
    return finalizeConsoleJsonError(context, 404, "Not found");
  }

  return finalizeConsoleResponse(assetResponse, context);
}

async function handleVaultLogo(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnershipJson(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const { vault } = ownership;

  if (context.request.method === "GET") {
    if (!vault.logo_object_key) {
      return finalizeConsoleJsonError(context, 404, LOGO_NOT_FOUND_MESSAGE);
    }

    const assetResponse = await serveVaultObject(context, vaultId, vault.logo_object_key);
    return assetResponse
      ? finalizeConsoleResponse(assetResponse, context)
      : finalizeConsoleJsonError(context, 404, LOGO_NOT_FOUND_MESSAGE);
  }

  if (context.request.method === "DELETE") {
    const params = await readForm(context.request).catch(() => ({}));
    if (!validateCsrf(context, params)) {
      return finalizeConsoleJsonError(context, 403, CSRF_ERROR_MESSAGE);
    }

    if (vault.logo_object_key) {
      await removeStorageObject(context.config, vaultId, vault.logo_object_key);
      context.db.updateVaultLogo(vaultId, null);
    }

    return finalizeConsoleNoContent(context);
  }

  const formData = await context.request.formData();
  const csrfToken = formData.get("__anti-forgery-token");
  if (
    !validateCsrf(context, {
      "__anti-forgery-token": typeof csrfToken === "string" ? csrfToken : "",
    })
  ) {
    return finalizeConsoleJsonError(context, 403, CSRF_ERROR_MESSAGE);
  }

  const uploaded = formData.get("logo");
  if (!(uploaded instanceof File)) {
    return finalizeConsoleJsonError(context, 400, "No file uploaded");
  }

  const contentType = uploaded.type.toLowerCase();
  if (!ALLOWED_LOGO_CONTENT_TYPES.has(contentType)) {
    return finalizeConsoleJsonError(context, 400, "Invalid file type. Allowed: PNG, JPEG.");
  }

  if (uploaded.size <= 0) {
    return finalizeConsoleJsonError(context, 400, "File is empty. Please upload a valid image file.");
  }

  if (uploaded.size > MAX_LOGO_FILE_SIZE) {
    return finalizeConsoleJsonError(context, 400, "File too large. Maximum size is 2MB.");
  }

  const bytes = new Uint8Array(await uploaded.arrayBuffer());
  const dimensions = readImageDimensions(bytes, contentType);
  if (!dimensions) {
    return finalizeConsoleJsonError(context, 400, "Invalid image file");
  }

  if (Math.min(dimensions.width, dimensions.height) < MIN_LOGO_DIMENSION) {
    return finalizeConsoleJsonError(
      context,
      400,
      `Image too small. Minimum size is ${MIN_LOGO_DIMENSION}x${MIN_LOGO_DIMENSION}.`,
    );
  }

  const extension = contentType === "image/png" ? "png" : "jpg";
  const nextObjectKey = logoObjectKey(sha256Bytes(bytes, 16), extension);
  await putStorageObject(context.config, vaultId, nextObjectKey, bytes, contentType);
  context.db.updateVaultLogo(vaultId, nextObjectKey);

  if (vault.logo_object_key && vault.logo_object_key !== nextObjectKey) {
    await removeStorageObject(context.config, vaultId, vault.logo_object_key);
  }

  return finalizeConsoleResponse(
    jsonResponse({
      success: true,
      logoUrl: resolveConsoleAssetUrl(context, vaultId, nextObjectKey),
    }),
    context,
  );
}

async function handleVaultFavicon(context: RequestContext, vaultId: string) {
  const ownership = requireVaultOwnershipJson(context, vaultId);
  if ("response" in ownership) {
    return ownership.response;
  }

  const { vault } = ownership;
  if (!vault.logo_object_key) {
    return finalizeConsoleJsonError(context, 404, LOGO_NOT_FOUND_MESSAGE);
  }

  const assetResponse = await serveVaultObject(context, vaultId, vault.logo_object_key);
  return assetResponse
    ? finalizeConsoleResponse(assetResponse, context)
    : finalizeConsoleJsonError(context, 404, LOGO_NOT_FOUND_MESSAGE);
}

async function handleDomainCheck(context: RequestContext) {
  const domain = cleanDomain(context.url.searchParams.get("domain") ?? "");
  const exists = Boolean(domain && context.db.getVaultByDomain(domain));
  return new Response(exists ? "ok" : "not found", {
    status: exists ? 200 : 404,
    headers: {
      "Content-Type": TEXT_CONTENT_TYPE,
      "Cache-Control": "no-store",
      "X-Robots-Tag": NOINDEX_HEADER,
    },
  });
}

async function handleChangePassword(context: RequestContext) {
  if (!isAuthenticated(context.session)) {
    return sessionExpired(context);
  }

  const params = await readForm(context.request);
  if (!validateCsrf(context, params)) {
    return finalizeConsoleHtmlError(context, CSRF_ERROR_MESSAGE, 403);
  }

  const currentPassword = params["current-password"] ?? "";
  const newPassword = params["new-password"] ?? "";
  const confirmPassword = params["confirm-password"] ?? "";
  if (!currentPassword || !newPassword || !confirmPassword) {
    return finalizeConsoleHtmlError(context, MISSING_REQUIRED_FIELDS_MESSAGE);
  }

  if (newPassword !== confirmPassword) {
    return finalizeConsoleHtmlError(context, "New password confirmation does not match");
  }

  if (newPassword.length < 8) {
    return finalizeConsoleHtmlError(context, "New password must be at least 8 characters");
  }

  const user = context.db.getUserById(context.session.userId);
  if (!user) {
    return sessionExpired(context);
  }

  if (!(await Bun.password.verify(currentPassword, user.password_hash))) {
    return finalizeConsoleHtmlError(context, "Current password is incorrect");
  }

  const passwordHash = await Bun.password.hash(newPassword);
  context.db.updateUserPassword(user.id, passwordHash);
  return finalizeConsoleNoContent(context);
}

async function handleLogout(context: RequestContext) {
  context.session = null;
  context.sessionDirty = true;
  return finalizeConsoleResponse(noContentResponse(), context, true);
}

async function handleVaultInfo(context: RequestContext) {
  const result = getSyncVault(context);
  if ("response" in result) {
    return result.response;
  }
  const { vault } = result;

  const body: VaultInfoResponse = {
    success: true,
    vault: {
      id: vault.id,
      name: vault.name,
      domain: vault.domain ?? undefined,
      createdAt: vault.created_at,
    },
  };

  return jsonResponse(body);
}

async function handleSyncChanges(context: RequestContext) {
  const result = getSyncVault(context);
  if ("response" in result) {
    return result.response;
  }
  const { vault } = result;

  try {
    const body = await readJson<SyncChangesRequest>(context.request);
    const clientNotes = normalizeHashEntries(body.notes);
    const clientAssets = normalizeHashEntries(body.assets);

    const clientNoteMap = new Map(clientNotes.map((entry) => [entry.id, entry.hash]));
    const clientAssetMap = new Map(clientAssets.map((entry) => [entry.id, entry.hash]));
    const serverNotes = context.db.listNotesByVault(vault.id);
    const serverAssets = context.db.listAssetsByVault(vault.id);
    const serverNoteMap = new Map(serverNotes.map((entry) => [entry.client_id, entry.hash ?? ""]));
    const serverAssetMap = new Map(serverAssets.map((entry) => [entry.client_id, entry.md5]));

    const notesToDelete = Array.from(serverNoteMap.entries())
      .filter(([id]) => !clientNoteMap.has(id))
      .map(([id, hash]) => ({ id, hash }));
    const assetsToDelete = Array.from(serverAssetMap.entries())
      .filter(([id]) => !clientAssetMap.has(id))
      .map(([id, hash]) => ({ id, hash }));
    const notesToUpsert = Array.from(clientNoteMap.entries())
      .filter(([id, hash]) => hash !== serverNoteMap.get(id))
      .map(([id, hash]) => ({ id, hash }));
    const assetsToUpsert = Array.from(clientAssetMap.entries())
      .filter(([id, hash]) => hash !== serverAssetMap.get(id))
      .map(([id, hash]) => ({ id, hash }));

    for (const entry of notesToDelete) {
      await deleteNote(context, vault.id, entry.id);
    }

    for (const entry of assetsToDelete) {
      await deleteAsset(context, vault.id, entry.id);
    }

    const response = jsonResponse({
      need_upsert: {
        notes: notesToUpsert,
        assets: assetsToUpsert,
      },
      deleted_on_server: {
        notes: notesToDelete,
        assets: assetsToDelete,
      },
    } satisfies Omit<SyncChangesResponse, "success">);

    return recordVaultPublishResult(context, vault.id, response);
  } catch (error) {
    recordVaultPublishException(context, vault.id, error);
    return jsonError(400, "Invalid JSON body");
  }
}

async function handleSyncNote(context: RequestContext, noteId: string) {
  const result = getSyncVault(context);
  if ("response" in result) {
    return result.response;
  }
  const { vault } = result;

  try {
    const body = await readJson<SyncNoteRequest>(context.request);
    const parsed = parseSyncNotePayload(noteId, body);
    if ("error" in parsed) {
      return recordVaultPublishResult(
        context,
        vault.id,
        jsonError(400, parsed.error),
        parsed.error,
      );
    }

    const existing = context.db.getNoteByClientId(vault.id, parsed.noteId);
    if (existing && existing.hash === parsed.noteHash && existing.path === parsed.notePath) {
      return recordVaultPublishResult(
        context,
        vault.id,
        jsonResponse({ status: "skipped", noteId: parsed.noteId }),
      );
    }

    context.db.upsertNote({
      id: randomUUID(),
      tenantId: vault.tenant_id,
      vaultId: vault.id,
      path: parsed.notePath,
      clientId: parsed.noteId,
      content: parsed.content,
      metadata: parsed.metadata == null ? null : JSON.stringify(parsed.metadata),
      hash: parsed.noteHash,
    });

    await syncNoteAssetRefs(
      context,
      vault.id,
      parsed.noteId,
      parsed.assetEntries.map((entry) => entry.id),
    );
    syncNoteLinks(context, vault.id, parsed.noteId, parsed.linkedEntries);

    const needUploadAssets = parsed.assetEntries.filter((entry) => {
      const existingAsset = context.db.getAssetByClientId(vault.id, entry.id);
      return !existingAsset || existingAsset.md5 !== entry.hash;
    });
    const needUploadNotes = parsed.linkedEntries.filter((entry) => {
      const existingNote = context.db.getNoteByClientId(vault.id, entry.id);
      return !existingNote || existingNote.hash !== entry.hash;
    });

    return recordVaultPublishResult(
      context,
      vault.id,
      jsonResponse({
        status: "stored",
        noteId: parsed.noteId,
        need_upload_assets: needUploadAssets,
        need_upload_notes: needUploadNotes,
      }),
    );
  } catch (error) {
    recordVaultPublishException(context, vault.id, error);
    return jsonError(400, "Invalid JSON body");
  }
}

async function handleSyncAsset(context: RequestContext, assetId: string) {
  const result = getSyncVault(context);
  if ("response" in result) {
    return result.response;
  }
  const { vault } = result;

  try {
    const body = await readJson<SyncAssetRequest>(context.request);
    const parsed = parseSyncAssetPayload(assetId, body);
    if ("error" in parsed) {
      return recordVaultPublishResult(
        context,
        vault.id,
        jsonError(400, parsed.error),
        parsed.error,
      );
    }

    const existing = context.db.getAssetByClientId(vault.id, parsed.assetId);
    if (existing && existing.md5 === parsed.assetHash && existing.path === parsed.assetPath) {
      return recordVaultPublishResult(
        context,
        vault.id,
        jsonResponse({ status: "skipped", assetId: parsed.assetId }),
      );
    }

    const contentBytes = Buffer.from(parsed.content, "base64");
    const objectKey = assetObjectKey(parsed.assetId, extensionFromPath(parsed.assetPath));
    await putStorageObject(context.config, vault.id, objectKey, contentBytes, parsed.contentType);

    context.db.upsertAsset({
      id: randomUUID(),
      tenantId: vault.tenant_id,
      vaultId: vault.id,
      clientId: parsed.assetId,
      path: parsed.assetPath,
      objectKey,
      sizeBytes: typeof parsed.size === "number" ? parsed.size : contentBytes.byteLength,
      contentType: parsed.contentType,
      md5: parsed.assetHash,
    });

    return recordVaultPublishResult(
      context,
      vault.id,
      jsonResponse({ status: "stored", assetId: parsed.assetId }),
    );
  } catch (error) {
    recordVaultPublishException(context, vault.id, error);
    return jsonError(400, "Invalid JSON body");
  }
}

async function handlePublicFavicon(context: RequestContext, faviconFile: string) {
  const hostMatch = getVaultByHost(context);
  if ("response" in hostMatch) {
    return faviconResponse(faviconFile);
  }

  const { vault } = hostMatch;
  if (!vault.logo_object_key) {
    return faviconResponse(faviconFile);
  }

  return redirect(storagePublicUrl(context.config, vault.id, vault.logo_object_key));
}

async function handlePublicAppRequest(context: RequestContext) {
  const hostMatch = getVaultByHost(context);
  if ("response" in hostMatch) {
    return hostMatch.response;
  }

  const { vault } = hostMatch;
  const vaultView = mapPublicVault(context.config, vault);
  const pathClientIds = parsePathIds(context.url.pathname);
  const { notes, assets } = publicPageContext(context, vault.id);

  if (pathClientIds.length === 0) {
    if (vault.root_note_id) {
      const rootNote = context.db.getNoteByClientId(vault.id, vault.root_note_id);
      if (rootNote) {
        const prepared = buildPublicNoteView(context, vault.id, rootNote, notes, assets);
        const description = noteDescription(rootNote.content);

        if (context.isHtmx) {
          return htmlResponse(renderPublicNoteFragment(prepared));
        }

        return htmlResponse(
          renderDocument(
            <PublicNotePage vault={vaultView} notes={[prepared]} description={description} />,
          ),
        );
      }
    }

    return htmlResponse(
      renderDocument(
        <PublicHomePage
          vault={vaultView}
          notes={context.db.listNotesByVault(vault.id).map((note) => ({
            clientId: note.client_id,
            path: note.path,
            mtime: note.mtime,
          }))}
        />,
      ),
    );
  }

  const validNotes = pathClientIds
    .map((clientId) => context.db.getNoteByClientId(vault.id, clientId))
    .filter((note): note is NoteRecord => note != null);

  if (validNotes.length === 0) {
    return notFound();
  }

  if (context.isHtmx) {
    const lastNote = validNotes[validNotes.length - 1]!;
    const prepared = buildPublicNoteView(context, vault.id, lastNote, notes, assets);
    const pushUrl = buildPushUrl(
      context.request.headers.get("hx-current-url"),
      context.request.headers.get("x-from-note-id"),
      lastNote.client_id,
      vault.root_note_id,
    );

    return new Response(renderPublicNoteFragment(prepared), {
      headers: {
        "Content-Type": HTML_CONTENT_TYPE,
        "HX-Push-Url": pushUrl,
      },
    });
  }

  const preparedNotes = validNotes.map((note) =>
    buildPublicNoteView(context, vault.id, note, notes, assets),
  );

  return htmlResponse(
    renderDocument(
      <PublicNotePage
        vault={vaultView}
        notes={preparedNotes}
        description={noteDescription(validNotes[0]?.content ?? null)}
      />,
    ),
  );
}

export function createMdbrainWebApp(options: MdbrainWebOptions = {}): MdbrainWebApp {
  const cwd = options.cwd ?? resolve(import.meta.dir, "../../..");
  const config = loadRuntimeConfig({ cwd, env: options.env });
  const db = new ConsoleDatabase({
    databasePath: config.databasePath,
    migrationFile:
      options.migrationFile ??
      resolve(cwd, "server/resources/migrations/001-initial-schema.up.sql"),
  });
  const publicsDir = options.publicsDir ?? resolve(cwd, "server/resources/publics");
  const faviconFile = resolve(publicsDir, "shared/images/favicon.png");

  return {
    config,
    db,
    close() {
      db.close();
    },
    async fetch(request: Request) {
      const url = new URL(request.url);
      const pathname = url.pathname;
      const existingSession = parseSessionCookie(
        request.headers.get("cookie"),
        config.sessionSecret,
      );
      const context: RequestContext = {
        request,
        url,
        db,
        config,
        session: existingSession ?? createAnonymousSession(),
        sessionDirty: !existingSession,
        isHtmx: request.headers.get("hx-request") === "true",
        isStateChanging: ["POST", "PUT", "PATCH", "DELETE"].includes(request.method),
      };
      const consoleRequest = isConsoleRequest(context);
      const appRequest = isAppRequest(context);

      if (pathname === "/" && consoleRequest) {
        return redirect("/console");
      }

      if (pathname === "/robots.txt") {
        return new Response("User-agent: *\nDisallow: /\n", {
          headers: {
            "Content-Type": TEXT_CONTENT_TYPE,
          },
        });
      }

      if (pathname === "/favicon.ico" && appRequest) {
        return handlePublicFavicon(context, faviconFile);
      }

      if (pathname === "/favicon.ico") {
        const file = Bun.file(faviconFile);
        if (await file.exists()) {
          return new Response(file);
        }
      }

      if (pathname.startsWith("/publics/")) {
        return servePublicFile(pathname, publicsDir);
      }

      if (pathname === "/console/health" && consoleRequest) {
        const ok = getHealthTokenFromRequest(request, url) === config.healthToken;
        return new Response(ok ? "ok" : "unauthorized", {
          status: ok ? 200 : 401,
          headers: {
            "Content-Type": TEXT_CONTENT_TYPE,
            "X-Robots-Tag": NOINDEX_HEADER,
          },
        });
      }

      if (pathname === "/console/domain-check" && consoleRequest && config.caddyOnDemandTlsEnabled) {
        return handleDomainCheck(context);
      }

      if (pathname === "/obsidian/vault/info" && request.method === "GET" && consoleRequest) {
        return handleVaultInfo(context);
      }

      if (pathname === "/obsidian/sync/changes" && request.method === "POST" && consoleRequest) {
        return handleSyncChanges(context);
      }

      const syncNoteMatch = pathname.match(/^\/obsidian\/sync\/notes\/([^/]+)$/);
      if (syncNoteMatch && request.method === "POST" && consoleRequest) {
        return handleSyncNote(context, decodeURIComponent(syncNoteMatch[1]));
      }

      const syncAssetMatch = pathname.match(/^\/obsidian\/sync\/assets\/([^/]+)$/);
      if (syncAssetMatch && request.method === "POST" && consoleRequest) {
        return handleSyncAsset(context, decodeURIComponent(syncAssetMatch[1]));
      }

      const storageMatch = pathname.match(/^\/storage\/(.+)$/);
      if (storageMatch && request.method === "GET") {
        return serveStoredAsset(context, decodeURIComponent(storageMatch[1]));
      }

      const consoleStorageMatch = pathname.match(/^\/console\/storage\/([^/]+)\/(.+)$/);
      if (consoleStorageMatch && request.method === "GET" && consoleRequest) {
        return handleConsoleStorageAsset(
          context,
          decodeURIComponent(consoleStorageMatch[1]),
          decodeURIComponent(consoleStorageMatch[2]),
        );
      }

      if (pathname === "/console/init" && ["GET", "POST"].includes(request.method) && consoleRequest) {
        return handleConsoleInit(context);
      }

      if (!db.hasAnyUser() && pathname.startsWith("/console") && consoleRequest) {
        return finalizeConsoleResponse(redirect("/console/init"), context);
      }

      if (pathname === "/console/login" && ["GET", "POST"].includes(request.method) && consoleRequest) {
        return handleConsoleLogin(context);
      }

      if (pathname === "/console" && request.method === "GET" && consoleRequest) {
        return handleConsoleHome(context);
      }

      if (pathname === "/console/logout" && request.method === "POST" && consoleRequest) {
        return handleLogout(context);
      }

      if (pathname === "/console/user/password" && request.method === "PUT" && consoleRequest) {
        return handleChangePassword(context);
      }

      if (pathname === "/console/vaults" && request.method === "GET" && consoleRequest) {
        return handleVaultList(context);
      }

      if (pathname === "/console/vaults" && request.method === "POST" && consoleRequest) {
        return handleCreateVault(context);
      }

      const vaultRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)$/);
      if (vaultRouteMatch && request.method === "PUT" && consoleRequest) {
        return handleUpdateVault(context, vaultRouteMatch[1]);
      }

      if (vaultRouteMatch && request.method === "DELETE" && consoleRequest) {
        return handleDeleteVault(context, vaultRouteMatch[1]);
      }

      const renewRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/renew-sync-key$/);
      if (renewRouteMatch && request.method === "POST" && consoleRequest) {
        return handleRenewSyncKey(context, renewRouteMatch[1]);
      }

      const vaultNotesRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/notes$/);
      if (vaultNotesRouteMatch && request.method === "GET" && consoleRequest) {
        return handleVaultNotes(context, vaultNotesRouteMatch[1]);
      }

      const rootNoteRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/root-note$/);
      if (rootNoteRouteMatch && request.method === "PUT" && consoleRequest) {
        return handleUpdateRootNote(context, rootNoteRouteMatch[1]);
      }

      const rootNoteSelectorRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/root-note-selector$/);
      if (rootNoteSelectorRouteMatch && request.method === "GET" && consoleRequest) {
        return handleRootNoteSelector(context, rootNoteSelectorRouteMatch[1]);
      }

      const customHeadHtmlRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/custom-head-html$/);
      if (customHeadHtmlRouteMatch && request.method === "PUT" && consoleRequest) {
        return handleUpdateCustomHeadHtml(context, customHeadHtmlRouteMatch[1]);
      }

      const logoRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/logo$/);
      if (logoRouteMatch && ["GET", "POST", "DELETE"].includes(request.method) && consoleRequest) {
        return handleVaultLogo(context, logoRouteMatch[1]);
      }

      const faviconRouteMatch = pathname.match(/^\/console\/vaults\/([^/]+)\/favicon$/);
      if (faviconRouteMatch && request.method === "GET" && consoleRequest) {
        return handleVaultFavicon(context, faviconRouteMatch[1]);
      }

      if (appRequest && request.method === "GET") {
        return handlePublicAppRequest(context);
      }

      return notFound();
    },
  };
}
