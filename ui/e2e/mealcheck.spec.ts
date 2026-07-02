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
    mode: "profile_generation",
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

type MockMealCheckApiOptions = {
  createRunError?: {
    status: number;
    body: unknown;
  };
  qualification?: unknown;
};

async function mockMealCheckApi(page: Page, options: MockMealCheckApiOptions = {}) {
  let runCounter = 0;
  const payloads: unknown[] = [];
  const deletedRunIDs: string[] = [];

  await page.route("**/mock-api/api/health", async (route) => {
    await route.fulfill({ json: { status: "ok", store: "mock", access_mode: "public_byok", public_openai_compatible: false } });
  });
  await page.route("**/mock-api/api/status", async (route) => {
    await route.fulfill({
      json: {
        schema_version: "0.1",
        generated_at: "2026-06-24T12:42:00Z",
        overall: { state: "operational", message: "All systems operational" },
        components: [
          { id: "meal_check_submission", name: "Meal Check Submission", state: "operational" },
          { id: "ai_meal_normalization", name: "AI Meal Normalization", state: "operational" },
          { id: "nutrition_allergen_checking", name: "Nutrition & Allergen Checking", state: "operational" },
          { id: "report_generation", name: "Report Generation", state: "operational" },
          { id: "sample_report", name: "Sample Report", state: "operational" },
        ],
        recent_incidents: [],
        links: {
          sample_report: "/api/demo-runs/seeded-one-day-peanut-allergy/report",
        },
      },
    });
  });
  await page.route("**/mock-api/api/runs", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers()["x-mealcheck-invite-token"]).toBeUndefined();
    payloads.push(route.request().postDataJSON());
    if (options.createRunError) {
      await route.fulfill({
        status: options.createRunError.status,
        json: options.createRunError.body,
      });
      return;
    }
    runCounter += 1;
    await route.fulfill({
      status: 202,
      json: { run_id: `run-${runCounter}`, status: "queued" },
    });
  });
  await page.route("**/mock-api/api/qualify", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers()["x-mealcheck-invite-token"]).toBeUndefined();
    payloads.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      json: options.qualification ? { qualification: options.qualification } : {
        qualification: {
          schema_version: "0.1",
          status: "eligible_for_verification",
          reason: "Candidate text was normalized into a MealCheck plan.",
          provider_used: false,
          normalized_plan: {
            schema_version: "0.1",
            plan_id: "mock-normalized",
            description: "Mock normalized plan.",
            days: [
              {
                day: 1,
                meals: [
                  {
                    name: "breakfast",
                    items: [{ food: "cooked oatmeal", quantity: 1, unit: "cup" }],
                  },
                ],
              },
            ],
            shopping_list: [],
            prep_notes: [],
          },
        },
      },
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

async function expectSharedFooter(page: Page) {
  const footer = page.getByRole("contentinfo");
  await expect(footer.getByRole("link", { name: "MealCheck" })).toHaveAttribute("href", "/");
  await expect(footer.getByRole("link", { name: "Status" })).toHaveAttribute("href", "/status.html");
  await expect(footer.getByRole("link", { name: "About" })).toHaveAttribute("href", "/about.html");
  await expect(footer.getByRole("link", { name: "GitHub" })).toHaveAttribute("href", "https://github.com/chranama/MealCheck");
}

async function expectNoPrimaryActionOverflow(page: Page) {
  const pageWidth = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(pageWidth.scrollWidth, JSON.stringify(pageWidth)).toBeLessThanOrEqual(pageWidth.clientWidth + 1);

  const overflowingButtons = await page.locator(".action-strip-actions .action-button").evaluateAll((buttons) => (
    buttons
      .map((button) => button as HTMLElement)
      .filter((button) => button.scrollWidth > button.clientWidth + 1)
      .map((button) => button.textContent?.trim() || "")
  ));
  expect(overflowingButtons).toEqual([]);
}

test("loads the live run homepage without seeded demo navigation", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck" })).toBeVisible();
  await expect(page.locator(".brand-mark")).toBeVisible();
  await expect(page.locator("#live-workspace")).toBeVisible();
  await expect(page.locator(".live-action-strip")).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "Check a bounded meal plan" })).toBeVisible();
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
  await expect(page.getByRole("tab", { name: "Nutrition" })).toHaveCount(0);
  await expect(page.getByText("Ready")).toHaveCount(0);
  await expect(page.getByText("Not started")).toHaveCount(0);
  await expectSharedFooter(page);
});

test("keeps the primary workflow usable on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck" })).toBeVisible();
  await expect(page.getByLabel("Meal plan text")).toBeVisible();
  await expectNoPrimaryActionOverflow(page);

  await page.getByRole("button", { name: "Check Eligibility" }).click();

  await expect(page.getByText("Eligible For Verification")).toBeVisible();
  await expectNoPrimaryActionOverflow(page);
  expect(api.payloads[0]).toMatchObject({
    text: expect.stringContaining("For breakfast I will have 1 cup cooked oatmeal"),
  });
});

