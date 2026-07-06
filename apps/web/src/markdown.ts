import type { AssetLookupRecord, NoteLookupRecord } from "../../../packages/shared/src/console-db";
import { publicAssetUrl } from "../../../packages/shared/src/storage";
import { marked } from "marked";

marked.use({
  gfm: true,
  breaks: true,
});

interface ObsidianLink {
  embed: boolean;
  path: string;
  display: string;
  anchor?: string;
}

interface NoteIndex {
  byPath: Map<string, NoteLookupRecord>;
  byPathNoExt: Map<string, NoteLookupRecord>;
  byBasename: Map<string, NoteLookupRecord>;
  byBasenameNoExt: Map<string, NoteLookupRecord>;
}

interface AssetIndex {
  byPath: Map<string, AssetLookupRecord>;
  byBasename: Map<string, AssetLookupRecord>;
}

interface MarkdownRenderContext {
  noteIndex: NoteIndex;
  assetIndex: AssetIndex;
}

const CODE_BLOCK_PATTERN = /(?:```|~~~)[\s\S]*?(?:```|~~~)/g;
const INLINE_CODE_PATTERN = /`[^`\n]*`/g;

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(value: string) {
  return escapeHtml(value);
}

function stripYamlFrontMatter(content: string) {
  if (!content.startsWith("---")) {
    return content;
  }

  const lines = content.split(/\r?\n/);
  for (let index = 1; index < lines.length; index += 1) {
    if (lines[index]?.startsWith("---")) {
      return lines.slice(index + 1).join("\n");
    }
  }

  return content;
}

function maskCode(content: string) {
  const segments: string[] = [];
  const replaceSegment = (match: string) => {
    const index = segments.length;
    segments.push(match);
    return `MB_CODE_SEGMENT_${index}_MB`;
  };

  return {
    content: content
      .replace(CODE_BLOCK_PATTERN, replaceSegment)
      .replace(INLINE_CODE_PATTERN, replaceSegment),
    segments,
  };
}

function unmaskCode(content: string, segments: string[]) {
  return segments.reduce(
    (result, segment, index) => result.replaceAll(`MB_CODE_SEGMENT_${index}_MB`, segment),
    content,
  );
}

function normalizeAssetPath(path: string) {
  const trimmed = path
    .trim()
    .replace(/^['"]|['"]$/g, "")
    .replace(/^<|>$/g, "")
    .replace(/^\.\//, "")
    .replaceAll("\\", "/");

  const decoded = (() => {
    try {
      return decodeURIComponent(trimmed);
    } catch {
      return trimmed;
    }
  })();

  const cutIndex = [decoded.indexOf("?"), decoded.indexOf("#")]
    .filter((index) => index >= 0)
    .sort((left, right) => left - right)[0];

  return cutIndex == null ? decoded : decoded.slice(0, cutIndex);
}

function normalizeNotePath(path: string) {
  return normalizeAssetPath(path).replace(/^\/+/, "");
}

function removeMarkdownExtension(path: string) {
  return path.replace(/\.md$/i, "");
}

function basename(path: string) {
  return path.split("/").filter(Boolean).pop() ?? path;
}

function assetEmbed(path: string) {
  return /\.(png|jpg|jpeg|gif|webp|svg|bmp|ico|pdf|mp3|mp4|webm|ogg|wav)$/i.test(path);
}

function parseObsidianLink(source: string): ObsidianLink {
  const embed = source.startsWith("!");
  const inner = source.replace(/^!?\[\[/, "").replace(/\]\]$/, "");
  const [pathPart, displayPart] = inner.split("|", 2);
  const [path, anchor] = pathPart.split("#", 2);

  return {
    embed,
    path: path.trim(),
    display: (displayPart ?? pathPart).trim(),
    anchor: anchor?.trim() || undefined,
  };
}

function renderInternalLink(target: NoteLookupRecord, display: string, anchor?: string) {
  const href = anchor ? `/${target.client_id}#${encodeURIComponent(anchor)}` : `/${target.client_id}`;
  return `<a href="${href}" class="internal-link" data-note-id="${escapeAttribute(target.client_id)}">${escapeHtml(display)}</a>`;
}

