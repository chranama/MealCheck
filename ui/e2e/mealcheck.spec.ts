import { expect, test, type Page } from "@playwright/test";

const artifactBodies: Record<string, unknown> = {
  "decision.json": {
    decision: "pass",
    risk_level: "low",
    summary: "Plan fits the configured checks.",
    checks: [
      {
        check_id: "sodium_limit",
        status: "pass",
        message: "Sodium is within the configured daily limit.",
        affected_days: [1],
        source_refs: ["dga_sodium"],
      },
    ],
  },
  "report.json": {
    guideline_pack_id: "dga-2025-2030-us-adult-general-v1",
    constraint_summary: {
      max_sodium_mg_per_day: 2300,
      max_saturated_fat_pct_calories: 10,
    },
    profile_summary: {
      calorie_target_kcal: 2000,
      protein_target_g: 98,
    },
  },
  "daily-totals.json": [
    {
      day: 1,
      saturated_fat_pct_calories: 8,
      nutrients: {
        energy_kcal: 1980,
        protein_g: 102,
        sodium_mg: 1750,
        added_sugar_g: 8,
      },
    },
  ],
  "resolved-foods.json": [
    {
      day: 1,
      meal: "breakfast",
      food: "cooked oatmeal",
      grams: 234,
      nutrients: {
        energy_kcal: 160,
        protein_g: 6,
        sodium_mg: 5,
        added_sugar_g: 0,
      },
    },
  ],
  "unresolved-foods.json": [],
  "manifest.json": {
    mode: "manual_structured",
    artifacts: [
      "decision.json",
      "report.json",
      "report.pdf",
      "daily-totals.json",
      "resolved-foods.json",
      "unresolved-foods.json",
      "manifest.json",
      "guideline-pack/citations.json",
    ],
  },
  "report.pdf": "PDF placeholder",
  "guideline-pack/citations.json": {
    sources: [
      {
        source_id: "dga_sodium",
        title: "Dietary Guidelines for Americans",
        publisher: "USDA and HHS",
        url: "https://www.dietaryguidelines.gov/",
      },
    ],
  },
};

async function mockMealCheckApi(page: Page) {
  let runCounter = 0;
  const payloads: unknown[] = [];
  const deletedRunIDs: string[] = [];

  await page.route("**/mock-api/api/health", async (route) => {
    await route.fulfill({ json: { status: "ok" } });
  });
  await page.route("**/mock-api/api/runs", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers()["x-mealcheck-invite-token"]).toBe("invite-1");
    payloads.push(route.request().postDataJSON());
    runCounter += 1;
    await route.fulfill({
      status: 202,
      json: { run_id: `run-${runCounter}`, status: "queued" },
    });
  });

  await page.route(/\/mock-api\/api\/runs\/(run-\d+)$/, async (route) => {
    const runID = route.request().url().match(/\/mock-api\/api\/runs\/(run-\d+)$/)?.[1] || "run-1";
    if (route.request().method() === "DELETE") {
      deletedRunIDs.push(runID);
      await route.fulfill({ json: {} });
      return;
    }
    await route.fulfill({
      json: {
        run: {
          status: "completed",
          summary: "Artifacts ready.",
        },
      },
    });
  });

  await page.route(/\/mock-api\/api\/runs\/run-\d+\/events$/, async (route) => {
    await route.fulfill({
      contentType: "text/event-stream",
      body: 'data: {"type":"completed","message":"Artifacts ready."}\n\n',
    });
  });
  await page.route(/\/mock-api\/api\/runs\/run-\d+\/artifacts$/, async (route) => {
    await route.fulfill({
      json: {
        artifacts: Object.keys(artifactBodies).map((path) => ({
          path,
          type: "application/json",
          url: `/api/runs/run-1/artifacts/${path}`,
        })),
      },
    });
  });
  await page.route(/\/mock-api\/api\/runs\/run-\d+\/artifacts\/(.+)$/, async (route) => {
    const match = route.request().url().match(/\/mock-api\/api\/runs\/run-\d+\/artifacts\/(.+)$/);
    const path = decodeURIComponent(match?.[1] || "");
    await route.fulfill({ json: artifactBodies[path] ?? {} });
  });

  return { payloads, deletedRunIDs };
}

