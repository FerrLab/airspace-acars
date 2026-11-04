// Plays one flight attendant clip: its segments in order, files and pauses,
// until the end or until the signal aborts it. The audio itself is behind
// ClipDeps so the sequencing is testable without a browser audio stack.

import type { AttendantClip } from "@/lib/attendant";

export interface ClipDeps {
  /** The cached file's bytes, base64, as AudioService.GetAudioData gives them. */
  getAudio(localFile: string): Promise<{ data: string; contentType: string } | null>;
  /** Plays decoded audio; resolves true when it ended, false when it failed. */
  playAudio(base64: string, contentType: string, signal: AbortSignal): Promise<boolean>;
  sleep(ms: number, signal: AbortSignal): Promise<void>;
}

export type ClipOutcome = "played" | "failed" | "aborted";

/**
 * CLIP_GAP_MS is the silence between one clip and the next. A phase's lines,
 * and phases that fire together, used to play back to back, one voice running
 * straight into the next; a crew draws breath between announcements. Long
 * enough to hear as a pause, short enough that a line followed by its
 * translation still sounds like one announcement. It is not put between the
 * segments of a clip: those are pieces of one line, and the server adds the
 * pauses it wants there.
 */
export const CLIP_GAP_MS = 1500;

/**
 * How long the next clip still has to wait, given when the last one ended
 * (epoch ms; -Infinity when none has played yet).
 */
export function gapRemaining(lastEndedAt: number, now: number, gapMs = CLIP_GAP_MS): number {
  return Math.max(0, lastEndedAt + gapMs - now);
}

/** Plays every segment of clip in order. */
export async function playClip(clip: Pick<AttendantClip, "segments">, deps: ClipDeps, signal: AbortSignal): Promise<ClipOutcome> {
  const segments = clip.segments ?? [];
  if (segments.length === 0) return "failed";
  let played = 0;
  for (const seg of segments) {
    if (signal.aborted) return "aborted";
    if (seg.type === "pause") {
      await deps.sleep(seg.duration_ms, signal);
      continue;
    }
    if (seg.type !== "play" || !seg.localFile) continue;
    const audio = await deps.getAudio(seg.localFile).catch(() => null);
    if (signal.aborted) return "aborted";
    if (!audio) return "failed";
    const ok = await deps.playAudio(audio.data, audio.contentType, signal);
    if (signal.aborted) return "aborted";
    if (!ok) return "failed";
    played++;
  }
  return played > 0 ? "played" : "failed";
}

/** A sleep that ends early when the signal aborts. */
export function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) return resolve();
    const id = setTimeout(done, ms);
    signal.addEventListener("abort", done, { once: true });
    function done() {
      clearTimeout(id);
      signal.removeEventListener("abort", done);
      resolve();
    }
  });
}
