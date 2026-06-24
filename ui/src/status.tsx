import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { StatusPage } from "./components/status/StatusPage";
import { loadRuntimeConfig } from "./lib/runtime_config";
import "./styles.css";

async function bootstrap() {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("Root element #root was not found.");
  }

  const runtimeConfig = await loadRuntimeConfig();
  createRoot(rootElement).render(
    <StrictMode>
      <StatusPage runtimeConfig={runtimeConfig} />
    </StrictMode>,
  );
}

bootstrap().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  const rootElement = document.getElementById("root");
  if (rootElement) {
    rootElement.textContent = `MealCheck status failed to start: ${message}`;
  }
});
