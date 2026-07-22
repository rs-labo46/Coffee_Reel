import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react(), tailwindcss()],

  server: {
    host: "0.0.0.0",
    port: 3000,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/tests/setup.ts"],

    env: {
      VITE_API_URL: "http://localhost:8080",
    },

    clearMocks: true,
    restoreMocks: true,
    unstubGlobals: true,
  },
});
