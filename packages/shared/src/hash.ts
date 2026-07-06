import { createHash } from "node:crypto";

export function md5Bytes(buffer: ArrayBuffer | ArrayBufferView) {
  const input = ArrayBuffer.isView(buffer)
    ? Buffer.from(buffer.buffer, buffer.byteOffset, buffer.byteLength)
    : Buffer.from(buffer);
  const hash = createHash("md5");
  hash.update(input);
  return hash.digest("hex");
}

export function md5String(value: string) {
  const encoder = new TextEncoder();
  return md5Bytes(encoder.encode(value));
}

export function sha256Bytes(buffer: ArrayBuffer | ArrayBufferView, byteLength?: number) {
  const input = ArrayBuffer.isView(buffer)
    ? Buffer.from(buffer.buffer, buffer.byteOffset, buffer.byteLength)
    : Buffer.from(buffer);
  const hash = createHash("sha256").update(input).digest("hex");

  if (!byteLength || byteLength <= 0) {
    return hash;
  }

  return hash.slice(0, byteLength * 2);
}
