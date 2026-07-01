import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-local",
  fullyParallel: false,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4173",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chrome",
      use: { ...devices["Desktop Chrome"], channel: "chrome" },
    },
  ],
  webServer: [
    {
      command: [
        "MEALCHECK_STORE=memory",
        "MEALCHECK_ACCESS_MODE=public_byok",
        "MEALCHECK_INVITE_TOKEN=invite-1",
        "MEALCHECK_ALLOWED_ORIGIN=http://127.0.0.1:4173",
        "MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH=../examples/seeded-one-day-peanut-allergy/plans/candidate.json",
        "go run ../cmd/mealcheck-server -root .. -store memory -addr 127.0.0.1:8081 -data-dir /tmp/mealcheck-local-e2e-data -artifact-dir /tmp/mealcheck-local-e2e-data/artifacts",
      ].join(" "),
      url: "http://127.0.0.1:8081/api/health",
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "npm run dev -- --strictPort",
      url: "http://127.0.0.1:4173",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