function renderBrokenLink(path: string, display: string) {
  return `<span class="internal-link broken" title="Note not found: ${escapeAttribute(path)}">${escapeHtml(display)}</span>`;
}

function renderAssetLink(asset: AssetLookupRecord, display: string) {
  const assetUrl = asset.public_url ?? publicAssetUrl(asset.object_key);
  return `<a href="${assetUrl}" class="asset-link">${escapeHtml(display)}</a>`;
}

function renderAssetEmbed(asset: AssetLookupRecord, display: string) {
  const assetUrl = asset.public_url ?? publicAssetUrl(asset.object_key);
  if (/\.(png|jpg|jpeg|gif|webp|svg|bmp|ico)$/i.test(asset.path)) {
    return `<img src="${assetUrl}" alt="${escapeAttribute(display)}" class="asset-embed">`;
  }

  if (/\.pdf$/i.test(asset.path)) {
    return `<a href="${assetUrl}" class="asset-link pdf-link">${escapeHtml(display)}</a>`;
  }

  if (/\.(mp3|ogg|wav)$/i.test(asset.path)) {
    return `<audio src="${assetUrl}" controls class="asset-embed">${escapeHtml(display)}</audio>`;
  }

  if (/\.(mp4|webm)$/i.test(asset.path)) {
    return `<video src="${assetUrl}" controls class="asset-embed">${escapeHtml(display)}</video>`;
  }

  return renderAssetLink(asset, display);
}

function splitLinkDestination(raw: string) {
  const trimmed = raw.trim();
  if (trimmed.startsWith("<")) {
    const closeIndex = trimmed.indexOf(">");
    const destination = closeIndex > 0 ? trimmed.slice(1, closeIndex) : "";
    const rest = closeIndex > 0 ? trimmed.slice(closeIndex + 1) : "";
    return [destination, rest] as const;
  }

  const [destination, rest] = trimmed.split(/\s+/, 2);
  return [destination, rest ? ` ${rest}` : ""] as const;
}

function buildNoteIndex(notes: NoteLookupRecord[]): NoteIndex {
  const byPath = new Map<string, NoteLookupRecord>();
  const byPathNoExt = new Map<string, NoteLookupRecord>();
  const byBasename = new Map<string, NoteLookupRecord>();
  const byBasenameNoExt = new Map<string, NoteLookupRecord>();

  for (const note of notes) {
    const normalizedPath = normalizeNotePath(note.path);
    const normalizedWithoutExt = removeMarkdownExtension(normalizedPath);
    const normalizedBasename = basename(normalizedPath);
    const normalizedBasenameWithoutExt = removeMarkdownExtension(normalizedBasename);

    if (!byPath.has(normalizedPath)) byPath.set(normalizedPath, note);
    if (!byPathNoExt.has(normalizedWithoutExt)) byPathNoExt.set(normalizedWithoutExt, note);
    if (!byBasename.has(normalizedBasename)) byBasename.set(normalizedBasename, note);
    if (!byBasenameNoExt.has(normalizedBasenameWithoutExt)) {
      byBasenameNoExt.set(normalizedBasenameWithoutExt, note);
    }
  }

  return { byPath, byPathNoExt, byBasename, byBasenameNoExt };
}

function buildAssetIndex(assets: AssetLookupRecord[]): AssetIndex {
  const byPath = new Map<string, AssetLookupRecord>();
  const byBasename = new Map<string, AssetLookupRecord>();

  for (const asset of assets) {
    const normalizedPath = normalizeAssetPath(asset.path);
    const normalizedBasename = basename(normalizedPath);
    if (!byPath.has(normalizedPath)) byPath.set(normalizedPath, asset);
    if (!byBasename.has(normalizedBasename)) byBasename.set(normalizedBasename, asset);
  }

  return { byPath, byBasename };
}

