import * as Sentry from "@sentry/react";

/**
 * Sentry for the webview half of the ACARS.
 *
 * The Go side reports through observability.Init; this covers what it cannot
 * see — a render crash, a rejected Wails call, an error thrown in an event
 * handler. Both halves report into the same project and are told apart by the
 * `side` tag.
 *
 * The DSN is injected at build time as VITE_SENTRY_DSN. Without it every call
 * here is inert, so a local `npm run dev` and a fork's build report nothing.
 */

const dsn = import.meta.env.VITE_SENTRY_DSN ?? "";
const release = import.meta.env.VITE_APP_VERSION ?? "dev";

/** Strings that must never leave the machine, mirroring the Go scrubber. */
const JWT = /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+/g;
const BEARER = /(?:bearer|token|authorization)\s*[:=]?\s*\S+/gi;
const LONG_SECRET = /\b[A-Za-z0-9_-]{40,}\b/g;
const QUERY_SECRET = /([?&](?:token|code|secret|key|password|auth)=)[^&\s]+/gi;

export function redact<T>(value: T): T {
  if (typeof value === "string") {
    return value
      .replace(JWT, "[redacted]")
      .replace(QUERY_SECRET, "$1[redacted]")
      .replace(BEARER, "[redacted]")
      .replace(LONG_SECRET, "[redacted]") as unknown as T;
  }
  if (Array.isArray(value)) {
    return value.map(redact) as unknown as T;
  }
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) out[k] = redact(v);
    return out as unknown as T;
  }
  return value;
}

export function initSentry(): void {
  if (!dsn) return;

  Sentry.init({
    dsn,
    release: `airspace-acars@${release}`,
    environment: release === "dev" || release.includes("beta") ? "development" : "production",

    // The ACARS shows a pilot's own flight; there is no reason to ship the
    // contents of their screen or their keystrokes to us.
    sendDefaultPii: false,
    integrations: [],

    // Errors are the point. A webview has no meaningful traffic to trace.
    tracesSampleRate: 0,

    beforeSend(event) {
      if (event.message) event.message = redact(event.message);
      if (event.exception?.values) {
        for (const value of event.exception.values) {
          if (value.value) value.value = redact(value.value);
        }
      }
      if (event.extra) event.extra = redact(event.extra);
      if (event.request) delete event.request;
      return event;
    },

    beforeBreadcrumb(crumb) {
      if (crumb.message) crumb.message = redact(crumb.message);
      if (crumb.data) crumb.data = redact(crumb.data);
      return crumb;
    },
  });

  Sentry.setTag("side", "frontend");
}

/** Report an error the UI handled but should not have had to. */
export function captureError(error: unknown, context?: Record<string, unknown>): void {
  if (!dsn) return;
  Sentry.withScope((scope) => {
    if (context) scope.setContext("detail", redact(context));
    Sentry.captureException(error);
  });
}

export { Sentry };
