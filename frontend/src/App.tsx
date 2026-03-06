import { useState, useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { useAuth } from "@/context/auth-context";
import { LoginScreen } from "@/components/login-screen";
import { AppShell } from "@/components/app-shell";
import { SplashScreen } from "@/components/splash-screen";

function App() {
  const { isAuthenticated } = useAuth();
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

  return <AppShell />;
}

export default App;
