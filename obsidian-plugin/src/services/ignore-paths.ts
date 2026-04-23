import type { TFile } from "obsidian";

const normalizePathValue = (value: string): string => {
  const trimmed = value.trim();
  const unified = trimmed.replace(/\\/g, "/").replace(/^\.\//, "").replace(/^\/+/, "");
  const collapsed = unified.replace(/\/+/g, "/");
  return collapsed;
};

const normalizeRule = (rule: string): string => {
  const normalized = normalizePathValue(rule);
  if (!normalized) return "";
  if (normalized === "/") return "";
  return normalized;
};

const matchesRule = (normalizedPath: string, normalizedRule: string): boolean => {
  if (!normalizedPath || !normalizedRule) return false;

  if (normalizedRule.endsWith("/")) {
    const dir = normalizedRule.slice(0, -1);
    if (!dir) return false;
    return normalizedPath === dir || normalizedPath.startsWith(`${dir}/`);
  }

  return normalizedPath === normalizedRule || normalizedPath.startsWith(`${normalizedRule}/`);
};

export const normalizeVaultPath = (path: string): string => normalizePathValue(path);

export const normalizeIgnoreRules = (rules: string[]): string[] => {
  const seen = new Set<string>();
  const result: string[] = [];

  for (const rule of rules) {
    const normalized = normalizeRule(rule);
    if (!normalized || seen.has(normalized)) continue;
    seen.add(normalized);
    result.push(normalized);
  }

  return result;
};

export const isPathIgnored = (path: string, rules: string[]): boolean => {
  const normalizedPath = normalizeVaultPath(path);
  if (!normalizedPath) return false;

  const normalizedRules = normalizeIgnoreRules(rules);
  return normalizedRules.some((rule) => matchesRule(normalizedPath, rule));
};

export const filterIgnoredFiles = (files: TFile[], rules: string[]): TFile[] => {
  return files.filter((file) => !isPathIgnored(file.path, rules));
};
