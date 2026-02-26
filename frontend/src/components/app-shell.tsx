import { useState, useEffect } from "react";
import { useAuth } from "@/context/auth-context";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { ChatTab } from "@/components/chat-tab";
import { DebugTab } from "@/components/debug-tab";
import { SettingsTab } from "@/components/settings-tab";
import { Building2, LogOut, MessageSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUnreadChat } from "@/hooks/use-unread-chat";
import { SettingsService } from "../../bindings/airspace-acars";

export function AppShell() {
  const [activeTab, setActiveTab] = useState<Tab>("acars");
  const [localMode, setLocalMode] = useState(false);
  const { tenant, logout } = useAuth();
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
        <header className="flex items-center justify-between border-b border-border/50 px-6 py-2">
          <div className="flex items-center gap-3">
            {tenant && (
              <>
                <Building2 className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm font-medium">{tenant.name}</span>
                <span className="text-xs text-muted-foreground">{tenant.domain}</span>
              </>
            )}
          </div>
          <div className="flex items-center gap-3">
            {hasUnread && activeTab !== "chat" && (
              <button
                onClick={() => setActiveTab("chat")}
                className="relative flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-yellow-400 hover:bg-accent transition-colors"
              >
                <MessageSquare className="h-3.5 w-3.5 animate-pulse" />
                <span>New message</span>
              </button>
            )}
            <Button variant="ghost" size="sm" onClick={logout}>
              <LogOut className="mr-1 h-3 w-3" />
              Logout
            </Button>
          </div>
        </header>
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
