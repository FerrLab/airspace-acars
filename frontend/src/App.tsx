import { useState, useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { useAuth } from "@/context/auth-context";
import { LoginScreen } from "@/components/login-screen";
import { AppShell } from "@/components/app-shell";
import { SplashScreen } from "@/components/splash-screen";

function App() {
  const { isAuthenticated, tokenSynced } = useAuth();
  const [updateCheckDone, setUpdateCheckDone] = useState(false);

  useEffect(() => {
    const cancel = Events.On("update-check-done", () => {
      setUpdateCheckDone(true);
    });

    // Safety timeout in case the event is lost
    const timer = setTimeout(() => setUpdateCheckDone(true), 30_000);

    return () => {
      cancel();
      clearTimeout(timer);
    };
  }, []);

  if (!updateCheckDone) {
    return <SplashScreen />;
  }

  if (!isAuthenticated) {
    return <LoginScreen />;
  }

  // A stored token makes isAuthenticated true immediately on launch, but the
  // Go backend only has it after an async round-trip (see tokenSynced in
  // auth-context). Mounting AppShell before that lands sent the unread-chat
  // poll's first request out with no Authorization header — a 401 the
  // pilot never saw but Sentry did (AIRSPACE-ACARS-A).
  if (!tokenSynced) {
    return <SplashScreen />;
  }

  return <AppShell />;
}

export default App;
