import DOMPurify from "dompurify";

/**
 * Sanitizes server-provided HTML for safe rendering in the document viewer.
 * Uses DOMPurify with an allowlist approach, stripping scripts, dangerous tags,
 * event handlers, malicious URI schemes (e.g. javascript:, vbscript:), and
 * style attributes to prevent overlay/hijacking attacks.
 */
export function sanitizeDocumentHtml(dirty: string): string {
  if (!dirty) return "";
  return DOMPurify.sanitize(dirty, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: [
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
    ],
    FORBID_ATTR: ["style"],
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
  });
}
