import { useTranslation } from "react-i18next";
import { useAuth } from "@/context/auth-context";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Settings, LogOut, Bug, MessageSquare, Radio, Building2 } from "lucide-react";
import { useDevMode } from "@/hooks/use-dev-mode";

export type Tab = "acars" | "chat" | "debug" | "settings";

interface SidebarProps {
  activeTab: Tab;
  onTabChange: (tab: Tab) => void;
  hasUnreadChat?: boolean;
  localMode?: boolean;
}

export function Sidebar({ activeTab, onTabChange, hasUnreadChat, localMode }: SidebarProps) {
  const { t } = useTranslation();
  const { logout, tenant } = useAuth();
  const devMode = useDevMode();

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: "acars", label: t("sidebar.acars"), icon: <Radio className="h-4 w-4" /> },
    { id: "chat", label: t("sidebar.chat"), icon: <MessageSquare className={`h-4 w-4 ${hasUnreadChat && activeTab !== "chat" ? "animate-pulse text-yellow-400" : ""}`} /> },
    ...(devMode ? [{ id: "debug" as Tab, label: t("sidebar.debug"), icon: <Bug className="h-4 w-4" /> }] : []),
    { id: "settings", label: t("sidebar.settings"), icon: <Settings className="h-4 w-4" /> },
  ];

  return (
    <div className="flex h-full w-[220px] flex-col border-r border-border/50 bg-card">
      {tenant?.banner_url ? (
        <img
          src={tenant.banner_url}
          alt={tenant.name}
          className="w-full max-h-20 object-cover"
        />
      ) : (
        <div className="flex h-14 items-center gap-2 px-5">
          {tenant?.logo_url ? (
            <img src={tenant.logo_url} alt="" className="h-6 w-6 rounded object-contain" />
          ) : (
            <Building2 className="h-6 w-6 text-muted-foreground" />
          )}
          <span className="text-sm font-semibold tracking-tight truncate">
            {tenant?.name ?? t("sidebar.defaultName")}
          </span>
          {localMode && (
            <Badge variant="outline" className="text-[9px] px-1.5 py-0 border-yellow-500/50 text-yellow-500">
              {t("sidebar.local")}
            </Badge>
          )}
        </div>
      )}

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
            {tab.id === "chat" && hasUnreadChat && activeTab !== "chat" && (
              <span className="ml-auto h-2 w-2 rounded-full bg-yellow-400 animate-pulse" />
            )}
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
              {t("sidebar.logout")}
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("sidebar.logoutTooltip")}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
