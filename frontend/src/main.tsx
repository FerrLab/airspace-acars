import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { AuthProvider } from "@/context/auth-context";
import { ThemeProvider } from "@/context/theme-context";
import { TooltipProvider } from "@/components/ui/tooltip";
import i18n from "@/lib/i18n";
import { SettingsService } from "../bindings/airspace-acars";
import "./index.css";

async function boot() {
  try {
    const settings = await SettingsService.GetSettings();
    if (settings.language) {
      await i18n.changeLanguage(settings.language);
    }
  } catch {
    // settings not available yet — keep default "en"
  }

  ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
    <React.StrictMode>
      <ThemeProvider>
        <AuthProvider>
          <TooltipProvider>
            <App />
          </TooltipProvider>
        </AuthProvider>
      </ThemeProvider>
    </React.StrictMode>
  );
}

boot();
