import { useState, useEffect } from "react";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { ChatTab } from "@/components/chat-tab";
import { DebugTab } from "@/components/debug-tab";
import { SettingsTab } from "@/components/settings-tab";
import { useUnreadChat } from "@/hooks/use-unread-chat";
import { SettingsService } from "../../bindings/airspace-acars";

export function AppShell() {
  const [activeTab, setActiveTab] = useState<Tab>("acars");
  const [localMode, setLocalMode] = useState(false);
  const { hasUnread } = useUnreadChat(activeTab === "chat", localMode);

  useEffect(() => {
    SettingsService.GetSettings()
      .then((s) => setLocalMode(s.localMode ?? false))
      .catch(() => {});
  }, []);

  return (
    <div className="flex h-full">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} hasUnreadChat={hasUnread} localMode={localMode} />
      <div className="flex flex-1 flex-col">
        <main className="flex-1 overflow-y-auto p-6">
          {activeTab === "acars" && <AcarsTab localMode={localMode} />}
          {activeTab === "chat" && <ChatTab localMode={localMode} />}
          {activeTab === "debug" && <DebugTab />}
          {activeTab === "settings" && <SettingsTab localMode={localMode} onLocalModeChange={setLocalMode} />}
        </main>
      </div>
    </div>
  );
}
