import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
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
      <App runtimeConfig={runtimeConfig} />
    </StrictMode>,
  );
}

bootstrap().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  const rootElement = document.getElementById("root");
  if (rootElement) {
    rootElement.textContent = `MealCheck failed to start: ${message}`;
  }
});