function resolveNote(path: string, index: NoteIndex) {
  const normalizedPath = normalizeNotePath(path);
  const normalizedWithoutExt = removeMarkdownExtension(normalizedPath);
  const normalizedBasename = basename(normalizedPath);
  const normalizedBasenameWithoutExt = removeMarkdownExtension(normalizedBasename);

  return (
    index.byPath.get(normalizedPath) ??
    index.byPathNoExt.get(normalizedWithoutExt) ??
    index.byBasename.get(normalizedBasename) ??
    index.byBasenameNoExt.get(normalizedBasenameWithoutExt) ??
    null
  );
}

function resolveAsset(path: string, index: AssetIndex) {
  const normalizedPath = normalizeAssetPath(path);
  return index.byPath.get(normalizedPath) ?? index.byBasename.get(basename(normalizedPath)) ?? null;
}

function rewriteInlineImages(content: string, assetIndex: AssetIndex) {
  return content.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_match, alt: string, inner: string) => {
    const [destination, rest] = splitLinkDestination(inner);
    const asset = resolveAsset(destination, assetIndex);
    return asset
      ? `![${alt}](${asset.public_url ?? publicAssetUrl(asset.object_key)}${rest})`
      : `![${alt}](${inner})`;
  });
}

function extractReferenceDefinitions(content: string) {
  const definitions = new Map<string, string>();
  const matches = content.matchAll(/^\s*\[([^\]]+)\]:\s+(.+)$/gm);

  for (const match of matches) {
    const label = match[1]?.trim().toLowerCase().replace(/\s+/g, " ");
    const destination = match[2] ? splitLinkDestination(match[2])[0] : "";
    if (label) {
      definitions.set(label, normalizeAssetPath(destination));
    }
  }

  return definitions;
}

function rewriteReferenceImages(content: string, assetIndex: AssetIndex) {
  const definitions = extractReferenceDefinitions(content);
  return content.replace(/!\[([^\]]*)\]\[([^\]]*)\]/g, (original, alt: string, label: string) => {
    const resolvedLabel = (label || alt).trim().toLowerCase().replace(/\s+/g, " ");
    const destination = definitions.get(resolvedLabel);
    if (!destination) {
      return original;
    }

    const asset = resolveAsset(destination, assetIndex);
    return asset ? `![${alt}](${asset.public_url ?? publicAssetUrl(asset.object_key)})` : original;
  });
}

function rewriteReferenceDefinitions(content: string, assetIndex: AssetIndex) {
  return content.replace(/^\s*\[([^\]]+)\]:\s+(.+)$/gm, (line, label: string, rest: string) => {
    const [destination, tail] = splitLinkDestination(rest);
    const asset = resolveAsset(destination, assetIndex);
    return asset ? `[${label}]: ${asset.public_url ?? publicAssetUrl(asset.object_key)}${tail}` : line;
  });
}

function rewriteHtmlMediaTags(content: string, assetIndex: AssetIndex) {
  return content.replace(/<(img|audio|video|source)\b[^>]*>/g, (tag) => {
    const srcMatch = tag.match(/src\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))/i);
    const source = srcMatch?.[1] ?? srcMatch?.[2] ?? srcMatch?.[3];
    if (!source) {
      return tag;
    }

    const asset = resolveAsset(source, assetIndex);
    return asset
      ? tag.replace(
          /src\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))/i,
          `src="${asset.public_url ?? publicAssetUrl(asset.object_key)}"`,
        )
      : tag;
  });
}

function replaceObsidianLinks(content: string, context: MarkdownRenderContext) {
  const masked = maskCode(content);
  const replaced = masked.content.replace(/(!?)\[\[([^\]]+)\]\]/g, (fullMatch) => {
    const parsed = parseObsidianLink(fullMatch);
    const display = parsed.display || parsed.path;

    if (parsed.embed && assetEmbed(parsed.path)) {
      const asset = resolveAsset(parsed.path, context.assetIndex);
      return asset ? renderAssetEmbed(asset, display) : renderBrokenLink(parsed.path, display);
    }

    if (!parsed.embed && assetEmbed(parsed.path)) {
      const asset = resolveAsset(parsed.path, context.assetIndex);
      return asset ? renderAssetLink(asset, display) : renderBrokenLink(parsed.path, display);
    }

    const note = resolveNote(parsed.path, context.noteIndex);
    if (!note) {
      return renderBrokenLink(parsed.path, display);
    }

    return renderInternalLink(note, display, parsed.anchor);
  });

  return unmaskCode(replaced, masked.segments);
}

