import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useTheme } from "@/context/theme-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Volume2, Headphones, Speaker, RotateCw } from "lucide-react";
import { SettingsService, UpdateService, DiscordService } from "../../bindings/airspace-acars";
import { useDevMode } from "@/hooks/use-dev-mode";
import {
  CHAT_SOUNDS,
  playNotificationPreview,
  getStoredVolumePercent,
  type ChatSoundType,
} from "@/lib/notification-sounds";
import {
  getAudioOutputDevices,
  getSelectedAudioDevice,
  setSelectedAudioDevice,
  playDeviceTestSound,
  subscribeAudioDeviceChange,
  type AudioOutputDeviceInfo,
} from "@/lib/audio-manager";
import { LANGUAGES } from "@/lib/i18n";

interface SettingsTabProps {
  localMode?: boolean;
  onLocalModeChange?: (localMode: boolean) => void;
}

export function SettingsTab({ localMode = false, onLocalModeChange }: SettingsTabProps) {
  const { t, i18n } = useTranslation();
  const { theme, setTheme } = useTheme();
  const devMode = useDevMode();
  const [simType, setSimType] = useState("auto");
  const [chatSound, setChatSound] = useState<ChatSoundType>("default");
  const [discordPresence, setDiscordPresence] = useState(true);
  const [autoStartFlight, setAutoStartFlight] = useState(true);
  const [confirmCloseApp, setConfirmCloseApp] = useState(false);
  const [confirmCancelFlight, setConfirmCancelFlight] = useState(false);
  const [confirmFinishFlight, setConfirmFinishFlight] = useState(false);
  const [apiBaseURL, setApiBaseURL] = useState("");
  const [language, setLanguage] = useState(i18n.language);
  const [loaded, setLoaded] = useState(false);

  // Audio track and output device states
  const [audioDevice, setAudioDevice] = useState(() => getSelectedAudioDevice());
  const [audioDevices, setAudioDevices] = useState<AudioOutputDeviceInfo[]>([]);
  const [refreshingAudio, setRefreshingAudio] = useState(false);
  const [testingAudio, setTestingAudio] = useState(false);
  const [volume, setVolume] = useState(() => getStoredVolumePercent());

  const refreshAudioDevices = useCallback(async () => {
    setRefreshingAudio(true);
    try {
      const defaultLabel = t("settings.audioDeviceDefault", "Padrão do Sistema");
      const unnamedPrefix = t("settings.audioDeviceUnnamed", "Dispositivo de Áudio");
      // An explicit refresh retries the media permission unlock even if it was
      // denied earlier (e.g. the user just flipped the Windows microphone privacy switch).
      const devs = await getAudioOutputDevices(defaultLabel, unnamedPrefix, { retryIfDenied: true });
      setAudioDevices(devs);
    } finally {
      setRefreshingAudio(false);
    }
  }, [t]);

  useEffect(() => {
    async function load() {
      try {
        const settings = await SettingsService.GetSettings();
        setSimType(settings.simType);
        setChatSound((settings.chatSound as ChatSoundType) || "default");
        setDiscordPresence(settings.discordPresence !== false);
        setAutoStartFlight(settings.autoStartFlight !== false);
        setConfirmCloseApp(settings.confirmCloseApp === true);
        setConfirmCancelFlight(settings.confirmCancelFlight === true);
        setConfirmFinishFlight(settings.confirmFinishFlight === true);
        setApiBaseURL(settings.apiBaseURL);
        if (settings.language) setLanguage(settings.language);
        if (settings.theme === "light" || settings.theme === "dark") {
          setTheme(settings.theme);
        }
        if (settings.audioOutputDevice) {
          setAudioDevice(settings.audioOutputDevice);
        }
        setLoaded(true);
      } catch {
        setLoaded(true);
      }
    }
    load();
  }, [setTheme]);

  useEffect(() => {
    const defaultLabel = t("settings.audioDeviceDefault", "Padrão do Sistema");
    const unnamedPrefix = t("settings.audioDeviceUnnamed", "Dispositivo de Áudio");
    getAudioOutputDevices(defaultLabel, unnamedPrefix).then(setAudioDevices);

    const unsubDevice = subscribeAudioDeviceChange((newDev) => {
      setAudioDevice(newDev);
      getAudioOutputDevices(defaultLabel, unnamedPrefix).then(setAudioDevices);
    });

    const handleVolumeEvent = (e: Event) => {
      const detail = (e as CustomEvent)?.detail;
      if (typeof detail?.volume === "number") {
        setVolume(detail.volume);
      }
    };
    window.addEventListener("acars-volume-changed", handleVolumeEvent);

    return () => {
      unsubDevice();
      window.removeEventListener("acars-volume-changed", handleVolumeEvent);
    };
  }, [t]);

  const handleThemeToggle = async (checked: boolean) => {
    const newTheme = checked ? "dark" : "light";
    setTheme(newTheme);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, theme: newTheme });
    } catch { /* ignore */ }
  };

  const handleChatSoundChange = async (value: string) => {
    const sound = value as ChatSoundType;
    setChatSound(sound);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, chatSound: sound });
    } catch { /* ignore */ }
    playNotificationPreview(sound);
  };

  const handleAudioDeviceChange = async (value: string) => {
    setAudioDevice(value);
    await setSelectedAudioDevice(value);
  };

  const handleTestAudio = async () => {
    setTestingAudio(true);
    try {
      await playDeviceTestSound(audioDevice, volume);
    } finally {
      setTimeout(() => setTestingAudio(false), 600);
    }
  };

  const handleVolumeChange = (newVol: number) => {
    setVolume(newVol);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("acars_volume", newVol.toString());
    }
    if (typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("acars-volume-changed", { detail: { volume: newVol } }));
    }
  };

  const handleDiscordToggle = async (checked: boolean) => {
    setDiscordPresence(checked);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, discordPresence: checked });
      await DiscordService.SetEnabled(checked);
    } catch { /* ignore */ }
  };

  const handleAutoStartToggle = async (checked: boolean) => {
    setAutoStartFlight(checked);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, autoStartFlight: checked });
    } catch { /* ignore */ }
  };

  const handleConfirmCloseAppToggle = async (checked: boolean) => {
    setConfirmCloseApp(checked);
    try {
      const settings = await SettingsService.GetSettings();
      settings.confirmCloseApp = checked;
      await SettingsService.UpdateSettings(settings);
    } catch { /* ignore */ }
  };

  const handleConfirmCancelFlightToggle = async (checked: boolean) => {
    setConfirmCancelFlight(checked);
    try {
      const settings = await SettingsService.GetSettings();
      settings.confirmCancelFlight = checked;
      await SettingsService.UpdateSettings(settings);
    } catch { /* ignore */ }
  };

  const handleConfirmFinishFlightToggle = async (checked: boolean) => {
    setConfirmFinishFlight(checked);
    try {
      const settings = await SettingsService.GetSettings();
      settings.confirmFinishFlight = checked;
      await SettingsService.UpdateSettings(settings);
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

  const handleLanguageChange = async (value: string) => {
    setLanguage(value);
    await i18n.changeLanguage(value);
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, language: value });
    } catch { /* ignore */ }
  };

  if (!loaded) return null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">{t("settings.title")}</h2>
        <p className="text-sm text-muted-foreground">
          {t("settings.subtitle")}
        </p>
      </div>

      <Separator />

      {devMode && (
        <Card className="border-border/50">
          <CardHeader>
            <CardTitle className="text-sm font-medium">{t("settings.connection")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between gap-4">
              <div className="shrink-0">
                <p className="text-sm font-medium">{t("settings.apiBaseUrl")}</p>
                <p className="text-xs text-muted-foreground">
                  {t("settings.apiBaseUrlDesc")}
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
                <p className="text-sm font-medium">{t("settings.localMode")}</p>
                <p className="text-xs text-muted-foreground">
                  {t("settings.localModeDesc")}
                </p>
              </div>
              <Switch checked={localMode} onCheckedChange={handleLocalModeToggle} />
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("settings.language")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.languageLabel")}</p>
              <p className="text-xs text-muted-foreground">{t("settings.languageDesc")}</p>
            </div>
            <Select value={language} onValueChange={handleLanguageChange}>
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {LANGUAGES.map((lang) => (
                  <SelectItem key={lang.code} value={lang.code}>
                    {lang.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("settings.appearance")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.darkMode")}</p>
              <p className="text-xs text-muted-foreground">{t("settings.darkModeDesc")}</p>
            </div>
            <Switch checked={theme === "dark"} onCheckedChange={handleThemeToggle} />
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("settings.audio", "Áudio")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Audio Output Device Selector */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">{t("settings.audioOutputDevice", "Dispositivo de Saída de Áudio")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.audioOutputDeviceDesc", "Selecione o canal ou dispositivo por onde o áudio do aplicativo será reproduzido.")}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Select
                value={audioDevice}
                onValueChange={handleAudioDeviceChange}
                onOpenChange={async (open) => {
                  if (open) {
                    await refreshAudioDevices();
                  }
                }}
              >
                <SelectTrigger className="w-[180px] sm:w-[220px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {audioDevices.map((d) => (
                    <SelectItem key={d.deviceId} value={d.deviceId}>
                      <div className="flex items-center gap-2 truncate max-w-[200px]">
                        {d.isDefault ? (
                          <Speaker className="h-3.5 w-3.5 text-sky-400 shrink-0" />
                        ) : (
                          <Headphones className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                        )}
                        <span className="truncate">{d.label}</span>
                      </div>
                    </SelectItem>
                  ))}
                  {!audioDevices.some((d) => d.deviceId === audioDevice) && audioDevice !== "default" && (
                    <SelectItem value={audioDevice}>
                      <div className="flex items-center gap-2 truncate max-w-[200px]">
                        <Headphones className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                        <span className="truncate">{audioDevice}</span>
                      </div>
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="icon"
                className="h-9 w-9 shrink-0 text-xs border-border/80"
                onClick={refreshAudioDevices}
                disabled={refreshingAudio}
                title={t("settings.audioRefresh", "Atualizar dispositivos")}
              >
                <RotateCw className={`h-3.5 w-3.5 ${refreshingAudio ? "animate-spin text-primary" : "text-muted-foreground"}`} />
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-9 px-3 gap-1.5 text-xs border-border/80 shrink-0"
                onClick={handleTestAudio}
                disabled={testingAudio}
                title={t("settings.audioTest", "Testar Áudio")}
              >
                <Volume2 className={`h-3.5 w-3.5 ${testingAudio ? "text-primary animate-pulse" : ""}`} />
                <span>{t("settings.audioTest", "Testar")}</span>
              </Button>
            </div>
          </div>

          <Separator />

          {/* Master Audio Track Volume */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">{t("settings.audioVolume", "Volume Geral")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.audioVolumeDesc", "Ajuste o volume dos avisos de cabine e notificações.")}
              </p>
            </div>
            <div className="flex items-center gap-3 w-full sm:w-[240px]">
              <input
                type="range"
                min="0"
                max="100"
                value={volume}
                onChange={(e) => handleVolumeChange(Number(e.target.value))}
                className="w-full h-1.5 bg-muted rounded-lg appearance-none cursor-pointer accent-primary"
              />
              <span className="font-mono text-xs w-9 text-right text-muted-foreground shrink-0">
                {volume}%
              </span>
            </div>
          </div>

          <Separator />

          {/* Chat Notification Sound */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">{t("settings.chatSound")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.chatSoundDesc")}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Select value={chatSound} onValueChange={handleChatSoundChange}>
                <SelectTrigger className="w-[140px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHAT_SOUNDS.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(`sounds.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="icon"
                className="h-9 w-9"
                onClick={() => playNotificationPreview(chatSound)}
                disabled={chatSound === "none"}
                title={t("settings.audioTest", "Testar")}
              >
                <Volume2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("settings.discord")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.richPresence")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.richPresenceDesc")}
              </p>
            </div>
            <Switch checked={discordPresence} onCheckedChange={handleDiscordToggle} />
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("settings.simulator")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.simType")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.simTypeDesc")}
              </p>
            </div>
            <Select value={simType} onValueChange={handleSimTypeChange}>
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto">{t("settings.simAuto")}</SelectItem>
                <SelectItem value="simconnect">{t("settings.simSimconnect")}</SelectItem>
                <SelectItem value="xplane">{t("settings.simXplane")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.autoStartFlight")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.autoStartFlightDesc")}
              </p>
            </div>
            <Switch checked={autoStartFlight} onCheckedChange={handleAutoStartToggle} />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.confirmCloseApp")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.confirmCloseAppDesc")}
              </p>
            </div>
            <Switch checked={confirmCloseApp} onCheckedChange={handleConfirmCloseAppToggle} />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.confirmCancelFlight")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.confirmCancelFlightDesc")}
              </p>
            </div>
            <Switch checked={confirmCancelFlight} onCheckedChange={handleConfirmCancelFlightToggle} />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("settings.confirmFinishFlight")}</p>
              <p className="text-xs text-muted-foreground">
                {t("settings.confirmFinishFlightDesc")}
              </p>
            </div>
            <Switch checked={confirmFinishFlight} onCheckedChange={handleConfirmFinishFlightToggle} />
          </div>
        </CardContent>
      </Card>

      <AboutSection />
    </div>
  );
}

