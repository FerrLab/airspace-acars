import { describe, it, expect } from "vitest";
import { filterDocumentClasses, filterDocumentStyle, sanitizeDocumentHtml } from "./document-sanitize";

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

  it("removes every element airspace forbids on write", () => {
    const dirty =
      "<p>Keep</p><style>p{display:none}</style><iframe src='https://x'></iframe><form><input><button>Go</button>" +
      "<textarea></textarea><select><option>1</option></select></form><object></object><embed>" +
      "<noscript>n</noscript><svg><circle/></svg><math><mi>x</mi></math><base href='https://x'><link rel='stylesheet'><meta>";
    const cleaned = sanitizeDocumentHtml(dirty);
    for (const tag of [
      "style",
      "iframe",
      "form",
      "input",
      "button",
      "textarea",
      "select",
      "option",
      "object",
      "embed",
      "noscript",
      "svg",
      "math",
      "base",
      "link",
      "meta",
    ]) {
      expect(cleaned, tag).not.toContain(`<${tag}`);
    }
    expect(cleaned).toContain("<p>Keep</p>");
  });

  it("strips on* event handlers and javascript: links", () => {
    const dirty = '<a href="javascript:steal()" onclick="hack()">Link</a><img src="valid.png" onerror="evil()" />';
    const cleaned = sanitizeDocumentHtml(dirty);
    expect(cleaned).not.toContain("onclick");
    expect(cleaned).not.toContain("onerror");
    expect(cleaned).not.toContain("javascript:");
    expect(cleaned).toContain('<img src="valid.png">');
  });

  it("keeps http and https links and drops every other scheme", () => {
    expect(sanitizeDocumentHtml('<a href="https://airspace.test/sop">SOP</a>')).toContain('href="https://airspace.test/sop"');
    expect(sanitizeDocumentHtml('<a href="/sop/b737">SOP</a>')).toContain('href="/sop/b737"');
    // OpenDocumentURL only opens http and https, so these would be dead links.
    expect(sanitizeDocumentHtml('<a href="mailto:ops@airspace.test">Ops</a>')).toBe("<a>Ops</a>");
    expect(sanitizeDocumentHtml('<a href="tel:+551140000000">Tower</a>')).toBe("<a>Tower</a>");
    expect(sanitizeDocumentHtml('<a href="vbscript:msgbox(1)">Bad</a>')).not.toContain("vbscript:");
    expect(sanitizeDocumentHtml('<a href="data:text/html,hi">Bad</a>')).not.toContain("data:");
  });

  it("keeps inline images pasted by the editor as data URIs", () => {
    const cleaned = sanitizeDocumentHtml('<img src="data:image/png;base64,iVBORw0KGgo=" alt="chart">');
    expect(cleaned).toContain('src="data:image/png;base64,iVBORw0KGgo="');
  });

  it("preserves the utility classes the generated documents are styled with", () => {
    const briefing =
      '<div class="border border-zinc-200 dark:border-zinc-700 rounded-lg p-4 mb-6">' +
      '<span class="text-[28px] font-bold text-zinc-900 dark:text-white">SBGR</span></div>' +
      '<table class="w-full border-collapse text-sm"><thead><tr class="bg-zinc-50 dark:bg-zinc-800">' +
      '<th class="text-left px-4 py-2 !m-0">Designator</th></tr></thead></table>';
    expect(sanitizeDocumentHtml(briefing)).toBe(briefing);
  });

  it("preserves the safe inline styles the generated documents use", () => {
    const bar = '<div class="h-full bg-blue-500 rounded-sm" style="width: 42%;"></div>';
    expect(sanitizeDocumentHtml(bar)).toBe('<div class="h-full bg-blue-500 rounded-sm" style="width: 42%"></div>');
    const centred = '<p style="text-align: center">Centred</p>';
    expect(sanitizeDocumentHtml(centred)).toBe(centred);
  });

  it("drops ids and data attributes like the server does", () => {
    const cleaned = sanitizeDocumentHtml('<div id="target" data-overlay="true">Overlay</div>');
    expect(cleaned).toBe("<div>Overlay</div>");
  });

  it("neutralizes bypass vectors: tabs, control chars, xlink:href, and overlay styling", () => {
    // Tab within javascript scheme
    const tabBypass = '<a href="java&#9;script:alert(1)">Click</a>';
    expect(sanitizeDocumentHtml(tabBypass)).not.toContain("alert(1)");

    // Control char within javascript scheme
    const ctrlBypass = '<a href="&#1;javascript:alert(1)">Click</a>';
    expect(sanitizeDocumentHtml(ctrlBypass)).not.toContain("alert(1)");

    // SVG xlink:href
    const svgBypass = '<svg><a xlink:href="javascript:alert(1)">Click</a></svg>';
    expect(sanitizeDocumentHtml(svgBypass)).not.toContain("alert(1)");

    // Fixed overlay style: positioning is not on the safe property list, the
    // colour is, so the element stays but cannot leave the flow.
    const styleOverlay = '<div style="position:fixed;inset:0;z-index:9999;color:red">Overlay</div>';
    const cleanedStyle = sanitizeDocumentHtml(styleOverlay);
    expect(cleanedStyle).toBe('<div style="color: red">Overlay</div>');

    // Overlay via Tailwind classes: positioning utilities are removed, the
    // rest of the class list survives.
    const classOverlay = '<div class="fixed inset-0 z-50 md:absolute !-top-4 bg-black p-2">Overlay</div>';
    expect(sanitizeDocumentHtml(classOverlay)).toBe('<div class="bg-black p-2">Overlay</div>');
  });

  it("handles empty or null input gracefully", () => {
    expect(sanitizeDocumentHtml("")).toBe("");
  });
});

describe("filterDocumentClasses", () => {
  it("removes positioning utilities regardless of variant, importance or sign", () => {
    expect(
      filterDocumentClasses("flex fixed sticky absolute z-10 -z-10 !z-50 inset-x-0 top-0 lg:left-4 end-2 start-1 gap-2")
    ).toBe("flex gap-2");
  });

  it("keeps utilities whose names merely start with a positioning word", () => {
    expect(filterDocumentClasses("z-auto-not inset-shadow-sm")).toBe("");
    expect(filterDocumentClasses("tracking-wide rounded-lg overflow-hidden")).toBe(
      "tracking-wide rounded-lg overflow-hidden"
    );
  });
});

describe("filterDocumentStyle", () => {
  it("keeps only allowlisted properties and strips !important", () => {
    expect(filterDocumentStyle("color: red !important; position: absolute; width: 10px; display: none")).toBe(
      "color: red; width: 10px"
    );
  });

  it("rejects expressions and non-http url() values", () => {
    expect(filterDocumentStyle("width: expression(alert(1))")).toBe("");
    expect(filterDocumentStyle("background-image: url(javascript:alert(1))")).toBe("");
    expect(filterDocumentStyle("background-image: url('https://airspace.test/logo.png')")).toBe(
      "background-image: url('https://airspace.test/logo.png')"
    );
    expect(filterDocumentStyle("background-image: url(data:image/png;base64,AAAA)")).toBe(
      "background-image: url(data:image/png;base64,AAAA)"
    );
  });

  it("ignores malformed declarations", () => {
    expect(filterDocumentStyle("nonsense; color")).toBe("");
  });
});
