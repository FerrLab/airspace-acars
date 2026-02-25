import { useState } from "react";
import { useAuth } from "@/context/auth-context";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { ChatTab } from "@/components/chat-tab";
import { DebugTab } from "@/components/debug-tab";
import { SettingsTab } from "@/components/settings-tab";
import { Building2, LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";

export function AppShell() {
  const [activeTab, setActiveTab] = useState<Tab>("acars");
  const { tenant, logout } = useAuth();

  return (
    <div className="flex h-full">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />
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
            <Button variant="ghost" size="sm" onClick={logout}>
              <LogOut className="mr-1 h-3 w-3" />
              Logout
            </Button>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-6">
          {activeTab === "acars" && <AcarsTab />}
          {activeTab === "chat" && <ChatTab />}
          {activeTab === "debug" && <DebugTab />}
          {activeTab === "settings" && <SettingsTab />}
        </main>
      </div>
    </div>
  );
}
