import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const cache = new Map<string, string>();
const lucideDir = resolve(import.meta.dir, "../../../server/resources/templates/lucide");
const FALLBACK_ICON_NAME = "circle-alert";
const ICON_ALIASES: Record<string, string> = {
  "alert-circle": "circle-alert",
  "alert-triangle": "triangle-alert",
  "check-circle": "circle-check",
  "more-vertical": "ellipsis-vertical",
};

function escapeAttribute(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

export function renderLucideIcon(name: string, className = "") {
  const resolvedName = ICON_ALIASES[name] ?? name;

  if (!cache.has(resolvedName)) {
    try {
      const svgPath = resolve(lucideDir, `${resolvedName}.svg`);
      cache.set(resolvedName, readFileSync(svgPath, "utf8"));
    } catch (error) {
      if (resolvedName !== FALLBACK_ICON_NAME) {
        return renderLucideIcon(FALLBACK_ICON_NAME, className);
      }

      throw error;
    }
  }

  const svg = cache.get(resolvedName) ?? "";
  return svg.replace(
    /<svg\b/,
    `<svg class="${escapeAttribute(className)}" data-lucide="${escapeAttribute(resolvedName)}" aria-hidden="true"`,
  );
}
