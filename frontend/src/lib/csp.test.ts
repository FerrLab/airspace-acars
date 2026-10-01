import { describe, expect, it } from "vitest";
import { CONTENT_SECURITY_POLICY, contentSecurityPolicyPlugin, cspMetaTag } from "../../csp";

function directive(name: string): string[] {
  const entry = CONTENT_SECURITY_POLICY.split("; ").find((d) => d.startsWith(`${name} `) || d === name);
  if (!entry) throw new Error(`directive ${name} missing`);
  return entry.split(" ").slice(1);
}

describe("content security policy", () => {
  it("never lets a script run that is not part of the bundle", () => {
    // The sanitizer keeps document scripts out; this is the layer behind it.
    expect(directive("script-src")).toEqual(["'self'"]);
    expect(CONTENT_SECURITY_POLICY).not.toContain("'unsafe-eval'");
    expect(CONTENT_SECURITY_POLICY).not.toMatch(/script-src[^;]*'unsafe-inline'/);
    expect(directive("object-src")).toEqual(["'none'"]);
    expect(directive("base-uri")).toEqual(["'none'"]);
    expect(directive("form-action")).toEqual(["'none'"]);
  });

  it("allows what the app itself loads", () => {
    // The Wails runtime and Sentry.
    expect(directive("connect-src")).toEqual(expect.arrayContaining(["'self'", "https:"]));
    // Map tiles, rank images, data: images in documents.
    expect(directive("img-src")).toEqual(expect.arrayContaining(["'self'", "https:", "data:", "blob:"]));
    // Notification sounds and the PDF viewer are blob URLs.
    expect(directive("media-src")).toContain("blob:");
    expect(directive("frame-src")).toContain("blob:");
    // React, Radix, Leaflet and document inline styles.
    expect(directive("style-src")).toContain("'unsafe-inline'");
  });

  it("is delivered as a meta tag at the top of head in production builds only", async () => {
    const plugin = contentSecurityPolicyPlugin();
    expect(plugin.apply).toBe("build");
    const hook = plugin.transformIndexHtml as { handler: (html: string) => string };
    const html = hook.handler("<!DOCTYPE html>\n<html>\n  <head>\n    <meta charset=\"UTF-8\" />\n  </head>\n</html>");
    expect(html).toContain(cspMetaTag());
    expect(html.indexOf(cspMetaTag())).toBeLessThan(html.indexOf('<meta charset="UTF-8" />'));
    expect(cspMetaTag()).not.toContain('content=""');
  });
});
