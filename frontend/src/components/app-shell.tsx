import { useState } from "react";
import { Sidebar, type Tab } from "@/components/sidebar";
import { AcarsTab } from "@/components/acars-tab";
import { SettingsTab } from "@/components/settings-tab";

export function AppShell() {
  const [activeTab, setActiveTab] = useState<Tab>("acars");

  return (
    <div className="flex h-full">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />
      <main className="flex-1 overflow-y-auto p-6">
        {activeTab === "acars" && <AcarsTab />}
        {activeTab === "settings" && <SettingsTab />}
      </main>
    </div>
  );
}
