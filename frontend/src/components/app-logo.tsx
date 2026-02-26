import { useTheme } from "@/context/theme-context";
import logoLight from "@/assets/logo-light.svg";
import logoDark from "@/assets/logo-dark.svg";

interface AppLogoProps {
  className?: string;
}

export function AppLogo({ className = "h-5 w-5" }: AppLogoProps) {
  const { theme } = useTheme();
  const src = theme === "dark" ? logoDark : logoLight;
  return <img src={src} alt="Airspace ACARS" className={className} />;
}
