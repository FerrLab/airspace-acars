/**
 * Sanitizes server-provided HTML for safe rendering in the document viewer.
 * Removes scripts, dangerous tags, and event handlers while preserving safe formatting.
 */
export function sanitizeDocumentHtml(dirty: string): string {
  if (!dirty) return "";
  if (typeof document === "undefined") {
    // Fallback for non-browser/SSR environments
    return dirty.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, "");
  }

  const template = document.createElement("template");
  template.innerHTML = dirty;
  const fragment = template.content;

  // Remove dangerous elements
  const dangerousTags = [
    "script",
    "style",
    "iframe",
    "object",
    "embed",
    "form",
    "input",
    "button",
    "link",
    "meta",
    "base",
    "applet",
  ];
  fragment.querySelectorAll(dangerousTags.join(",")).forEach((node) => node.remove());

  // Strip dangerous attributes (e.g. onerror, onclick, javascript: hrefs)
  const allElements = fragment.querySelectorAll("*");
  allElements.forEach((el) => {
    Array.from(el.attributes).forEach((attr) => {
      const name = attr.name.toLowerCase();
      const val = attr.value.trim().toLowerCase();

      if (name.startsWith("on")) {
        el.removeAttribute(attr.name);
      } else if ((name === "href" || name === "src" || name === "action") && (val.startsWith("javascript:") || val.startsWith("vbscript:"))) {
        el.removeAttribute(attr.name);
      }
    });
  });

  return template.innerHTML;
}
