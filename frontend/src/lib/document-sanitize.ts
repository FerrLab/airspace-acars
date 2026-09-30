import DOMPurify from "dompurify";

/**
 * Sanitizes server-provided document HTML for the document viewer.
 *
 * This mirrors the allowlist that airspace applies on write
 * (app/Support/HtmlSanitizer.php, an HTMLPurifier configuration) so a
 * document renders the same in the ACARS client as on the web:
 *
 * - the same forbidden elements (anything executable, embedded, or a form);
 * - class attributes preserved, since the auto-generated documents (airport
 *   briefings, FDM and load profiles) are styled with Tailwind utilities;
 * - inline styles preserved but reduced to HTMLPurifier's safe property
 *   list, so no positioning, stacking or visibility tricks survive;
 * - ids and data attributes dropped;
 * - http, https, mailto and tel links, plus data: URIs on images only.
 *
 * On top of the server rules, positioning utilities are also stripped from
 * class attributes so a document can never overlay the rest of the app.
 */

/** Elements that are never allowed, matching airspace's FORBIDDEN_ELEMENTS. */
const FORBIDDEN_TAGS = [
  "script",
  "style",
  "iframe",
  "frame",
  "frameset",
  "object",
  "embed",
  "applet",
  "param",
  "form",
  "input",
  "button",
  "textarea",
  "select",
  "option",
  "base",
  "link",
  "meta",
  "noscript",
  "svg",
  "math",
];

/**
 * CSS properties HTMLPurifier accepts by default (CSS.AllowTricky off), so an
 * inline style that survived the server survives here with the same meaning.
 */
const ALLOWED_CSS_PROPERTIES = new Set([
  "background",
  "background-attachment",
  "background-color",
  "background-image",
  "background-position",
  "background-repeat",
  "background-size",
  "border",
  "border-bottom",
  "border-bottom-color",
  "border-bottom-style",
  "border-bottom-width",
  "border-collapse",
  "border-color",
  "border-left",
  "border-left-color",
  "border-left-style",
  "border-left-width",
  "border-radius",
  "border-right",
  "border-right-color",
  "border-right-style",
  "border-right-width",
  "border-spacing",
  "border-style",
  "border-top",
  "border-top-color",
  "border-top-style",
  "border-top-width",
  "border-width",
  "caption-side",
  "clear",
  "color",
  "float",
  "font",
  "font-family",
  "font-size",
  "font-style",
  "font-variant",
  "font-weight",
  "height",
  "letter-spacing",
  "line-height",
  "list-style",
  "list-style-position",
  "list-style-type",
  "margin",
  "margin-bottom",
  "margin-left",
  "margin-right",
  "margin-top",
  "max-height",
  "max-width",
  "min-height",
  "min-width",
  "opacity",
  "padding",
  "padding-bottom",
  "padding-left",
  "padding-right",
  "padding-top",
  "table-layout",
  "text-align",
  "text-decoration",
  "text-indent",
  "text-transform",
  "vertical-align",
  "white-space",
  "width",
  "word-spacing",
]);

/**
 * Tailwind utilities that take an element out of the document flow. Matched
 * against the utility with its variants (`md:`), importance (`!`) and
 * negative (`-`) markers removed.
 */
const OVERLAY_CLASS = /^(?:fixed|absolute|sticky|z-.+|inset-.+|top-.+|right-.+|bottom-.+|left-.+|start-.+|end-.+)$/;

/** A `url()` in a style value may only reference http(s) or an inline image. */
const CSS_URL = /url\(\s*(['"]?)([^'")]*)\1\s*\)/gi;
const SAFE_CSS_URL = /^(?:https?:|data:image\/)/i;

export function sanitizeDocumentHtml(dirty: string): string {
  if (!dirty) return "";
  return purifier().sanitize(dirty, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: FORBIDDEN_TAGS,
    FORBID_ATTR: ["id"],
    ALLOW_DATA_ATTR: false,
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
  });
}

/** Keep only the utilities that do not position the element. */
export function filterDocumentClasses(value: string): string {
  return value
    .split(/\s+/)
    .filter((cls) => {
      if (!cls) return false;
      const utility = cls.slice(cls.lastIndexOf(":") + 1).replace(/^!/, "").replace(/^-/, "");
      return !OVERLAY_CLASS.test(utility);
    })
    .join(" ");
}

/** Split a style attribute on the semicolons that sit outside `url(...)`. */
function splitDeclarations(value: string): string[] {
  const declarations: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < value.length; i++) {
    const ch = value[i];
    if (ch === "(") depth++;
    else if (ch === ")") depth = Math.max(0, depth - 1);
    else if (ch === ";" && depth === 0) {
      declarations.push(value.slice(start, i));
      start = i + 1;
    }
  }
  declarations.push(value.slice(start));
  return declarations;
}

/** Keep only allowlisted properties, dropping `!important` and unsafe URLs. */
export function filterDocumentStyle(value: string): string {
  const kept: string[] = [];
  for (const declaration of splitDeclarations(value)) {
    const colon = declaration.indexOf(":");
    if (colon === -1) continue;
    const property = declaration.slice(0, colon).trim().toLowerCase();
    const raw = declaration.slice(colon + 1).replace(/!\s*important/gi, "").trim();
    if (!raw || !ALLOWED_CSS_PROPERTIES.has(property)) continue;
    if (/expression\s*\(|@import|javascript:|vbscript:|behavior\s*:/i.test(raw)) continue;
    let unsafeUrl = false;
    raw.replace(CSS_URL, (_match, _quote, url: string) => {
      if (!SAFE_CSS_URL.test(url.trim())) unsafeUrl = true;
      return "";
    });
    if (unsafeUrl) continue;
    kept.push(`${property}: ${raw}`);
  }
  return kept.join("; ");
}

let instance: typeof DOMPurify | null = null;

function purifier(): typeof DOMPurify {
  if (instance) return instance;
  instance = DOMPurify;
  instance.addHook("uponSanitizeAttribute", (_node, data) => {
    if (data.attrName === "class") {
      data.attrValue = filterDocumentClasses(data.attrValue);
      data.keepAttr = data.attrValue !== "";
    } else if (data.attrName === "style") {
      data.attrValue = filterDocumentStyle(data.attrValue);
      data.keepAttr = data.attrValue !== "";
    }
  });
  return instance;
}
