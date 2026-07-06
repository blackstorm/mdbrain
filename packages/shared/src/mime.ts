export const MIME_TYPES: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  svg: "image/svg+xml",
  bmp: "image/bmp",
  ico: "image/x-icon",
  pdf: "application/pdf",
  mp3: "audio/mpeg",
  ogg: "audio/ogg",
  wav: "audio/wav",
  mp4: "video/mp4",
  webm: "video/webm",
  md: "text/markdown",
  txt: "text/plain",
  json: "application/json",
  html: "text/html",
  css: "text/css",
  js: "application/javascript",
};

export function getContentType(extension: string) {
  const normalized = extension.replace(/^\./, "").toLowerCase();
  return MIME_TYPES[normalized] || "application/octet-stream";
}

export function getContentTypeFromPath(path: string) {
  const match = path.toLowerCase().match(/\.([a-z0-9+_-]+)$/);
  return getContentType(match?.[1] ?? "");
}
