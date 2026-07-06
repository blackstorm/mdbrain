import { md5Bytes, md5String } from "../../../packages/shared/src/hash";

export async function md5Hash(buffer: ArrayBuffer): Promise<string> {
  return md5Bytes(buffer);
}

export async function hashString(str: string): Promise<string> {
  return md5String(str);
}
