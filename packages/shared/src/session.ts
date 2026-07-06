import { createHmac, randomBytes, timingSafeEqual } from "node:crypto";

export const SESSION_COOKIE_NAME = "mdbrain-session";

export interface ConsoleSession {
  userId?: string;
  tenantId?: string;
  csrfToken: string;
}

function toBase64Url(value: string) {
  return Buffer.from(value, "utf8").toString("base64url");
}

function fromBase64Url(value: string) {
  return Buffer.from(value, "base64url").toString("utf8");
}

function sign(payload: string, secret: string) {
  return createHmac("sha256", secret).update(payload).digest("base64url");
}

function serializeCookieValue(session: ConsoleSession, secret: string) {
  const payload = toBase64Url(JSON.stringify(session));
  return `${payload}.${sign(payload, secret)}`;
}

function escapeCookieValue(value: string) {
  return encodeURIComponent(value);
}

function unescapeCookieValue(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export function parseCookies(header: string | null) {
  if (!header) {
    return new Map<string, string>();
  }

  return header.split(/;\s*/).reduce((cookies, chunk) => {
    const separator = chunk.indexOf("=");
    if (separator <= 0) {
      return cookies;
    }

    const name = chunk.slice(0, separator).trim();
    const value = chunk.slice(separator + 1).trim();
    cookies.set(name, unescapeCookieValue(value));
    return cookies;
  }, new Map<string, string>());
}

export function parseSessionCookie(
  cookieHeader: string | null,
  secret: string,
): ConsoleSession | null {
  const rawValue = parseCookies(cookieHeader).get(SESSION_COOKIE_NAME);
  if (!rawValue) {
    return null;
  }

  const [payload, signature] = rawValue.split(".");
  if (!payload || !signature) {
    return null;
  }

  const expectedSignature = sign(payload, secret);
  const actualBuffer = Buffer.from(signature);
  const expectedBuffer = Buffer.from(expectedSignature);
  if (
    actualBuffer.length !== expectedBuffer.length ||
    !timingSafeEqual(actualBuffer, expectedBuffer)
  ) {
    return null;
  }

  try {
    const parsed = JSON.parse(fromBase64Url(payload));
    if (typeof parsed?.csrfToken !== "string" || !parsed.csrfToken) {
      return null;
    }
    return parsed as ConsoleSession;
  } catch {
    return null;
  }
}

export function createAnonymousSession(): ConsoleSession {
  return {
    csrfToken: randomBytes(24).toString("hex"),
  };
}

export function serializeSessionCookie(
  session: ConsoleSession,
  secret: string,
  secure: boolean,
) {
  const serialized = serializeCookieValue(session, secret);
  const parts = [
    `${SESSION_COOKIE_NAME}=${escapeCookieValue(serialized)}`,
    "Path=/",
    "HttpOnly",
    "SameSite=Lax",
    "Max-Age=604800",
  ];

  if (secure) {
    parts.push("Secure");
  }

  return parts.join("; ");
}

export function serializeExpiredSessionCookie(secure: boolean) {
  const parts = [
    `${SESSION_COOKIE_NAME}=`,
    "Path=/",
    "HttpOnly",
    "SameSite=Lax",
    "Max-Age=0",
    "Expires=Thu, 01 Jan 1970 00:00:00 GMT",
  ];

  if (secure) {
    parts.push("Secure");
  }

  return parts.join("; ");
}
