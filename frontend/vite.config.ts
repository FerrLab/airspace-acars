import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";
import path from "path";
import { contentSecurityPolicyPlugin } from "./csp";

export default defineConfig({
  plugins: [react(), tailwindcss(), wails("./bindings"), contentSecurityPolicyPlugin()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
