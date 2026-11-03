import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Events } from "@wailsio/runtime";
import { AttendantService } from "../../bindings/airspace-acars";
import type { AttendantClip } from "@/lib/attendant";
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
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

// The backend keeps the queue: the player takes clips until there are none
// and reports how each ended, which is what boarding repeats are timed from.
it("takes every waiting clip and reports how each ended", async () => {
  const take = vi.spyOn(AttendantService, "TakeClip")
    .mockResolvedValueOnce(clip("clip-1"))
    .mockResolvedValueOnce(clip("clip-2"))
    .mockResolvedValue(null);
  const failed = vi.spyOn(AttendantService, "ClipFailed");

  renderHook(() => useAttendantPlayer(80, true));
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); });

  expect(take).toHaveBeenCalledTimes(3);
  // A clip with nothing to play is reported failed rather than left hanging.
  expect(failed).toHaveBeenCalledWith("clip-1");
  expect(failed).toHaveBeenCalledWith("clip-2");
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
