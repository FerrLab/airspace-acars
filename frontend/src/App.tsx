import { useAuth } from "@/context/auth-context";
import { LoginScreen } from "@/components/login-screen";
import { AppShell } from "@/components/app-shell";

function App() {
  const { isAuthenticated } = useAuth();

  if (!isAuthenticated) {
    return <LoginScreen />;
  }

  return <AppShell />;
}

export default App;
