import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    rollupOptions: {
      input: {
        main: "index.html",
        status: "status.html",
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    exclude: ["e2e/**", "e2e-local/**", "**/node_modules/**", "**/.git/**"],
    globals: true,
    restoreMocks: true,
    clearMocks: true,
    mockReset: true,
  },
});
