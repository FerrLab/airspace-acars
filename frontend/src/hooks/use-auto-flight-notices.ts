import { useEffect, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { playAutoStartDing } from "@/lib/notification-sounds";

// AutoFlightSkip is the payload of "auto-flight-skipped" (domain.AutoFlightSkip):
// a flight the ACARS did not start or finish on its own because the aircraft
// is not at the airport the booking names.
export interface AutoFlightSkip {
  action: "start" | "finish";
  airport: string;
}

// useAutoFlightNotices tells the pilot what the ACARS did, or declined to do,
// on its own: a flight started or finished automatically, or not, because the
// aircraft is somewhere other than the booked airport. notify shows a message;
// volumeRef is read when the start chime plays, so a volume change does not
// re-subscribe.
export function useAutoFlightNotices(
  notify: (message: string) => void,
  volumeRef: MutableRefObject<number>
) {
  const { t } = useTranslation();

  useEffect(() => {
    const offStart = Events.On("auto-flight-start", (event: { data?: unknown }) => {
      const callsign = typeof event.data === "string" ? event.data : "";
      notify(t("acars.autoStarted", { callsign }));
      playAutoStartDing(volumeRef.current);
    });
    const offFinish = Events.On("auto-flight-finish", (event: { data?: unknown }) => {
      const airport = typeof event.data === "string" ? event.data : "";
      notify(t("acars.autoFinished", { airport }));
    });
    const offSkip = Events.On("auto-flight-skipped", (event: { data?: unknown }) => {
      const skip = event.data as Partial<AutoFlightSkip> | undefined;
      const airport = skip?.airport ?? "";
      if (skip?.action === "start") notify(t("acars.autoStartSkipped", { airport }));
      if (skip?.action === "finish") notify(t("acars.autoFinishSkipped", { airport }));
    });
    return () => {
      offStart();
      offFinish();
      offSkip();
    };
  }, [notify, t, volumeRef]);
}
