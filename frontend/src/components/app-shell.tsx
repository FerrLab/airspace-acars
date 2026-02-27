import { useState, useEffect } from "react";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { ChatTab } from "@/components/chat-tab";
import { DebugTab } from "@/components/debug-tab";
import { SettingsTab } from "@/components/settings-tab";
import { useUnreadChat } from "@/hooks/use-unread-chat";
import { useSoundPlayer } from "@/hooks/use-sound-player";
import { SettingsService, FlightService } from "../../bindings/airspace-acars";
import { Events } from "@wailsio/runtime";

export function AppShell() {
  const [activeTab, setActiveTab] = useState<Tab>("acars");
  const [localMode, setLocalMode] = useState(false);
  const { hasUnread } = useUnreadChat(activeTab === "chat", localMode);

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

    const cancel = Events.On("flight-state", (event: any) => {
      setFlightState(event.data);
    });
    return () => cancel();
  }, [localMode]);

  const handleVolumeChange = (v: number) => {
    setVolume(v);
    localStorage.setItem("acars_volume", String(v));
  };

  return (
    <div className="flex h-full">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} hasUnreadChat={hasUnread} localMode={localMode} />
      <div className="flex flex-1 flex-col">
        <main className="flex-1 overflow-y-auto p-6">
          {activeTab === "acars" && <AcarsTab localMode={localMode} volume={volume} onVolumeChange={handleVolumeChange} />}
          {activeTab === "chat" && <ChatTab localMode={localMode} />}
          {activeTab === "debug" && <DebugTab />}
          {activeTab === "settings" && <SettingsTab localMode={localMode} onLocalModeChange={setLocalMode} />}
        </main>
      </div>
    </div>
  );
}
