import { describe, expect, it, vi } from "vitest";
import { abortableSleep, playClip, type ClipDeps } from "./clip-player";

function deps(overrides: Partial<ClipDeps> = {}): ClipDeps & { played: string[] } {
  const played: string[] = [];
  return {
    played,
    getAudio: async (file) => ({ data: file, contentType: "audio/mpeg" }),
    playAudio: async (data) => { played.push(data); return true; },
    sleep: async (ms) => { played.push(`pause ${ms}`); },
    ...overrides,
  };
}

const ac = () => new AbortController();

describe("playClip", () => {
  it("plays the segments in order, pauses included", async () => {
    const d = deps();
    const clip = { segments: [
      { type: "play", localFile: "a.mp3", duration_ms: 0 },
      { type: "pause", duration_ms: 300 },
      { type: "play", localFile: "b.mp3", duration_ms: 0 },
    ] };
    expect(await playClip(clip, d, ac().signal)).toBe("played");
    expect(d.played).toEqual(["a.mp3", "pause 300", "b.mp3"]);
  });

  it("fails a clip whose file cannot be read or played", async () => {
    expect(await playClip({ segments: [{ type: "play", localFile: "a.mp3", duration_ms: 0 }] },
      deps({ getAudio: async () => null }), ac().signal)).toBe("failed");
    expect(await playClip({ segments: [{ type: "play", localFile: "a.mp3", duration_ms: 0 }] },
      deps({ playAudio: async () => false }), ac().signal)).toBe("failed");
    expect(await playClip({ segments: [] }, deps(), ac().signal)).toBe("failed");
    expect(await playClip({ segments: null }, deps(), ac().signal)).toBe("failed");
  });

  it("stops at once when interrupted, without reporting a failure", async () => {
    const c = ac();
    const d = deps({ playAudio: async () => { c.abort(); return false; } });
    const clip = { segments: [
      { type: "play", localFile: "a.mp3", duration_ms: 0 },
      { type: "play", localFile: "b.mp3", duration_ms: 0 },
    ] };
    expect(await playClip(clip, d, c.signal)).toBe("aborted");
  });
});

describe("abortableSleep", () => {
  it("ends early when aborted", async () => {
    vi.useFakeTimers();
    const c = ac();
    let done = false;
    const p = abortableSleep(60_000, c.signal).then(() => { done = true; });
    c.abort();
    await p;
    expect(done).toBe(true);
    vi.useRealTimers();
  });
});
