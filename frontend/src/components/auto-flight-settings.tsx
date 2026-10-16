import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@/components/ui/switch";
import { SettingsService } from "../../bindings/airspace-acars";

type AutoFlightSetting = "autoStartFlight" | "autoFinishFlight";

const ROWS: { key: AutoFlightSetting; label: string; desc: string }[] = [
  { key: "autoStartFlight", label: "settings.autoStartFlight", desc: "settings.autoStartFlightDesc" },
  { key: "autoFinishFlight", label: "settings.autoFinishFlight", desc: "settings.autoFinishFlightDesc" },
];

// The switches that let the ACARS start and finish flights on its own. Both
// default on, as the backend's settings do, so a settings file from before a
// switch existed reads as on.
export function AutoFlightSettings() {
  const { t } = useTranslation();
  const [values, setValues] = useState<Record<AutoFlightSetting, boolean>>({
    autoStartFlight: true,
    autoFinishFlight: true,
  });

  useEffect(() => {
    let cancelled = false;
    SettingsService.GetSettings()
      .then((s) => {
        if (cancelled) return;
        setValues({
          autoStartFlight: s?.autoStartFlight !== false,
          autoFinishFlight: s?.autoFinishFlight !== false,
        });
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const toggle = async (key: AutoFlightSetting, checked: boolean) => {
    setValues((v) => ({ ...v, [key]: checked }));
    try {
      const settings = await SettingsService.GetSettings();
      await SettingsService.UpdateSettings({ ...settings, [key]: checked });
    } catch {
      // As every other switch on this tab: the next load shows what was saved.
    }
  };

  return (
    <>
      {ROWS.map(({ key, label, desc }) => (
        <div key={key} className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">{t(label)}</p>
            <p className="text-xs text-muted-foreground">{t(desc)}</p>
          </div>
          <Switch
            checked={values[key]}
            onCheckedChange={(checked) => toggle(key, checked)}
            aria-label={t(label)}
          />
        </div>
      ))}
    </>
  );
}
