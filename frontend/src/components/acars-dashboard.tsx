import { useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  AlertTriangle,
  ArrowDownUp,
  Check,
  CheckCircle2,
  Circle,
  Compass,
  Gauge,
  Headphones,
  Loader2,
  Plane,
  Plug,
  RefreshCw,
  Square,
  Unplug,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Slider } from "@/components/ui/slider";
import { cn } from "@/lib/utils";
import type { FlightData } from "@/hooks/use-flight-data";

interface Props {
  localMode: boolean;
  connectedAdapter: string;
  connecting: boolean;
  flightState: "idle" | "active" | "finishing";
  booking: any;
  activeFlightInfo: { callsign?: string; departure?: string; arrival?: string } | null;
  flightData: FlightData | null;
  onGround: boolean;
  groundSpeed: number;
  starting: boolean;
  ending: boolean;
  finishCooldown: number;
  finishPending: number | null;
  volume: number;
  refreshing: boolean;
  bookingError: boolean;
  error: string | null;
  onDismissError: () => void;
  onConnect: () => void;
  onDisconnect: () => void;
  onRefresh: () => void;
  onStart: () => void;
  onStop: () => void;
  onFinish: () => void;
  onCancelFinish: () => void;
  onVolumeChange: (volume: number) => void;
}

export function AcarsDashboard(p: Props) {
  const { t, i18n } = useTranslation();
  const lastVolume = useRef(25);
  if (p.volume > 0) lastVolume.current = p.volume;
  const connected = !!p.connectedAdapter;
  const active = !p.localMode && p.flightState !== "idle";
  const finishing = active && p.flightState === "finishing";
  const booking = !p.localMode ? p.booking : null;
  const route = active ? p.activeFlightInfo : {
    callsign: booking?.callsign ?? booking?.flight_number,
    departure: booking?.departure_airport?.icao,
    arrival: booking?.alternate_airport?.icao ?? booking?.arrival_airport?.icao,
  };
  const hasRoute = active || !!booking;
  const data = connected ? p.flightData : null;
  const status = p.localMode
    ? "acars.localMode"
    : finishing
      ? "acars.finishing"
      : active
        ? "acars.flightActive"
        : booking
          ? "acars.activeBooking"
          : "acars.dashboard.standby";
  const checklist = [
    { label: t("acars.dashboard.simulator"), done: connected },
    { label: t("acars.activeBooking"), done: !!booking },
    { label: t("acars.dashboard.onGround"), done: connected && p.onGround },
    { label: t("acars.dashboard.stationary"), done: connected && p.groundSpeed < 1 },
  ];
  const ready = checklist.every((item) => item.done);
  const number = (value?: number) =>
    value == null || !Number.isFinite(value)
      ? "—"
      : new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 0 }).format(value);
  const metrics = [
    { key: "altitude", icon: ArrowDownUp, value: data?.position.altitude, unit: "ft" },
    { key: "groundSpeed", icon: Gauge, value: data?.attitude.gs, unit: "kt" },
    { key: "verticalSpeed", icon: Activity, value: data?.attitude.vs, unit: "ft/min" },
    { key: "heading", icon: Compass, value: data?.attitude.headingMag, unit: "°" },
  ];

  return (
    <div className="acars-dashboard space-y-5">
      {/* Header section with page title and connection badge/button */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground">{t("acars.dashboard.title")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t("acars.dashboard.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Badge
            variant="outline"
            className={cn(
              "gap-2 px-3 py-1 text-xs font-medium border-border/80 bg-card/60 shadow-xs",
              connected ? "text-emerald-500 border-emerald-500/20 bg-emerald-500/5" : "text-muted-foreground"
            )}
          >
            <span
              className={cn(
                "h-2 w-2 rounded-full",
                connected ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]" : "bg-muted-foreground/40"
              )}
            />
            {connected ? t("acars.connectedTo", { adapter: p.connectedAdapter }) : t("acars.disconnected")}
          </Badge>
          <Button
            variant="outline"
            size="sm"
            disabled={p.connecting}
            onClick={connected ? p.onDisconnect : p.onConnect}
            className="shadow-xs"
          >
            {p.connecting ? <Loader2 className="animate-spin motion-reduce:animate-none" /> : connected ? <Unplug /> : <Plug />}
            {p.connecting ? t("acars.connecting") : connected ? t("acars.disconnect") : t("acars.connect")}
          </Button>
        </div>
      </div>

      {/* Action error banner */}
      {p.error && (
        <div
          role="alert"
          className="flex items-start justify-between gap-3 rounded-xl border border-destructive/25 bg-destructive/10 px-4 py-3 text-sm text-destructive shadow-xs"
        >
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" />
            <p className="font-medium text-destructive">{p.error}</p>
          </div>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t("acars.dashboard.dismiss")}
            onClick={p.onDismissError}
            className="text-destructive hover:bg-destructive/20"
          >
            <X />
          </Button>
        </div>
      )}

      {/* Main route and checklist section */}
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_260px]">
        {/* Route Card */}
        <Card className="gap-0 p-0 overflow-hidden border-border/80 bg-card shadow-xs" aria-label={t("acars.dashboard.route")}>
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/70 px-5 py-3 bg-muted/20">
            <Badge
              variant="outline"
              className={cn(
                "gap-2 px-2.5 py-0.5 text-xs font-medium border-border/60",
                active ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400" : "border-sky-500/30 bg-sky-500/10 text-sky-400"
              )}
            >
              {finishing ? (
                <Loader2 className="h-3 w-3 animate-spin motion-reduce:animate-none" />
              ) : (
                <span
                  className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    active ? "bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.5)]" : "bg-sky-500"
                  )}
                />
              )}
              {t(status)}
            </Badge>
            <span className="font-mono text-xs font-semibold tracking-wider px-2 py-0.5 rounded-md bg-muted/60 border border-border/60 text-foreground">
              {route?.callsign || "—"}
            </span>
          </div>

          <div className="px-5 py-6 sm:px-6">
            <div className="grid grid-cols-[1fr_minmax(40px,0.6fr)_1fr] items-center gap-3">
              <div>
                <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{t("acars.departure")}</p>
                <p className="mt-1 font-mono text-3xl font-bold tracking-tight text-foreground sm:text-4xl">{route?.departure || "— — —"}</p>
                {!active && booking?.departure_airport?.city && (
                  <p className="mt-1 truncate text-xs font-medium text-muted-foreground">{booking.departure_airport.city}</p>
                )}
              </div>
              <div className="flex items-center gap-2 text-sky-500" aria-hidden="true">
                <span className="h-px flex-1 border-t border-dashed border-current opacity-30" />
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-sky-500/10 border border-sky-500/20">
                  <Plane className="h-4 w-4 rotate-45 text-sky-400" />
                </div>
                <span className="h-px flex-1 border-t border-dashed border-current opacity-30" />
              </div>
              <div className="text-right">
                <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{t("acars.arrival")}</p>
                <p className="mt-1 font-mono text-3xl font-bold tracking-tight text-foreground sm:text-4xl">{route?.arrival || "— — —"}</p>
                {!active && booking?.alternate_airport ? (
                  <p className="mt-1 text-xs font-medium text-amber-500">{t("acars.dashboard.diverted", { airport: booking.arrival_airport?.icao ?? "—" })}</p>
                ) : !active && booking?.arrival_airport?.city ? (
                  <p className="mt-1 truncate text-xs font-medium text-muted-foreground">{booking.arrival_airport.city}</p>
                ) : null}
              </div>
            </div>
            <div className="mt-5 flex items-center gap-2 text-xs text-muted-foreground">
              <Plane className="h-3.5 w-3.5 shrink-0 text-muted-foreground/70" aria-hidden="true" />
              <span className="truncate font-medium">{data?.aircraftName || t("acars.dashboard.awaitingAircraft")}</span>
            </div>
          </div>

          <div className="border-t border-border/70 bg-muted/25 px-5 py-4">
            {p.localMode ? (
              <p className="text-sm text-muted-foreground">{t("acars.localModeDesc")}</p>
            ) : finishing ? (
              <div className="space-y-3">
                <p role="status" className="text-sm font-medium text-foreground">
                  {p.finishPending !== null && p.finishPending > 0 ? t("acars.finishingDrain", { count: p.finishPending }) : t("acars.finishing")}
                </p>
                <Button size="sm" variant="outline" onClick={p.onCancelFinish} className="shadow-xs">
                  {t("acars.cancelFinish")}
                </Button>
              </div>
            ) : active ? (
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  onClick={p.onFinish}
                  disabled={p.ending || p.finishCooldown > 0}
                  className="shadow-xs"
                >
                  <CheckCircle2 className="h-4 w-4" />
                  {p.ending ? t("acars.finishing") : p.finishCooldown > 0 ? t("acars.finishCooldown", { seconds: p.finishCooldown }) : t("acars.finishFlight")}
                </Button>
                <Button
                  variant="ghost"
                  onClick={p.onStop}
                  disabled={p.ending}
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                >
                  <Square className="h-4 w-4" />
                  {t("acars.cancel")}
                </Button>
              </div>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="max-w-sm text-xs leading-relaxed text-muted-foreground">
                  {p.bookingError ? t("acars.dashboard.bookingError") : !connected ? t("acars.dashboard.connectHint") : !hasRoute ? t("acars.noBooking") : ready ? t("acars.dashboard.readyHint") : t("acars.groundRequired")}
                </p>
                <Button
                  onClick={p.onStart}
                  disabled={!ready || p.starting}
                  className="shadow-xs"
                >
                  {p.starting ? <Loader2 className="animate-spin motion-reduce:animate-none" /> : <Plane className="h-4 w-4" />}
                  {p.starting ? t("acars.starting") : t("acars.startFlight")}
                </Button>
              </div>
            )}
          </div>
        </Card>

        {/* Preflight Checklist Card */}
        <Card className="gap-0 p-5 border-border/80 bg-card shadow-xs" aria-label={t(active ? "acars.dashboard.tracking" : "acars.dashboard.preflight")}>
          <div className="mb-4 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold tracking-tight text-foreground">{t(active ? "acars.dashboard.tracking" : "acars.dashboard.preflight")}</h3>
            {!active && !p.localMode && (
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t("acars.dashboard.refreshBooking")}
                onClick={p.onRefresh}
                disabled={!connected || p.refreshing}
                className="text-muted-foreground hover:text-foreground"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", p.refreshing && "animate-spin motion-reduce:animate-none")} />
              </Button>
            )}
          </div>
          {p.localMode ? (
            <p className="text-sm leading-relaxed text-muted-foreground">{t("acars.dashboard.localHint")}</p>
          ) : active ? (
            <div className="space-y-4">
              <div
                className={cn(
                  "inline-flex rounded-xl p-3 border",
                  connected ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                )}
              >
                <Activity className="h-5 w-5" />
              </div>
              <p className="text-sm font-semibold text-foreground">{t(finishing ? "acars.finishing" : connected ? "acars.dashboard.trackingActive" : "acars.dashboard.connectionLost")}</p>
              <p className="text-xs leading-relaxed text-muted-foreground">{t(finishing ? "acars.dashboard.finishHint" : connected ? "acars.dashboard.trackingHint" : "acars.dashboard.reconnectHint")}</p>
            </div>
          ) : (
            <ul className="space-y-3">
              {checklist.map(({ label, done }) => (
                <li key={label} className="flex items-center gap-2.5 text-xs">
                  <span
                    className={cn(
                      "flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors",
                      done ? "bg-emerald-500/15 text-emerald-500 border-emerald-500/30" : "bg-muted/40 text-muted-foreground/40 border-border/80"
                    )}
                  >
                    {done ? <Check className="h-3 w-3 stroke-[2.5]" /> : <Circle className="h-1.5 w-1.5 fill-current" />}
                  </span>
                  <span className={cn("font-medium transition-colors", done ? "text-foreground" : "text-muted-foreground")}>{label}</span>
                  <span className="sr-only">{t(done ? "acars.dashboard.complete" : "acars.dashboard.pending")}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Telemetry Grid */}
      <section aria-label={t("acars.dashboard.telemetry")}>
        <div className="mb-2.5 flex items-center justify-between gap-3">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("acars.dashboard.telemetry")}</h3>
          <span className="text-[11px] font-medium text-muted-foreground">{t(data ? "acars.dashboard.live" : "acars.dashboard.awaitingData")}</span>
        </div>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {metrics.map(({ key, icon: Icon, value, unit }) => (
            <Card key={key} className="gap-0 py-3.5 px-4 bg-card/60 border-border/80 shadow-xs">
              <dt className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <Icon className="h-3.5 w-3.5 text-muted-foreground/80" aria-hidden="true" />
                {t(`acars.dashboard.${key}`)}
              </dt>
              <dd className="mt-2 flex items-baseline gap-1.5">
                <span className="font-mono text-2xl font-bold tracking-tight text-foreground tabular-nums">
                  {number(value)}
                </span>
                <span className="text-xs font-medium text-muted-foreground">{unit}</span>
              </dd>
            </Card>
          ))}
        </dl>
      </section>

      {/* Cabin Audio Card */}
      <Card
        className="gap-0 flex-row flex-wrap items-center justify-between gap-4 py-3.5 px-5 border-border/80 bg-card shadow-xs"
        aria-label={t("acars.cabinAudio")}
      >
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted/60 border border-border/60 text-muted-foreground">
            <Headphones className="h-4 w-4" aria-hidden="true" />
          </div>
          <div>
            <h3 className="text-xs font-semibold text-foreground">{t("acars.cabinAudio")}</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">{t("acars.dashboard.volumeHint")}</p>
          </div>
        </div>
        <div className="flex min-w-48 flex-1 items-center gap-3 sm:max-w-xs">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t(p.volume === 0 ? "acars.dashboard.unmute" : "acars.dashboard.mute")}
            aria-pressed={p.volume === 0}
            onClick={() => p.onVolumeChange(p.volume === 0 ? lastVolume.current : 0)}
            className="text-muted-foreground hover:text-foreground"
          >
            {p.volume === 0 ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}
          </Button>
          <Slider
            min={0}
            max={100}
            step={1}
            value={[p.volume]}
            onValueChange={([value]) => p.onVolumeChange(value)}
            aria-label={t("acars.cabinAudio")}
            className="flex-1"
          />
          <Badge variant="outline" className="w-12 justify-center font-mono text-xs font-semibold tabular-nums border-border/60">
            {p.volume}%
          </Badge>
        </div>
      </Card>
    </div>
  );
}