test("loads the public status page with summarized component states", async ({ page }) => {
  await mockMealCheckApi(page);
  await page.goto("/status.html?api=/mock-api");

  await expect(page.getByRole("heading", { level: 1, name: "MealCheck Status" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "All systems operational" })).toBeVisible();
  await expect(page.getByText("Website")).toBeVisible();
  await expect(page.getByText("Meal Check Submission")).toBeVisible();
  await expect(page.getByText("AI Meal Normalization")).toBeVisible();
  await expect(page.getByText("Nutrition & Allergen Checking")).toBeVisible();
  await expect(page.getByText("Report Generation")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Sample Report" })).toBeVisible();
  await expect(page.getByText("No incidents reported in the past 7 days.")).toBeVisible();
  await expect(page.getByText("queue_size")).toHaveCount(0);
  await expect(page.getByText("local_model")).toHaveCount(0);
  await expectSharedFooter(page);
});

test("loads the consumer about page with shared footer navigation", async ({ page }) => {
  await page.goto("/about.html");

  await expect(page.getByRole("heading", { level: 1, name: "Bounded checks for meal plans." })).toBeVisible();
  await expectSharedFooter(page);
});

test("qualifies mocked candidate text", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await expect(page.locator(".backend-status")).toHaveCount(0);
  await expect(page.locator("#backend-guidance")).toHaveCount(0);
  await expect(page.getByText("Service ready")).toHaveCount(0);
  await page.getByRole("button", { name: "Check Eligibility" }).click();

  await expect(page.getByText("Candidate text was normalized into a MealCheck plan.")).toBeVisible();
  await expect(page.getByText("Eligible For Verification")).toBeVisible();
  await expect(page.getByText("1 day, 1 meal, 1 item")).toBeVisible();
  await expect(page.getByRole("tab", { name: "Checks" })).toHaveCount(0);
  expect(api.payloads[0]).toMatchObject({
    text: expect.stringContaining("For breakfast I will have 1 cup cooked oatmeal"),
  });
});

test("shows recovery guidance for vague meal-plan input", async ({ page }) => {
  await mockMealCheckApi(page, {
    qualification: {
      schema_version: "0.1",
      status: "meal_plan_too_vague",
      reason: "Breakfast, lunch, and dinner are named but quantities are missing.",
      missing_fields: ["quantities"],
      provider_used: false,
    },
  });
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Check Eligibility" }).click();

  await expect(page.getByText("Add amounts and units")).toBeVisible();
  await expect(page.getByText(/Add quantities such as 1 cup, 4 oz, 2 eggs, or 1 tbsp/)).toBeVisible();
  await expect(page.getByText("Meal Plan Too Vague")).toBeVisible();
});

test("shows service recovery guidance when the report queue is full", async ({ page }) => {
  await mockMealCheckApi(page, {
    createRunError: {
      status: 429,
      body: {
        error: {
          code: "queue_full",
          message: "run queue is full",
          details: { queue_size: 3 },
        },
      },
    },
  });
  await page.goto("/?api=/mock-api");

  await page.getByLabel("Model").fill("gpt-test");
  await page.getByLabel("API key").fill("secret-queue-key");
  await page.getByRole("button", { name: "Create Report" }).click();

  const alert = page.getByRole("alert");
  await expect(alert.getByText("MealCheck is busy")).toBeVisible();
  await expect(alert.getByText("The report queue is full right now. Your input was not submitted.")).toBeVisible();
  await expect(alert.getByRole("link", { name: "Open status page" })).toHaveAttribute("href", "/status.html");
  await expect(page.getByText("HTTP 429")).toHaveCount(0);
});

test("creates a mocked BYOK profile-generation run without persisting provider keys", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Targets" }).click();
  await expect(page.getByText("Model provider disclosure")).toBeVisible();
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

test("lets keyboard users cancel report deletion", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Targets" }).click();
  await page.getByLabel("Model").fill("gpt-test");
  await page.getByLabel("API key").fill("secret-profile-key");
  await page.getByRole("button", { name: "Create Report" }).click();
  await expect(page.getByText("run-1").first()).toBeVisible();

  const deleteButton = page.locator("#delete-run-button");
  await deleteButton.focus();
  await page.keyboard.press("Enter");

  const dialog = page.getByRole("dialog", { name: "Delete report?" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  expect(api.deletedRunIDs).toEqual([]);

  await deleteButton.focus();
  await page.keyboard.press("Enter");
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(dialog).toHaveCount(0);
  expect(api.deletedRunIDs).toEqual([]);
});

test("creates a mocked BYOK prompt-generation run", async ({ page }) => {
  const api = await mockMealCheckApi(page);
  await page.goto("/?api=/mock-api");

  await page.getByRole("button", { name: "Prompt" }).click();
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
