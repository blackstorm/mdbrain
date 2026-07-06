const RAW_HTML = Symbol.for("mdbrain.raw-html");
const FRAGMENT = Symbol.for("mdbrain.fragment");

type Primitive = string | number | boolean | null | undefined;
type HtmlChild = Primitive | RawHtml | HtmlNode | HtmlChild[];
type HtmlTag = string | ((props: Record<string, unknown>) => HtmlChild) | typeof FRAGMENT;

interface RawHtml {
  [RAW_HTML]: string;
}

interface HtmlNode {
  type: string | typeof FRAGMENT;
  props: Record<string, unknown>;
}

const VOID_TAGS = new Set([
  "area",
  "base",
  "br",
  "col",
  "embed",
  "hr",
  "img",
  "input",
  "link",
  "meta",
  "param",
  "source",
  "track",
  "wbr",
]);

function flattenChildren(children: HtmlChild[]): HtmlChild[] {
  const flattened: HtmlChild[] = [];

  for (const child of children) {
    if (Array.isArray(child)) {
      flattened.push(...flattenChildren(child));
    } else {
      flattened.push(child);
    }
  }

  return flattened;
}

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function renderStyle(style: unknown) {
  if (style == null) {
    return "";
  }

  if (typeof style === "string") {
    return style;
  }

  if (typeof style !== "object") {
    return String(style);
  }

  return Object.entries(style)
    .map(([key, value]) => {
      const normalizedKey = key.replace(/[A-Z]/g, (match) => `-${match.toLowerCase()}`);
      return `${normalizedKey}:${String(value)}`;
    })
    .join(";");
}

function renderAttribute(name: string, value: unknown) {
  if (value == null || value === false) {
    return "";
  }

  const attributeName =
    name === "className" ? "class" : name === "htmlFor" ? "for" : name;

  if (value === true) {
    return ` ${attributeName}`;
  }

  const normalizedValue =
    attributeName === "style" ? renderStyle(value) : String(value);

  return ` ${attributeName}="${escapeHtml(normalizedValue)}"`;
}

function isRawHtml(value: unknown): value is RawHtml {
  return Boolean(value && typeof value === "object" && RAW_HTML in value);
}

function isHtmlNode(value: unknown): value is HtmlNode {
  return Boolean(
    value &&
      typeof value === "object" &&
      "type" in (value as Record<string, unknown>) &&
      "props" in (value as Record<string, unknown>),
  );
}

function renderChildren(children: unknown) {
  if (children == null) {
    return "";
  }

  return flattenChildren(Array.isArray(children) ? children : [children])
    .map((child) => renderToString(child))
    .join("");
}

export const Fragment = FRAGMENT;

export function raw(html: string): RawHtml {
  return {
    [RAW_HTML]: html,
  };
}

export function h(type: HtmlTag, props: Record<string, unknown> | null, ...children: HtmlChild[]) {
  const mergedProps: Record<string, unknown> = props ? { ...props } : {};
  if (children.length > 0) {
    mergedProps.children =
      mergedProps.children == null
        ? children
        : [mergedProps.children as HtmlChild, ...children];
  }

  if (type === FRAGMENT) {
    return (mergedProps.children ?? []) as HtmlChild;
  }

  if (typeof type === "function") {
    return type(mergedProps);
  }

  return {
    type,
    props: mergedProps,
  } satisfies HtmlNode;
}

export function renderToString(node: HtmlChild): string {
  if (node == null || node === false) {
    return "";
  }

  if (node === true) {
    return "true";
  }

  if (typeof node === "string" || typeof node === "number") {
    return escapeHtml(String(node));
  }

  if (Array.isArray(node)) {
    return renderChildren(node);
  }

  if (isRawHtml(node)) {
    return node[RAW_HTML];
  }

  if (!isHtmlNode(node)) {
    return escapeHtml(String(node));
  }

  if (node.type === FRAGMENT) {
    return renderChildren(node.props.children);
  }

  const { children, dangerouslySetInnerHTML, ...attributes } = node.props;
  const renderedAttributes = Object.entries(attributes)
    .map(([name, value]) => renderAttribute(name, value))
    .join("");

  if (dangerouslySetInnerHTML && typeof dangerouslySetInnerHTML === "object") {
    const html = (dangerouslySetInnerHTML as { __html?: string }).__html ?? "";
    return `<${node.type}${renderedAttributes}>${html}</${node.type}>`;
  }

  if (VOID_TAGS.has(node.type)) {
    return `<${node.type}${renderedAttributes}>`;
  }

  return `<${node.type}${renderedAttributes}>${renderChildren(children)}</${node.type}>`;
}

export function renderDocument(node: HtmlChild) {
  return `<!DOCTYPE html>${renderToString(node)}`;
}
