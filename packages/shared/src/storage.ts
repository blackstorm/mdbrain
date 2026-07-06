import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve, sep } from "node:path";
import type { MdbrainRuntimeConfig } from "./config";

const DEFAULT_CONTENT_TYPE = "application/octet-stream";

export function vaultPrefix(vaultId: string) {
  return vaultId.replaceAll("-", "");
}

export function normalizePath(path: string) {
  return path
    .replaceAll("\\", "/")
    .split("/")
    .reduce<string[]>((parts, segment) => {
      if (!segment || segment === ".") {
        return parts;
      }

      if (segment === "..") {
        parts.pop();
        return parts;
      }

      parts.push(segment);
      return parts;
    }, [])
    .join("/");
}

function sanitizeExtension(extension: string | null | undefined) {
  if (!extension) {
    return null;
  }

  const normalized = extension
    .trim()
    .replace(/^\./, "")
    .toLowerCase()
    .replace(/[^a-z0-9+_-]/g, "")
    .slice(0, 32);

  return normalized || null;
}

export function extensionFromPath(path: string) {
  const filename = path.split("/").pop() ?? path;
  const dotIndex = filename.lastIndexOf(".");
  if (dotIndex <= 0 || dotIndex === filename.length - 1) {
    return null;
  }

  return sanitizeExtension(filename.slice(dotIndex + 1));
}

export function assetObjectKey(clientId: string, extension?: string | null) {
  const sanitized = sanitizeExtension(extension);
  return sanitized ? `assets/${clientId}.${sanitized}` : `assets/${clientId}`;
}

export function logoObjectKey(contentHash: string, extension: string) {
  const sanitized = sanitizeExtension(extension);
  if (!sanitized) {
    throw new Error("Invalid logo extension");
  }

  return `site/logo/${contentHash}.${sanitized}`;
}

export function faviconObjectKey(currentLogoObjectKey: string) {
  const extension = extensionFromPath(currentLogoObjectKey);
  if (!extension) {
    return null;
  }

  const dotIndex = currentLogoObjectKey.lastIndexOf(".");
  if (dotIndex <= 0) {
    return null;
  }

  return `${currentLogoObjectKey.slice(0, dotIndex)}.favicon.${extension}`;
}

export function publicAssetUrl(objectKey: string) {
  return `/storage/${normalizePath(objectKey)}`;
}

function s3ObjectKey(vaultId: string, objectKey: string) {
  return `${vaultPrefix(vaultId)}/${normalizePath(objectKey)}`;
}

type BunS3Client = InstanceType<typeof Bun.S3Client>;
type StorageFile = ReturnType<typeof Bun.file>;
type StorageReadResult = {
  body: Uint8Array;
  contentType: string;
};

let cachedS3Client: BunS3Client | null = null;
let cachedS3ClientKey = "";

function requireS3Config(config: MdbrainRuntimeConfig) {
  if (!config.s3Endpoint || !config.s3AccessKey || !config.s3SecretKey) {
    throw new Error("Missing S3 configuration");
  }
}

function s3ClientCacheKey(config: MdbrainRuntimeConfig) {
  return [
    config.s3Endpoint,
    config.s3AccessKey,
    config.s3SecretKey,
    config.s3Region,
    config.s3Bucket,
  ].join("|");
}

function getS3Client(config: MdbrainRuntimeConfig) {
  requireS3Config(config);

  const cacheKey = s3ClientCacheKey(config);
  if (cachedS3Client && cachedS3ClientKey === cacheKey) {
    return cachedS3Client;
  }

  cachedS3Client = new Bun.S3Client({
    accessKeyId: config.s3AccessKey!,
    secretAccessKey: config.s3SecretKey!,
    bucket: config.s3Bucket,
    endpoint: config.s3Endpoint!,
    region: config.s3Region,
  });
  cachedS3ClientKey = cacheKey;
  return cachedS3Client;
}

function getS3File(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
) {
  return getS3Client(config).file(s3ObjectKey(vaultId, objectKey));
}

async function readFileContents(file: StorageFile): Promise<StorageReadResult> {
  return {
    body: new Uint8Array(await file.arrayBuffer()),
    contentType: file.type || DEFAULT_CONTENT_TYPE,
  };
}

async function readLocalStorageObject(
  basePath: string,
  vaultId: string,
  objectKey: string,
) {
  try {
    const filePath = storageObjectPath(basePath, vaultId, objectKey);
    const file = Bun.file(filePath);
    if (!(await file.exists())) {
      return null;
    }

    return readFileContents(file);
  } catch {
    return null;
  }
}

