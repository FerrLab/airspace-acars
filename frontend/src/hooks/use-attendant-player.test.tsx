import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Events } from "@wailsio/runtime";
import { AttendantService } from "../../bindings/airspace-acars";
import type { AttendantClip } from "@/lib/attendant";
import { CLIP_GAP_MS } from "@/lib/clip-player";
import { useAttendantPlayer } from "./use-attendant-player";

type Handler = (event: { data: unknown }) => void;
let handlers: Record<string, Handler>;

const clip = (id: string): AttendantClip => ({
  id, lineId: 1, phase: "cruise", text: "", language: "en", voice: "", music: false,
  reason: { code: "cruise" }, segments: [],
});

beforeEach(() => {
  handlers = {};
  vi.spyOn(Events, "On").mockImplementation(((name: string, cb: Handler) => {
    handlers[name] = cb;
    return () => { delete handlers[name]; };
  }) as unknown as typeof Events.On);
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

const advance = (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });

// The backend keeps the queue: the player takes clips until there are none
// and reports how each ended, which is what boarding repeats are timed from.
it("takes every waiting clip and reports how each ended", async () => {
  vi.useFakeTimers();
  const take = vi.spyOn(AttendantService, "TakeClip")
    .mockResolvedValueOnce(clip("clip-1"))
    .mockResolvedValueOnce(clip("clip-2"))
    .mockResolvedValue(null);
  const failed = vi.spyOn(AttendantService, "ClipFailed");

  renderHook(() => useAttendantPlayer(80, true));
  await advance(3 * CLIP_GAP_MS);

  expect(take).toHaveBeenCalledTimes(3);
  // A clip with nothing to play is reported failed rather than left hanging.
  expect(failed).toHaveBeenCalledWith("clip-1");
  expect(failed).toHaveBeenCalledWith("clip-2");
});

// One announcement does not run straight into the next: the player waits
// out the gap before it takes the next clip, so the backend shows that clip
// queued, where Skip and Stop all can still reach it.
it("leaves a gap after a clip before taking the next", async () => {
  vi.useFakeTimers();
  const take = vi.spyOn(AttendantService, "TakeClip")
    .mockResolvedValueOnce(clip("clip-1"))
    .mockResolvedValueOnce(clip("clip-2"))
    .mockResolvedValue(null);

  renderHook(() => useAttendantPlayer(80, true));
  await advance(0);
  expect(take).toHaveBeenCalledTimes(1);

  await advance(CLIP_GAP_MS - 1);
  expect(take, "the next clip was taken before the gap was over").toHaveBeenCalledTimes(1);

  await advance(1);
  expect(take, "the next clip was not taken once the gap was over").toHaveBeenCalledTimes(2);
});

it("pulls again when the backend says a clip is ready, and not while inactive", async () => {
  const take = vi.spyOn(AttendantService, "TakeClip").mockResolvedValue(null);
  const { rerender } = renderHook(({ active }) => useAttendantPlayer(80, active), { initialProps: { active: false } });
  expect(handlers["attendant-clip-ready"]).toBeUndefined();
  expect(take).not.toHaveBeenCalled();

  rerender({ active: true });
  await act(async () => { await Promise.resolve(); });
  await act(async () => { handlers["attendant-clip-ready"]({ data: true }); await Promise.resolve(); });
  expect(take).toHaveBeenCalledTimes(2);
});
