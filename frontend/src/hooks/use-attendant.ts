import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { AttendantService } from "../../bindings/airspace-acars";
import { EMPTY_ATTENDANT, type AttendantSnapshot } from "@/lib/attendant";

/** How often the snapshot is re-read for the readings no event announces (GSX, seatbelt). */
export const ATTENDANT_POLL_MS = 1000;

interface StateEvent {
  data: AttendantSnapshot;
}

/** The flight attendant's snapshot: polled, and refreshed on every "attendant-state" event. */
export function useAttendant(): AttendantSnapshot {
  const [snapshot, setSnapshot] = useState<AttendantSnapshot>(EMPTY_ATTENDANT);
  useEffect(() => {
    let cancelled = false;
    const poll = () => {
      Promise.resolve(AttendantService.GetSnapshot())
        .then((s) => {
          if (!cancelled && s) setSnapshot(s as unknown as AttendantSnapshot);
        })
        // A missed poll is retried a second later.
        .catch(() => {});
    };
    poll();
    const id = setInterval(poll, ATTENDANT_POLL_MS);
    const off = Events.On("attendant-state", (event: StateEvent) => {
      if (!cancelled && event?.data) setSnapshot(event.data);
    });
    return () => {
      cancelled = true;
      clearInterval(id);
      off();
    };
  }, []);
  return snapshot;
}
