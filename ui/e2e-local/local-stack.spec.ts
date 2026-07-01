import { expect, test } from "@playwright/test";

const apiBase = "http://127.0.0.1:8081";
const providerKey = "local-e2e-provider-key";

test.describe.configure({ mode: "serial" });

test("renders the live run homepage without an API base", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck" })).toBeVisible();
  await expect(page.locator(".brand-mark")).toBeVisible();
  await expect(page.locator("#live-workspace")).toBeVisible();
  await expect(page.locator(".live-action-strip")).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "Check a meal plan" })).toBeVisible();
  await expect(page.locator(".mode-icon")).toHaveCount(0);
  await expect(page.locator(".nav-icon")).toHaveCount(0);
  await expect(page.locator(".pipeline-graphic")).toHaveCount(0);
  await expect(page.getByLabel("Service URL")).toHaveCount(0);
  await expect(page.locator("#backend-guidance")).toHaveCount(0);
  await expect(page.getByText("Service ready")).toHaveCount(0);
  await expect(page.getByLabel("Access code")).toHaveCount(0);
  await expect(page.getByLabel("Meal plan text")).toBeVisible();
  await expect(page.getByText("Model Provider", { exact: true })).toBeVisible();
  await expect(page.getByText("Verification Settings")).toBeVisible();
  await expect(page.getByText("Advanced constraints")).toBeHidden();
  await expect(page.getByRole("button", { name: /One-day peanut allergy check/ })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "Checks" })).toHaveCount(0);
});

test("renders the public status page against the local backend", async ({ page }) => {
  await page.goto(`/status.html?api=${apiBase}`);

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck Status" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "All systems operational" })).toBeVisible();
  await expect(page.getByText("Meal Check Submission")).toBeVisible();
  await expect(page.getByText("AI Meal Normalization")).toBeVisible();
  await expect(page.getByText("Nutrition & Allergen Checking")).toBeVisible();
  await expect(page.getByText("Report Generation")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Sample Report" })).toBeVisible();
  await expect(page.getByText("No incidents reported in the past 7 days.")).toBeVisible();
});

test("qualifies a real local candidate meal plan through the fake provider", async ({ page }) => {
  await page.goto(`/?api=${apiBase}`);

  await expect(page.locator(".backend-status")).toHaveCount(0);
  await expect(page.locator("#backend-guidance")).toHaveCount(0);
  await expect(page.getByText("Service ready")).toHaveCount(0);
  await page.getByLabel("Model").fill("fake-meal-plan");
  await page.getByLabel("API key").fill(providerKey);

  const qualifyResponsePromise = page.waitForResponse((response) => (
    response.url() === `${apiBase}/api/qualify` &&
    response.request().method() === "POST"
  ));
  await page.getByRole("button", { name: "Check Eligibility" }).click();
  const qualified = await (await qualifyResponsePromise).json() as { qualification: { status: string; provider_used: boolean } };

  expect(["eligible_for_verification", "eligible_with_unresolved_items"]).toContain(qualified.qualification.status);
  expect(qualified.qualification.provider_used).toBeTruthy();
  await expect(page.getByText(/Eligible (For Verification|With Unresolved Items)/)).toBeVisible();
  await expect(page.getByLabel("API key")).toHaveValue("");
  expect(await page.evaluate(() => document.body.textContent || "")).not.toContain(providerKey);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(providerKey);
});

test("creates a real local BYOK run through the fake provider and redacts secrets", async ({ page }) => {
  await page.goto(`/?api=${apiBase}`);

  await page.getByRole("button", { name: "Targets" }).click();
  await expect(page.getByText("Model provider disclosure")).toBeVisible();
  await page.getByLabel("Model").fill("fake-meal-plan");
  await page.getByLabel("API key").fill(providerKey);

  const createResponsePromise = page.waitForResponse((response) => (
    response.url() === `${apiBase}/api/runs` &&
    response.request().method() === "POST"
  ));
  await page.getByRole("button", { name: "Create Report" }).click();
  const created = await (await createResponsePromise).json() as { run_id: string };

  await expect(page.getByText(created.run_id).first()).toBeVisible();
  await expect(page.getByRole("tab", { name: "Checks" })).toBeVisible();
  await expect(page.getByLabel("API key")).toHaveValue("");
  expect(await page.evaluate(() => document.body.textContent || "")).not.toContain(providerKey);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(providerKey);

  const artifactsResp = await page.request.get(`${apiBase}/api/runs/${created.run_id}/artifacts`);
  expect(artifactsResp.ok()).toBeTruthy();
  const artifacts = await artifactsResp.json() as { artifacts: Array<{ path: string; url: string }> };
  expect(artifacts.artifacts.map((artifact) => artifact.path)).toContain("configs/redacted-provider.json");
  expect(artifacts.artifacts.map((artifact) => artifact.path)).toContain("optional/llm-output.json");

  for (const artifact of artifacts.artifacts) {
    const artifactResp = await page.request.get(`${apiBase}${artifact.url}`);
    expect(artifactResp.ok()).toBeTruthy();
    expect(await artifactResp.text()).not.toContain(providerKey);
  }

  const redactedResp = await page.request.get(`${apiBase}/api/runs/${created.run_id}/artifacts/configs/redacted-provider.json`);
  expect(await redactedResp.json()).toMatchObject({ api_key: "redacted" });

  await page.request.delete(`${apiBase}/api/runs/${created.run_id}`);
});

test("allows only the configured local frontend origin through CORS", async ({ request }) => {
  const allowed = await request.fetch(`${apiBase}/api/runs`, {
    method: "OPTIONS",
    headers: {
      Origin: "http://127.0.0.1:4173",
      "Access-Control-Request-Method": "POST",
    },
  });
  expect(allowed.status()).toBe(204);
  expect(allowed.headers()["access-control-allow-origin"]).toBe("http://127.0.0.1:4173");

  const disallowed = await request.fetch(`${apiBase}/api/runs`, {
    method: "OPTIONS",
    headers: {
      Origin: "https://example.invalid",
      "Access-Control-Request-Method": "POST",
    },
  });
  expect(disallowed.status()).toBe(204);
  expect(disallowed.headers()["access-control-allow-origin"]).toBeUndefined();
});
