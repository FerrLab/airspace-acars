import { describe, expect, it } from "vitest";
import { isUnsetTime, outcomeKey, secondsSince } from "./debug-snapshot";

describe("debug snapshot helpers", () => {
  it("treats Go's zero time as unset", () => {
    expect(isUnsetTime("0001-01-01T00:00:00Z")).toBe(true);
    expect(isUnsetTime("")).toBe(true);
    expect(isUnsetTime(undefined)).toBe(true);
    expect(isUnsetTime("2026-10-08T03:00:00Z")).toBe(false);
  });

  it("counts whole seconds since a time, and nothing for an unset one", () => {
    const now = Date.parse("2026-10-08T03:00:12Z");
    expect(secondsSince("2026-10-08T03:00:00Z", now)).toBe(12);
    expect(secondsSince("0001-01-01T00:00:00Z", now)).toBeNull();
    expect(secondsSince("not a time", now)).toBeNull();
    expect(secondsSince("2026-10-08T03:00:30Z", now)).toBe(0);
  });

  it("maps every outcome to a translation key", () => {
    expect(outcomeKey("sent")).toBe("debug.outcome.sent");
    expect(outcomeKey("outbox")).toBe("debug.outcome.outbox");
    expect(outcomeKey("dropped")).toBe("debug.outcome.dropped");
    expect(outcomeKey("preview")).toBe("debug.outcome.preview");
    expect(outcomeKey("")).toBe("debug.outcome.none");
  });
});
