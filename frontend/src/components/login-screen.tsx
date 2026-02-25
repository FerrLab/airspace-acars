import { useState } from "react";
import { useAuth } from "@/context/auth-context";
import { TenantSelector } from "@/components/tenant-selector";
import { DeviceCodeAuth } from "@/components/device-code-auth";

type Step = "tenant" | "auth";

export function LoginScreen() {
  const { tenant } = useAuth();
  const [step, setStep] = useState<Step>(tenant ? "auth" : "tenant");

  if (step === "tenant") {
    return (
      <TenantSelector
        onTenantSelected={(hasToken) => {
          if (!hasToken) setStep("auth");
          // If hasToken is true, auth context already set the token
          // and isAuthenticated will flip to true, unmounting LoginScreen
        }}
      />
    );
  }

  return <DeviceCodeAuth onBack={() => setStep("tenant")} />;
}
