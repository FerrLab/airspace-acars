import { useEffect, useState } from "react";
import { DebugService } from "../../bindings/airspace-acars";
import type { DebugSnapshot } from "@/lib/debug-snapshot";

/** How often the debug tab asks for a snapshot: the data stream's own rate. */
export const DEBUG_POLL_MS = 1000;

/** The backend's debug snapshot, refreshed while the calling component is mounted. */
export function useDebugSnapshot(): DebugSnapshot | null {
  const [snapshot, setSnapshot] = useState<DebugSnapshot | null>(null);
  useEffect(() => {
    let cancelled = false;
    const poll = () => {
      Promise.resolve(DebugService.GetDebugSnapshot())
        .then((s) => {
          if (!cancelled) setSnapshot(s as unknown as DebugSnapshot);
        })
        // A missed poll is retried a second later; there is nothing to show.
        .catch(() => {});
    };
    poll();
    const id = setInterval(poll, DEBUG_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);
  return snapshot;
}
