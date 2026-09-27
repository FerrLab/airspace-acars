import { describe, it, expect } from "vitest";
import { sanitizeDocumentHtml } from "./document-sanitize";

describe("sanitizeDocumentHtml", () => {
  it("preserves safe HTML tags and formatting", () => {
    const safeHtml = "<h3>Departure Procedure</h3><p>Climb straight to <strong>4000 ft</strong>.</p>";
    expect(sanitizeDocumentHtml(safeHtml)).toBe(safeHtml);
  });

  it("removes script tags and inline execution", () => {
    const dirty = "<p>Briefing</p><script>alert('xss')</script><b>Caution</b>";
    const cleaned = sanitizeDocumentHtml(dirty);
    expect(cleaned).not.toContain("<script>");
    expect(cleaned).not.toContain("alert('xss')");
    expect(cleaned).toContain("<p>Briefing</p>");
    expect(cleaned).toContain("<b>Caution</b>");
  });

  it("strips on* event handlers and javascript: links", () => {
    const dirty = '<a href="javascript:steal()" onclick="hack()">Link</a><img src="valid.png" onerror="evil()" />';
    const cleaned = sanitizeDocumentHtml(dirty);
    expect(cleaned).not.toContain("onclick");
    expect(cleaned).not.toContain("onerror");
    expect(cleaned).not.toContain("javascript:");
    expect(cleaned).toContain("<img src=\"valid.png\">");
  });

  it("handles empty or null input gracefully", () => {
    expect(sanitizeDocumentHtml("")).toBe("");
  });
});
