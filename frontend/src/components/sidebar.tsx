import { useAuth } from "@/context/auth-context";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Radio, Settings, LogOut } from "lucide-react";

export type Tab = "acars" | "settings";

interface SidebarProps {
  activeTab: Tab;
  onTabChange: (tab: Tab) => void;
}

export function Sidebar({ activeTab, onTabChange }: SidebarProps) {
  const { logout } = useAuth();

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: "acars", label: "ACARS", icon: <Radio className="h-4 w-4" /> },
    { id: "settings", label: "Settings", icon: <Settings className="h-4 w-4" /> },
  ];

  return (
    <div className="flex h-full w-[220px] flex-col border-r border-border/50 bg-card">
      <div className="flex h-14 items-center gap-2 px-5">
        <Radio className="h-5 w-5 text-primary" />
        <span className="text-sm font-semibold tracking-tight">Airspace ACARS</span>
      </div>

      <Separator />

      <nav className="flex-1 space-y-1 p-3">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => onTabChange(tab.id)}
            className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
              activeTab === tab.id
                ? "bg-accent text-accent-foreground font-medium"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            }`}
          >
            {tab.icon}
            {tab.label}
          </button>
        ))}
      </nav>

      <div className="p-3">
        <Separator className="mb-3" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start gap-3 text-muted-foreground hover:text-foreground"
              onClick={logout}
            >
              <LogOut className="h-4 w-4" />
              Log out
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Sign out of your account</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
