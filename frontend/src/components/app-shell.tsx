import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { ChatTab } from "@/components/chat-tab";
import { DebugTab } from "@/components/debug-tab";
import { SettingsTab } from "@/components/settings-tab";
import { useUnreadChat } from "@/hooks/use-unread-chat";
import { useSoundPlayer } from "@/hooks/use-sound-player";
import { Card, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { AlertTriangle, LogOut } from "lucide-react";
import { SettingsService, FlightService } from "../../bindings/airspace-acars";
import { Events } from "@wailsio/runtime";

export function AppShell() {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<Tab>("acars");
  const [localMode, setLocalMode] = useState(false);
  const { hasUnread } = useUnreadChat(activeTab === "chat", localMode);
  const [showCloseModal, setShowCloseModal] = useState(false);

  const [flightState, setFlightState] = useState<"idle" | "active">("idle");
  const [volume, setVolume] = useState(() => {
    const stored = localStorage.getItem("acars_volume");
    return stored ? parseInt(stored, 10) : 25;
  });

  // Sound player lives here so it persists across tab switches
  useSoundPlayer(volume, flightState === "active" && !localMode);

  useEffect(() => {
    SettingsService.GetSettings()
      .then((s) => setLocalMode(s.localMode ?? false))
      .catch(() => {});

    if (!localMode) {
      FlightService.GetFlightState().then((s) => setFlightState(s as any)).catch(() => {});
    }

    const cancelFlightState = Events.On("flight-state", (event: any) => {
      setFlightState(event.data);
    });

    const cancelCloseReq = Events.On("request-window-close", () => {
      setShowCloseModal(true);
    });

    return () => {
      cancelFlightState();
      cancelCloseReq();
    };
  }, [localMode]);

  const handleVolumeChange = (v: number) => {
    setVolume(v);
    localStorage.setItem("acars_volume", String(v));
  };

  const handleConfirmCloseApp = async () => {
    setShowCloseModal(false);
    try {
      await FlightService.StopFlight();
    } catch {}
    try {
      await FlightService.QuitApp();
    } catch {}
  };

  return (
    <div className="flex h-full relative">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} hasUnreadChat={hasUnread} localMode={localMode} />
      <div className="flex flex-1 flex-col">
        <main className="flex-1 overflow-y-auto p-6">
          {activeTab === "acars" && <AcarsTab localMode={localMode} volume={volume} onVolumeChange={handleVolumeChange} />}
          {activeTab === "chat" && <ChatTab localMode={localMode} />}
          {activeTab === "debug" && <DebugTab />}
          {activeTab === "settings" && <SettingsTab localMode={localMode} onLocalModeChange={setLocalMode} />}
        </main>
      </div>

      {showCloseModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4 animate-in fade-in duration-200">
          <Card className="w-[420px] max-w-full border-border bg-card shadow-2xl space-y-4">
            <CardHeader className="space-y-2 text-center pb-2">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 mb-1">
                <AlertTriangle className="h-6 w-6 text-destructive" />
              </div>
              <CardTitle className="text-xl tracking-tight font-semibold">
                {t("acars.closeConfirmTitle")}
              </CardTitle>
              <CardDescription className="text-sm text-muted-foreground leading-relaxed">
                {flightState === "active" ? t("acars.closeConfirmDesc") : t("acars.closeAppPrompt")}
              </CardDescription>
            </CardHeader>
            <CardFooter className="flex items-center justify-end gap-3 pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowCloseModal(false)}
              >
                {t("acars.confirmReturn")}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={handleConfirmCloseApp}
                className="gap-2"
              >
                <LogOut className="h-3.5 w-3.5" />
                {t("acars.closeConfirmAction")}
              </Button>
            </CardFooter>
          </Card>
        </div>
      )}
    </div>
  );
}