function extractMath(content: string) {
  const formulas: Array<{ type: "inline" | "block"; formula: string }> = [];
  const blockReplaced = content.replace(/\$\$([^$]+?)\$\$/g, (_match, formula: string) => {
    const index = formulas.length;
    formulas.push({ type: "block", formula: formula.trim() });
    return `MATHBLOCK${index}MATHBLOCK`;
  });

  const inlineReplaced = blockReplaced.replace(/(?<!\$)\$(?!\$)([^$\n]+?)\$(?!\$)/g, (_match, formula: string) => {
    const index = formulas.length;
    formulas.push({ type: "inline", formula: formula.trim() });
    return `MATHINLINE${index}MATHINLINE`;
  });

  return {
    content: inlineReplaced,
    formulas,
  };
}

function restoreMath(html: string, formulas: Array<{ type: "inline" | "block"; formula: string }>) {
  return formulas.reduce((result, formula, index) => {
    const escapedFormula = escapeAttribute(formula.formula);
    const replacement =
      formula.type === "inline"
        ? `<span class="math-inline" data-expr="${escapedFormula}">${escapeHtml(formula.formula)}</span>`
        : `<div class="math-block" data-expr="${escapedFormula}">${escapeHtml(formula.formula)}</div>`;

    return result
      .replaceAll(`MATHINLINE${index}MATHINLINE`, replacement)
      .replaceAll(`MATHBLOCK${index}MATHBLOCK`, replacement);
  }, html);
}

function imageLine(line: string) {
  const trimmed = line.trim();
  return /^!\[.*\]\(.*\)$/.test(trimmed) || /^!\[.*\]\[.*\]$/.test(trimmed) || /^!\[\[.*\]\]$/.test(trimmed);
}

export function createMarkdownContext(notes: NoteLookupRecord[], assets: AssetLookupRecord[]): MarkdownRenderContext {
  return {
    noteIndex: buildNoteIndex(notes),
    assetIndex: buildAssetIndex(assets),
  };
}

export function noteTitle(content: string | null, path: string) {
  if (content) {
    const stripped = stripYamlFrontMatter(content);
    for (const line of stripped.split(/\r?\n/)) {
      const match = line.match(/^#\s+(.+?)\s*$/);
      if (match?.[1]) {
        return match[1].trim();
      }
    }
  }

  return removeMarkdownExtension(path).replaceAll("/", " / ");
}

export function noteDescription(content: string | null, maxLength = 160) {
  if (!content) {
    return null;
  }

  const stripped = stripYamlFrontMatter(content);
  const lines = stripped.split(/\r?\n/);
  for (const line of lines) {
    if (!line.trim() || line.trim().startsWith("#") || imageLine(line)) {
      continue;
    }

    return line.trim().slice(0, maxLength);
  }

  return null;
}

export function renderMarkdown(content: string | null, context: MarkdownRenderContext) {
  if (!content) {
    return "";
  }

  const stripped = stripYamlFrontMatter(content);
  const { content: mathContent, formulas } = extractMath(stripped);
  const withAssets = rewriteHtmlMediaTags(
    rewriteReferenceDefinitions(
      rewriteReferenceImages(rewriteInlineImages(mathContent, context.assetIndex), context.assetIndex),
      context.assetIndex,
    ),
    context.assetIndex,
  );
  const withObsidianLinks = replaceObsidianLinks(withAssets, context);
  const html = marked.parse(withObsidianLinks) as string;
  return restoreMath(html, formulas);
}
