import { useState, useEffect } from "react";
import { useTheme } from "@/context/theme-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { SettingsService } from "../../bindings/airspace-acars";

interface SettingsTabProps {
  localMode?: boolean;
  onLocalModeChange?: (localMode: boolean) => void;
}

export function SettingsTab({ localMode = false, onLocalModeChange }: SettingsTabProps) {
  const { theme, setTheme } = useTheme();
  const [simType, setSimType] = useState("auto");
  const [apiBaseURL, setApiBaseURL] = useState("");
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    async function load() {
      try {
        const settings = await SettingsService.GetSettings();
        setSimType(settings.simType);
        setApiBaseURL(settings.apiBaseURL);
        if (settings.theme === "light" || settings.theme === "dark") {
          setTheme(settings.theme);
        }
        setLoaded(true);
      } catch {
        setLoaded(true);
      }
    }
    load();
  }, [setTheme]);

  const handleThemeToggle = async (checked: boolean) => {
    const newTheme = checked ? "dark" : "light";
    setTheme(newTheme);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, theme: newTheme });
    } catch { /* ignore */ }
  };

  const handleSimTypeChange = async (value: string) => {
    setSimType(value);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, simType: value });
    } catch { /* ignore */ }
  };

  const handleApiBaseURLBlur = async () => {
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, apiBaseURL: apiBaseURL });
    } catch { /* ignore */ }
  };

  const handleLocalModeToggle = async (checked: boolean) => {
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, localMode: checked });
      onLocalModeChange?.(checked);
    } catch { /* ignore */ }
  };

  if (!loaded) return null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">Settings</h2>
        <p className="text-sm text-muted-foreground">
          Configure your application preferences
        </p>
      </div>

      <Separator />

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">Connection</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="shrink-0">
              <p className="text-sm font-medium">API Base URL</p>
              <p className="text-xs text-muted-foreground">
                Airspace platform endpoint
              </p>
            </div>
            <Input
              value={apiBaseURL}
              onChange={(e) => setApiBaseURL(e.target.value)}
              onBlur={handleApiBaseURLBlur}
              placeholder="https://airspace.ferrlab.com"
              className="max-w-[300px] font-mono text-xs"
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Local mode</p>
              <p className="text-xs text-muted-foreground">
                Only use authentication, disable flights, chat, and cabin audio
              </p>
            </div>
            <Switch checked={localMode} onCheckedChange={handleLocalModeToggle} />
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">Appearance</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Dark mode</p>
              <p className="text-xs text-muted-foreground">Toggle dark theme</p>
            </div>
            <Switch checked={theme === "dark"} onCheckedChange={handleThemeToggle} />
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">Simulator</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Simulator type</p>
              <p className="text-xs text-muted-foreground">
                Choose which simulator to connect to
              </p>
            </div>
            <Select value={simType} onValueChange={handleSimTypeChange}>
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto">Auto-detect</SelectItem>
                <SelectItem value="simconnect">SimConnect (MSFS)</SelectItem>
                <SelectItem value="xplane">X-Plane (UDP)</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">About</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-1.5 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Application</span>
              <span>Airspace ACARS</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Version</span>
              <span className="tabular-nums">1.0.0</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Runtime</span>
              <span>Wails v3</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
