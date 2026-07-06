import { createHash, randomBytes } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

export interface MdbrainRuntimeConfig {
  host: string;
  appPort: number;
  consolePort: number;
  dataPath: string;
  databasePath: string;
  storageType: "local" | "s3";
  localStoragePath: string;
  environment: string;
  secureCookies: boolean;
  sessionSecret: string;
  healthToken: string;
  caddyOnDemandTlsEnabled: boolean;
  s3Endpoint: string | null;
  s3AccessKey: string | null;
  s3SecretKey: string | null;
  s3Region: string;
  s3Bucket: string;
  s3PublicUrl: string | null;
}

interface EnvSource {
  env?: Record<string, string | undefined>;
  cwd?: string;
}

function readDotEnv(cwd: string): Record<string, string> {
  const envFile = resolve(cwd, ".env");
  if (!existsSync(envFile)) {
    return {};
  }

  return readFileSync(envFile, "utf8")
    .split(/\r?\n/)
    .reduce<Record<string, string>>((acc, line) => {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#")) {
        return acc;
      }

      const separator = trimmed.indexOf("=");
      if (separator <= 0) {
        return acc;
      }

      const key = trimmed.slice(0, separator).trim();
      const rawValue = trimmed.slice(separator + 1).trim();
      const value = rawValue.replace(/^["']|["']$/g, "");
      acc[key] = value;
      return acc;
    }, {});
}

function getEnvValue(
  name: string,
  env: Record<string, string | undefined>,
  dotEnv: Record<string, string>,
  fallback?: string,
): string {
  return env[name] ?? dotEnv[name] ?? fallback ?? "";
}

function getIntEnvValue(
  name: string,
  env: Record<string, string | undefined>,
  dotEnv: Record<string, string>,
  fallback: number,
): number {
  const value = getEnvValue(name, env, dotEnv, String(fallback));
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function ensureParentDirectory(filePath: string) {
  mkdirSync(dirname(filePath), { recursive: true });
}

function generateRandomHex(byteLength: number) {
  return randomBytes(byteLength).toString("hex");
}

function parseEdnSecrets(contents: string): { sessionSecret?: string } {
  const match = contents.match(/:session-secret\s+"([^"]+)"/);
  return {
    sessionSecret: match?.[1],
  };
}

function writeSecretsFile(filePath: string, sessionSecret: string) {
  ensureParentDirectory(filePath);
  writeFileSync(filePath, `{:session-secret "${sessionSecret}"}\n`, "utf8");
}

function readOrCreateSessionSecret(
  dataPath: string,
  env: Record<string, string | undefined>,
  dotEnv: Record<string, string>,
): string {
  const envSecret = getEnvValue("SESSION_SECRET", env, dotEnv);
  if (envSecret) {
    return envSecret;
  }

  const secretsPath = resolve(dataPath, ".secrets.edn");
  if (existsSync(secretsPath)) {
    const parsed = parseEdnSecrets(readFileSync(secretsPath, "utf8"));
    if (parsed.sessionSecret) {
      return parsed.sessionSecret;
    }
  }

  const sessionSecret = generateRandomHex(16);
  writeSecretsFile(secretsPath, sessionSecret);
  return sessionSecret;
}

function readOrCreateHealthToken(dataPath: string): string {
  const tokenPath = resolve(dataPath, ".health-token");
  if (existsSync(tokenPath)) {
    const token = readFileSync(tokenPath, "utf8").trim();
    if (token) {
      return token;
    }
  }

  const token = generateRandomHex(32);
  ensureParentDirectory(tokenPath);
  writeFileSync(tokenPath, `${token}\n`, "utf8");
  return token;
}

export function md5Hex(input: string) {
  return createHash("md5").update(input).digest("hex");
}

export function loadRuntimeConfig(options: EnvSource = {}): MdbrainRuntimeConfig {
  const cwd = options.cwd ?? process.cwd();
  const env = options.env ?? process.env;
  const dotEnv = readDotEnv(cwd);
  const dataPath = resolve(cwd, getEnvValue("DATA_PATH", env, dotEnv, "data"));
  mkdirSync(dataPath, { recursive: true });

  const environment = getEnvValue("ENVIRONMENT", env, dotEnv, "development");
  const sessionSecret = readOrCreateSessionSecret(dataPath, env, dotEnv);
  const healthToken = readOrCreateHealthToken(dataPath);

  return {
    host: getEnvValue("HOST", env, dotEnv, "0.0.0.0"),
    appPort: getIntEnvValue("APP_PORT", env, dotEnv, 8080),
    consolePort: getIntEnvValue("CONSOLE_PORT", env, dotEnv, 9090),
    dataPath,
    databasePath: resolve(dataPath, "mdbrain.db"),
    storageType: getEnvValue("STORAGE_TYPE", env, dotEnv, "local") === "s3" ? "s3" : "local",
    localStoragePath: resolve(
      cwd,
      getEnvValue("LOCAL_STORAGE_PATH", env, dotEnv, resolve(dataPath, "storage")),
    ),
    environment,
    secureCookies: environment === "production",
    sessionSecret,
    healthToken,
    caddyOnDemandTlsEnabled: getEnvValue("CADDY_ON_DEMAND_TLS_ENABLED", env, dotEnv, "false") === "true",
    s3Endpoint: getEnvValue("S3_ENDPOINT", env, dotEnv) || null,
    s3AccessKey: getEnvValue("S3_ACCESS_KEY", env, dotEnv) || null,
    s3SecretKey: getEnvValue("S3_SECRET_KEY", env, dotEnv) || null,
    s3Region: getEnvValue("S3_REGION", env, dotEnv, "us-east-1"),
    s3Bucket: getEnvValue("S3_BUCKET", env, dotEnv, "mdbrain"),
    s3PublicUrl: getEnvValue("S3_PUBLIC_URL", env, dotEnv) || null,
  };
}
