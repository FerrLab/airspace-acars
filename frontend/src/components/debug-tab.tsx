import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useFlightData } from "@/hooks/use-flight-data";
import { useDebugSnapshot } from "@/hooks/use-debug-snapshot";
import { FlightDataService } from "../../bindings/airspace-acars";
import { DebugStatusStrip } from "./debug/debug-status-strip";
import { DebugOverview } from "./debug/debug-overview";
import { DebugTelemetry } from "./debug/debug-telemetry";
import { DebugPayload } from "./debug/debug-payload";
import { DebugLogs } from "./debug/debug-logs";

export function DebugTab() {
  const { t } = useTranslation();
  const { flightData } = useFlightData();
  const snapshot = useDebugSnapshot();
  const [connected, setConnected] = useState(false);
  const [adapter, setAdapter] = useState("");

  useEffect(() => {
    let cancelled = false;
    Promise.resolve(FlightDataService.IsConnected())
      .then((c) => { if (!cancelled) setConnected(c); })
      .catch(() => {});
    Promise.resolve(FlightDataService.ConnectedAdapter())
      .then((name) => { if (!cancelled) setAdapter(name); })
      .catch(() => {});
    // "connection-state" carries the adapter name, or "" when the link is lost.
    const off = Events.On("connection-state", (event: { data: unknown }) => {
      const name = typeof event.data === "string" ? event.data : "";
      setConnected(name !== "");
      setAdapter(name);
    });
    return () => { cancelled = true; off(); };
  }, []);

  // Re-rendered by every snapshot poll, so ages stay current.
  const now = Date.now();

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">{t("debug.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("debug.subtitle")}</p>
      </div>
      <DebugStatusStrip connected={connected} adapter={adapter} flightData={flightData} snapshot={snapshot} now={now} />
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("debug.tab.overview")}</TabsTrigger>
          <TabsTrigger value="telemetry">{t("debug.tab.telemetry")}</TabsTrigger>
          <TabsTrigger value="payload">{t("debug.tab.payload")}</TabsTrigger>
          <TabsTrigger value="logs">{t("debug.tab.logs")}</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="pt-3">
          <DebugOverview snapshot={snapshot} flightData={flightData} now={now} />
        </TabsContent>
        <TabsContent value="telemetry" className="pt-3">
          {flightData
            ? <DebugTelemetry flightData={flightData} />
            : <p className="text-sm text-muted-foreground">{t("debug.waitingForData")}</p>}
        </TabsContent>
        <TabsContent value="payload" className="pt-3">
          <DebugPayload report={snapshot?.report} now={now} />
        </TabsContent>
        <TabsContent value="logs" className="pt-3">
          <DebugLogs />
        </TabsContent>
      </Tabs>
    </div>
  );
}