async function readS3StorageObject(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
) {
  try {
    const file = getS3File(config, vaultId, objectKey);
    const stat = await file.stat();
    const body = new Uint8Array(await file.arrayBuffer());

    return {
      body,
      contentType: stat.type || DEFAULT_CONTENT_TYPE,
    } satisfies StorageReadResult;
  } catch {
    return null;
  }
}

async function deleteLocalStorageObject(
  basePath: string,
  vaultId: string,
  objectKey: string,
) {
  try {
    const targetPath = storageObjectPath(basePath, vaultId, objectKey);
    await Bun.file(targetPath).delete();
  } catch {
    // Ignore invalid or already-missing paths during cleanup.
  }
}

async function deleteS3VaultStorage(
  config: MdbrainRuntimeConfig,
  vaultId: string,
) {
  const client = getS3Client(config);
  let startAfter: string | undefined;

  do {
    const listed = await client.list({
      prefix: `${vaultPrefix(vaultId)}/`,
      startAfter,
    });
    const objectKeys = listed.contents?.map((item) => item.key).filter(Boolean) ?? [];

    if (objectKeys.length > 0) {
      await Promise.all(objectKeys.map((key) => client.delete(key)));
    }

    startAfter = listed.isTruncated ? objectKeys.at(-1) : undefined;
  } while (startAfter);
}

export function storagePublicUrl(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
) {
  if (config.storageType === "s3") {
    if (!config.s3PublicUrl) {
      throw new Error("Missing S3_PUBLIC_URL");
    }

    return `${config.s3PublicUrl.replace(/\/+$/, "")}/${config.s3Bucket}/${s3ObjectKey(vaultId, objectKey)}`;
  }

  return publicAssetUrl(objectKey);
}

export function storageRootPath(basePath: string, vaultId: string) {
  return resolve(basePath, vaultPrefix(vaultId));
}

export function storageObjectPath(basePath: string, vaultId: string, objectKey: string) {
  const rootPath = storageRootPath(basePath, vaultId);
  const normalizedObjectKey = normalizePath(objectKey);
  const absolutePath = resolve(rootPath, normalizedObjectKey);
  const normalizedRoot = `${rootPath}${sep}`;

  if (absolutePath !== rootPath && !absolutePath.startsWith(normalizedRoot)) {
    throw new Error("Invalid object key path");
  }

  return absolutePath;
}

export async function writeStorageObject(
  basePath: string,
  vaultId: string,
  objectKey: string,
  content: ArrayBuffer | Uint8Array | Buffer,
) {
  const targetPath = storageObjectPath(basePath, vaultId, objectKey);
  mkdirSync(dirname(targetPath), { recursive: true });
  await Bun.write(targetPath, content);
  return targetPath;
}

export async function putStorageObject(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
  content: ArrayBuffer | Uint8Array | Buffer,
  contentType: string,
) {
  if (config.storageType === "s3") {
    const file = getS3File(config, vaultId, objectKey);
    await file.write(content, {
      type: contentType,
    });
    return;
  }

  await writeStorageObject(config.localStoragePath, vaultId, objectKey, content);
}

export async function readStorageObject(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
) {
  if (config.storageType === "s3") {
    return readS3StorageObject(config, vaultId, objectKey);
  }

  return readLocalStorageObject(config.localStoragePath, vaultId, objectKey);
}

export async function deleteStorageObject(basePath: string, vaultId: string, objectKey: string) {
  await deleteLocalStorageObject(basePath, vaultId, objectKey);
}

export function deleteVaultStorage(basePath: string, vaultId: string) {
  try {
    rmSync(storageRootPath(basePath, vaultId), { recursive: true, force: true });
  } catch {
    // Ignore missing vault roots during cleanup.
  }
}

export async function removeStorageObject(
  config: MdbrainRuntimeConfig,
  vaultId: string,
  objectKey: string,
) {
  if (config.storageType === "s3") {
    await getS3Client(config).delete(s3ObjectKey(vaultId, objectKey));
    return;
  }

  await deleteStorageObject(config.localStoragePath, vaultId, objectKey);
}

export async function removeVaultStorage(
  config: MdbrainRuntimeConfig,
  vaultId: string,
) {
  if (config.storageType === "s3") {
    await deleteS3VaultStorage(config, vaultId);
    return;
  }

  deleteVaultStorage(config.localStoragePath, vaultId);
}
