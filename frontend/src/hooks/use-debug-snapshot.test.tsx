import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DebugService } from "../../bindings/airspace-acars";
import { DEBUG_POLL_MS, useDebugSnapshot } from "./use-debug-snapshot";
import { EMPTY_SNAPSHOT } from "@/lib/debug-snapshot";

const snap = { ...EMPTY_SNAPSHOT, ground: { ...EMPTY_SNAPSHOT.ground, airport: "SBRF", stand: "7" } };

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(DebugService, "GetDebugSnapshot").mockResolvedValue(snap);
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

it("polls once a second and returns the latest snapshot", async () => {
  const { result } = renderHook(() => useDebugSnapshot());
  await act(async () => { await Promise.resolve(); });
  expect(result.current?.ground.stand).toBe("7");
  await act(async () => { vi.advanceTimersByTime(DEBUG_POLL_MS * 2); });
  expect(DebugService.GetDebugSnapshot).toHaveBeenCalledTimes(3);
});

it("stops polling when unmounted", async () => {
  const { unmount } = renderHook(() => useDebugSnapshot());
  await act(async () => { await Promise.resolve(); });
  unmount();
  await act(async () => { vi.advanceTimersByTime(DEBUG_POLL_MS * 5); });
  expect(DebugService.GetDebugSnapshot).toHaveBeenCalledTimes(1);
});
