import { useState, useEffect, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Card, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { Square, CheckCircle2, Zap, AlertTriangle } from "lucide-react";
import { AcarsDashboard } from "@/components/acars-dashboard";
import { RecordingControls } from "@/components/recording-controls";
import { useFlightData } from "@/hooks/use-flight-data";
import { useDevMode } from "@/hooks/use-dev-mode";
import { FlightDataService, FlightService, SettingsService } from "../../bindings/airspace-acars";
import { Events } from "@wailsio/runtime";
import { translateError } from "@/lib/translate-error";
import { playAutoStartDing } from "@/lib/notification-sounds";

interface AcarsTabProps {
  localMode?: boolean;
  volume: number;
  onVolumeChange: (v: number) => void;
}

export function AcarsTab({ localMode = false, volume, onVolumeChange }: AcarsTabProps) {
  const { t } = useTranslation();
  const { isRecording, flightData } = useFlightData();
  const [actionError, setActionError] = useState<string | null>(null);
  const [refreshingBooking, setRefreshingBooking] = useState(false);
  const [bookingError, setBookingError] = useState(false);
  const devMode = useDevMode();
  const [connectedAdapter, setConnectedAdapter] = useState("");
  const [connecting, setConnecting] = useState(false);
  const isConnected = connectedAdapter !== "";
  const [flightState, setFlightState] = useState<"idle" | "active" | "finishing">("idle");
  const [booking, setBooking] = useState<any>(null);
  const [startingFlight, setStartingFlight] = useState(false);
  const [endingFlight, setEndingFlight] = useState(false);
  const [onGround, setOnGround] = useState(false);
  const [groundSpeed, setGroundSpeed] = useState(0);
  const [activeFlightInfo, setActiveFlightInfo] = useState<{ callsign?: string; departure?: string; arrival?: string } | null>(null);
  const [autoNotification, setAutoNotification] = useState<string | null>(null);
  const [finishPending, setFinishPending] = useState<number | null>(null);
  const [resumeNotice, setResumeNotice] = useState<string | null>(null);
  // Seconds until the backend's minFlightDuration guard allows finishing;
  // seeded from GetActiveFlightInfo so it survives remounts mid-flight.
  const [finishCooldown, setFinishCooldown] = useState(0);
  const [confirmModal, setConfirmModal] = useState<"none" | "cancel" | "finish">("none");

  useEffect(() => {
    if (finishCooldown <= 0) return;
    const id = setTimeout(() => setFinishCooldown((s) => Math.max(0, s - 1)), 1000);
    return () => clearTimeout(id);
  }, [finishCooldown]);

  // Kept in a ref so the auto-start event handler always reads the current
  // volume without re-subscribing to the Wails event on every slider drag.
  const volumeRef = useRef(volume);
  useEffect(() => {
    volumeRef.current = volume;
  }, [volume]);

  useEffect(() => {
    FlightDataService.ConnectedAdapter().then(setConnectedAdapter).catch(() => {});
    if (!localMode) {
      FlightService.GetFlightState().then((s) => setFlightState(s as any)).catch(() => {});
    }

    const cancelConn = Events.On("connection-state", (event: any) => {
      setConnectedAdapter(event.data ?? "");
    });
    const cancelFlight = localMode ? () => {} : Events.On("flight-state", (event: any) => {
      setFlightState(event.data);
    });
    const cancelData = Events.On("flight-data", (event: any) => {
      const d = event.data;
      if (d?.sensors) setOnGround(d.sensors.onGround ?? false);
      if (d?.attitude) setGroundSpeed(d.attitude.gs ?? 0);
    });
    const cancelAutoStart = Events.On("auto-flight-start", (event: any) => {
      const callsign = event.data ?? "";
      setAutoNotification(t("acars.autoStarted", { callsign }));
      setTimeout(() => setAutoNotification(null), 5000);
      playAutoStartDing(volumeRef.current);
    });
    const cancelFinishProgress = Events.On("flight-finish-progress", (event: any) => {
      const pending = event?.data?.pending ?? 0;
      setFinishPending(pending);
    });
    const cancelFinishComplete = Events.On("flight-finish-complete", () => {
      setFinishPending(null);
      setAutoNotification(t("acars.finishComplete"));
      setTimeout(() => setAutoNotification(null), 5000);
    });
    const cancelFinishFailed = Events.On("flight-finish-failed", (event: any) => {
      const reason = event?.data?.reason ?? "";
      setFinishPending(null);
      setActionError(t("acars.finishFailedWithReason", { reason }));
    });
    const cancelResume = Events.On("flight-outbox-resuming", (event: any) => {
      const pending = event?.data?.pending ?? 0;
      setResumeNotice(t("acars.resumingOutbox", { count: pending }));
      setTimeout(() => setResumeNotice(null), 8000);
    });

    return () => {
      cancelConn();
      cancelFlight();
      cancelData();
      cancelAutoStart();
      cancelFinishProgress();
      cancelFinishComplete();
      cancelFinishFailed();
      cancelResume();
    };
  }, [localMode]);

  const fetchBooking = useCallback(async () => {
    try {
      const result = await FlightService.GetBooking();
      setBooking(result);
      setBookingError(false);
    } catch {
      setBooking(null);
      setBookingError(true);
    }
  }, []);

  // Poll booking every 10s when idle and connected (skip in local mode)
  useEffect(() => {
    if (localMode || !isConnected || flightState !== "idle") return;
    fetchBooking();
    const interval = setInterval(fetchBooking, 10_000);
    return () => clearInterval(interval);
  }, [localMode, isConnected, flightState, fetchBooking]);

  // Fetch active flight info when flight becomes active
  useEffect(() => {
    if (localMode || flightState === "idle") {
      setActiveFlightInfo(null);
      setFinishCooldown(0);
      return;
    }
    if (flightState === "finishing") return;
    FlightService.GetActiveFlightInfo()
      .then((info: any) => {
        setActiveFlightInfo(info);
        setFinishCooldown(Number(info?.finishCooldownSec ?? 0) || 0);
      })
      .catch(() => {});
  }, [localMode, flightState]);

  const handleConnect = async () => {
    setActionError(null);
    setConnecting(true);
    try {
      const adapter = await FlightDataService.ConnectSim("auto");
      setConnectedAdapter(adapter);
    } catch (e: any) {
      console.error("Failed to connect:", e);
      setActionError(translateError(t, "Failed to connect: " + e));
    } finally {
      setConnecting(false);
    }
  };

  const handleDisconnect = async () => {
    try {
      await FlightDataService.DisconnectSim();
      setConnectedAdapter("");
    } catch (e: any) {
      console.error("Failed to disconnect:", e);
    }
  };

  const handleStartFlight = async () => {
    if (!booking) return;
    setActionError(null);
    setStartingFlight(true);
    try {
      const callsign = booking.callsign ?? booking.flight_number ?? "";
      const departure = booking.departure_airport?.icao ?? "";
      const arrival = booking.alternate_airport?.icao ?? booking.arrival_airport?.icao ?? "";
      const bookingID = String(booking.id ?? "");
      await FlightService.StartFlight(callsign, departure, arrival, bookingID);
    } catch (e: any) {
      setActionError(translateError(t, "Failed to start flight: " + e));
    } finally {
      setStartingFlight(false);
    }
  };

  const handleStopFlight = async () => {
    setActionError(null);
    setEndingFlight(true);
    try {
      await FlightService.StopFlight();
    } catch (e: any) {
      setActionError(translateError(t, "Failed to stop flight: " + e));
    } finally {
      setEndingFlight(false);
    }
  };

  const handleFinishFlight = async () => {
    if (finishCooldown > 0) return;
    setActionError(null);
    setEndingFlight(true);
    try {
      await FlightService.FinishFlight();
    } catch (e: any) {
      setActionError(translateError(t, "Failed to finish flight: " + e));
    } finally {
      setEndingFlight(false);
    }
  };

  const [confirmCancelFlightSetting, setConfirmCancelFlightSetting] = useState(false);
  const [confirmFinishFlightSetting, setConfirmFinishFlightSetting] = useState(false);

  const reloadSettings = useCallback(async () => {
    try {
      const s = await SettingsService.GetSettings();
      setConfirmCancelFlightSetting(Boolean(s?.confirmCancelFlight));
      setConfirmFinishFlightSetting(Boolean(s?.confirmFinishFlight));
    } catch {}
  }, []);

  useEffect(() => {
    reloadSettings();
    window.addEventListener("focus", reloadSettings);
    return () => window.removeEventListener("focus", reloadSettings);
  }, [reloadSettings, flightState]);

  const requestFinishFlight = async () => {
    if (finishCooldown > 0) return;
    try {
      const s = await SettingsService.GetSettings();
      if (s?.confirmFinishFlight) {
        setConfirmFinishFlightSetting(true);
        setConfirmModal("finish");
        return;
      }
    } catch {}

    if (confirmFinishFlightSetting) {
      setConfirmModal("finish");
      return;
    }

    handleFinishFlight();
  };

  const requestStopFlight = async () => {
    try {
      const s = await SettingsService.GetSettings();
      if (s?.confirmCancelFlight) {
        setConfirmCancelFlightSetting(true);
        setConfirmModal("cancel");
        return;
      }
    } catch {}

    if (confirmCancelFlightSetting) {
      setConfirmModal("cancel");
      return;
    }

    handleStopFlight();
  };

  const handleConfirmCancel = async () => {
    setConfirmModal("none");
    await handleStopFlight();
  };

  const handleConfirmFinish = async () => {
    setConfirmModal("none");
    await handleFinishFlight();
  };

  const handleCancelFinish = async () => {
    try {
      await FlightService.CancelFinish();
    } catch (e: any) {
      console.error("Cancel finish failed:", e);
    }
  };

  const refreshBooking = async () => {
    setRefreshingBooking(true);
    try { await fetchBooking(); } finally { setRefreshingBooking(false); }
  };

  return (
    <div className="relative space-y-4">
      {autoNotification && <div role="status" className="flex items-center gap-2 rounded-xl border border-emerald-500/20 bg-emerald-500/5 px-4 py-3 text-sm"><Zap className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />{autoNotification}</div>}
      {resumeNotice && <div role="status" className="rounded-xl border border-amber-500/25 bg-amber-500/5 px-4 py-3 text-sm">{resumeNotice}</div>}
      <AcarsDashboard
        localMode={localMode} connectedAdapter={connectedAdapter} connecting={connecting}
        flightState={flightState} booking={booking} activeFlightInfo={activeFlightInfo}
        flightData={flightData} onGround={onGround} groundSpeed={groundSpeed}
        starting={startingFlight} ending={endingFlight} finishCooldown={finishCooldown}
        finishPending={finishPending} volume={volume} refreshing={refreshingBooking}
        bookingError={bookingError} error={actionError} onDismissError={() => setActionError(null)}
        onConnect={handleConnect} onDisconnect={handleDisconnect} onRefresh={refreshBooking}
        onStart={handleStartFlight} onStop={requestStopFlight} onFinish={requestFinishFlight}
        onCancelFinish={handleCancelFinish} onVolumeChange={onVolumeChange}
      />
      {devMode && <><Separator /><RecordingControls isRecording={isRecording} isConnected={isConnected} /></>}

      {/* Confirmation Modal Overlay */}
      {confirmModal !== "none" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4 animate-in fade-in duration-200">
          <Card className="w-[420px] max-w-full border-border bg-card shadow-2xl space-y-4">
            <CardHeader className="space-y-2 text-center pb-2">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 mb-1">
                {confirmModal === "cancel" ? (
                  <AlertTriangle className="h-6 w-6 text-destructive" />
                ) : (
                  <CheckCircle2 className="h-6 w-6 text-primary" />
                )}
              </div>
              <CardTitle className="text-xl tracking-tight font-semibold">
                {confirmModal === "cancel"
                  ? t("acars.cancelConfirmTitle")
                  : t("acars.finishConfirmTitle")}
              </CardTitle>
              <CardDescription className="text-sm text-muted-foreground leading-relaxed">
                {confirmModal === "cancel"
                  ? t("acars.cancelConfirmDesc")
                  : t("acars.finishConfirmDesc")}
              </CardDescription>
            </CardHeader>
            <CardFooter className="flex items-center justify-end gap-3 pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setConfirmModal("none")}
                disabled={endingFlight}
              >
                {t("acars.confirmReturn")}
              </Button>
              {confirmModal === "cancel" ? (
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={handleConfirmCancel}
                  disabled={endingFlight}
                  className="gap-2"
                >
                  <Square className="h-3.5 w-3.5" />
                  {t("acars.cancelConfirmAction")}
                </Button>
              ) : (
                <Button
                  variant="default"
                  size="sm"
                  onClick={handleConfirmFinish}
                  disabled={endingFlight}
                  className="gap-2"
                >
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  {t("acars.finishConfirmAction")}
                </Button>
              )}
            </CardFooter>
          </Card>
        </div>
      )}
    </div>
  );
}

