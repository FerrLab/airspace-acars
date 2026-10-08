import type { Plugin } from "vite";

/**
 * Content Security Policy for the production bundle.
 *
 * Everything the app itself needs comes from the bundle: Vite emits no
 * inline scripts, and the Wails runtime is loaded from the same origin (its
 * calls go to /wails/runtime on the app origin, or through the webview's
 * message channel, which no policy governs). The app renders company
 * documents from the airline server (HTML through DOMPurify, PDFs in a
 * blob: iframe), so this policy is the layer behind the sanitizer: should a
 * document ever get a script past it, the script does not run.
 *
 * - script-src 'self': the bundle only; no inline scripts, no eval, no
 *   remote scripts.
 * - style-src 'unsafe-inline': React, Radix, Leaflet and the document
 *   viewer set style attributes. Inline styles cannot run code.
 * - img-src https: data: blob:: map tiles (ArcGIS), rank images and
 *   document images from the airline server, data: images the sanitizer
 *   allows, Vite-inlined assets.
 * - media-src blob:: notification sounds are generated in memory.
 * - frame-src blob:: the document PDF viewer.
 * - connect-src 'self' https: ws: wss:: the Wails runtime (same origin,
 *   plus its WebSocket transport in dev/server mode) and Sentry over https.
 * - object-src 'none', base-uri 'none', form-action 'none': nothing here
 *   embeds plugins, rewrites relative URLs or submits forms.
 *
 * The policy is delivered as a <meta> tag by a Vite plugin at build time,
 * because the asset server differs per platform (a custom scheme handler on
 * macOS and Linux, WebView2 on Windows) and a header set on the Go side
 * would need each of them to forward it; the tag works everywhere and is
 * not applied in the dev server, where Vite injects its own scripts.
 */
export const CONTENT_SECURITY_POLICY = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' https: data: blob:",
  "font-src 'self' data:",
  "media-src 'self' blob:",
  "frame-src 'self' blob:",
  "connect-src 'self' https: ws: wss:",
  "worker-src 'self' blob:",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join("; ");

/** The <meta> tag the policy is delivered in. */
export function cspMetaTag(policy: string = CONTENT_SECURITY_POLICY): string {
  return `<meta http-equiv="Content-Security-Policy" content="${policy}" />`;
}

/** Vite plugin that puts the policy into index.html for production builds. */
export function contentSecurityPolicyPlugin(): Plugin {
  return {
    name: "airspace-content-security-policy",
    apply: "build",
    transformIndexHtml: {
      order: "pre",
      handler(html) {
        return html.replace("<head>", `<head>\n    ${cspMetaTag()}`);
      },
    },
  };
}
