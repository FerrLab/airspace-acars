import { Loader2 } from "lucide-react";
import { AppLogo } from "@/components/app-logo";
import { useTranslation } from "react-i18next";

export function SplashScreen() {
  const { t } = useTranslation();

  return (
    <div className="flex h-full items-center justify-center">
      <div className="flex flex-col items-center gap-6">
        <AppLogo className="h-16 w-16" />
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>{t("splash.checkingForUpdates")}</span>
        </div>
      </div>
    </div>
  );
}