type UpdateStatus = "idle" | "checking" | "up-to-date" | "update-available" | "downloading" | "done";

function AboutSection() {
  const { t } = useTranslation();
  const [version, setVersion] = useState("...");
  const [status, setStatus] = useState<UpdateStatus>("idle");
  const [latestVersion, setLatestVersion] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    UpdateService.GetCurrentVersion().then(setVersion).catch(() => {});
  }, []);

  const handleCheck = async () => {
    setStatus("checking");
    setError("");
    try {
      const info = await UpdateService.CheckForUpdate();
      if (info && info.updateAvailable) {
        setLatestVersion(info.latestVersion);
        setStatus("update-available");
      } else {
        setStatus("up-to-date");
      }
    } catch (e) {
      setError(String(e));
      setStatus("idle");
    }
  };

  const handleUpdate = async () => {
    setStatus("downloading");
    setError("");
    try {
      await UpdateService.ApplyUpdate();
      setStatus("done");
    } catch (e) {
      setError(String(e));
      setStatus("update-available");
    }
  };

  return (
    <Card className="border-border/50">
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("settings.about")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t("settings.application")}</span>
            <span>{t("settings.appName")}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t("settings.version")}</span>
            <span className="tabular-nums">{version}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t("settings.runtime")}</span>
            <span>Wails v3</span>
          </div>
        </div>

        <Separator />

        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <p className="text-sm font-medium">{t("settings.updates")}</p>
            <p className="text-xs text-muted-foreground">
              {status === "checking" && t("settings.checkingUpdates")}
              {status === "up-to-date" && t("settings.upToDate")}
              {status === "update-available" && (
                <>
                  {t("settings.updateAvailable")}{" "}
                  <Badge variant="secondary" className="ml-1">{latestVersion}</Badge>
                </>
              )}
              {status === "downloading" && t("settings.downloading")}
              {status === "done" && t("settings.updateDone")}
              {status === "idle" && t("settings.checkIdle")}
            </p>
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>

          <div className="flex gap-2 shrink-0">
            {(status === "idle" || status === "up-to-date") && (
              <Button variant="outline" size="sm" onClick={handleCheck}>
                {t("settings.checkForUpdates")}
              </Button>
            )}
            {status === "checking" && (
              <Button variant="outline" size="sm" disabled>
                {t("settings.checking")}
              </Button>
            )}
            {status === "update-available" && (
              <Button size="sm" onClick={handleUpdate}>
                {t("settings.updateNow")}
              </Button>
            )}
            {status === "downloading" && (
              <Button size="sm" disabled>
                {t("settings.downloadingBtn")}
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