test("loads the live run homepage and can open a seeded demo", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck" })).toBeVisible();
  await expect(page.locator(".brand-mark")).toBeVisible();
  await expect(page.locator("#live-workspace")).toBeVisible();
  await expect(page.locator(".live-action-strip")).toBeVisible();
  await expect(page.locator(".mode-icon")).toHaveCount(0);
  await expect(page.locator(".nav-icon")).toHaveCount(0);
  await expect(page.locator(".pipeline-graphic")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /New meal check/ })).toHaveClass(/is-active/);
  await expect(page.getByLabel("Service URL")).toHaveCount(0);
  await expect(page.locator("#backend-guidance")).toHaveCount(0);
  await expect(page.getByText("Service ready")).toHaveCount(0);
  await expect(page.getByLabel("Access code")).toBeVisible();
  await expect(page.getByText("Advanced constraints")).toBeVisible();
  await expect(page.getByRole("button", { name: /Three-day peanut allergy check/ })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Nutrition" })).toHaveCount(0);

  await page.getByRole("button", { name: /Three-day peanut allergy check/ }).click();
  await expect(page.getByText("Healthy adult seeded plan with allergen")).toBeVisible();
  await expect(page.getByRole("tab", { name: "Nutrition" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Report" })).toBeVisible();
});

test("creates and deletes a mocked live manual run", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await expect(page.locator(".backend-status")).toHaveCount(0);
  await expect(page.locator("#backend-guidance")).toHaveCount(0);
  await expect(page.getByText("Service ready")).toHaveCount(0);
  await page.getByLabel("Access code").fill("invite-1");
  await page.getByRole("button", { name: "Create Report" }).click();

  await expect(page.getByText("run-1").first()).toBeVisible();
  await expect(page.getByRole("tab", { name: "Checks" })).toBeVisible();
  await expect(page.getByText("Sodium is within the configured daily limit.")).toBeVisible();
  await page.getByRole("tab", { name: "Report" }).click();
  await expect(page.getByRole("link", { name: "Download report PDF" })).toBeVisible();
  expect(api.payloads[0]).toMatchObject({ input_mode: "manual_structured" });

  await page.getByRole("button", { name: "Delete Report" }).click();
  await expect(page.getByRole("dialog", { name: "Delete report?" })).toBeVisible();
  await page.getByRole("dialog", { name: "Delete report?" }).getByRole("button", { name: "Delete Report" }).click();
  await expect(page.getByText("Report deleted.").first()).toBeVisible();
  await expect(page.getByRole("tab", { name: "Checks" })).toHaveCount(0);
  expect(api.deletedRunIDs).toEqual(["run-1"]);
});

test("creates a mocked BYOK profile-generation run without persisting provider keys", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Profile" }).click();
  await expect(page.getByText("BYOK provider disclosure")).toBeVisible();
  await page.getByLabel("Access code").fill("invite-1");
  await page.getByLabel("Model").fill("gpt-test");
  await page.getByLabel("API key").fill("secret-profile-key");
  await page.getByRole("button", { name: "Create Report" }).click();

  await expect(page.getByText("run-1").first()).toBeVisible();
  expect(api.payloads[0]).toMatchObject({
    input_mode: "profile_generation",
    provider: {
      model: "gpt-test",
      api_key: "secret-profile-key",
    },
  });
  await expect(page.getByLabel("API key")).toHaveValue("");
  await expect(page.getByText("secret-profile-key")).toHaveCount(0);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain("secret-profile-key");
});

test("creates a mocked BYOK prompt-generation run", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Prompt" }).click();
  await page.getByLabel("Access code").fill("invite-1");
  await page.getByLabel("Model").fill("gpt-test");
  await page.getByLabel("API key").fill("secret-prompt-key");
  await page.getByLabel("Prompt").fill("Create a two-day meal plan with salmon and oatmeal.");
  await page.getByRole("button", { name: "Create Report" }).click();

  await expect(page.getByText("run-1").first()).toBeVisible();
  expect(api.payloads[0]).toMatchObject({
    input_mode: "prompt_generation",
    generation_prompt: "Create a two-day meal plan with salmon and oatmeal.",
    provider: {
      model: "gpt-test",
      api_key: "secret-prompt-key",
    },
  });
  await expect(page.getByLabel("API key")).toHaveValue("");
  expect(await page.evaluate(() => document.body.textContent || "")).not.toContain("secret-prompt-key");
});
