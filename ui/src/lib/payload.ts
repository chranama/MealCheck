import type {
  GenerationMode,
  ManualItem,
  MealPlan,
  ProviderConfig,
  QualifyMealPlanPayload,
  RunPayload,
  Settings,
  SettingsDraft,
  VerificationConstraints,
} from "../types";
import { csvValue } from "./format";

export function normalizeSettings(settings: SettingsDraft): Settings {
  return {
    nutrition_targets: {
      calorie_target_kcal: Math.round(Number(settings.nutrition_targets.calorie_target_kcal)),
      protein_target_g: Math.round(Number(settings.nutrition_targets.protein_target_g)),
    },
    verification_constraints: normalizeVerificationConstraints(settings.verification_constraints),
  };
}

function normalizeVerificationConstraints(constraints: SettingsDraft["verification_constraints"]): VerificationConstraints {
  return {
    days: Math.round(Number(constraints.days)),
    meals_per_day: Math.round(Number(constraints.meals_per_day)),
    allergies: csvValue(constraints.allergies),
    excluded_foods: csvValue(constraints.excluded_foods),
    max_sodium_mg_per_day: Math.round(Number(constraints.max_sodium_mg_per_day)),
    max_added_sugar_g_per_meal: Number(constraints.max_added_sugar_g_per_meal),
    max_saturated_fat_pct_calories: Number(constraints.max_saturated_fat_pct_calories),
    calorie_tolerance_pct: Number(constraints.calorie_tolerance_pct),
    requires_prep_safety_notes: Boolean(constraints.requires_prep_safety_notes),
  };
}

export function buildManualPlan(items: ManualItem[], prepNotes: string, now: number = Date.now()): MealPlan {
  const rows = items
    .map((item) => ({
      day: Number(item.day),
      meal: item.meal,
      food: item.food,
      quantity: Number(item.quantity),
      unit: item.unit,
    }))
    .filter((row) => row.food && row.quantity > 0);
  if (rows.length === 0) {
    throw new Error("At least one manual food item is required.");
  }

  const dayMap = new Map<number, Map<string, Array<{ food: string; quantity: number; unit: string }>>>();
  rows.forEach((row) => {
    if (!dayMap.has(row.day)) {
      dayMap.set(row.day, new Map());
    }
    const mealMap = dayMap.get(row.day);
    if (!mealMap) return;
    if (!mealMap.has(row.meal)) {
      mealMap.set(row.meal, []);
    }
    mealMap.get(row.meal)?.push({
      food: row.food,
      quantity: row.quantity,
      unit: row.unit,
    });
  });

  const days = Array.from(dayMap.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([day, mealMap]) => ({
      day,
      meals: Array.from(mealMap.entries()).map(([name, mealItems]) => ({ name, items: mealItems })),
    }));

  return {
    schema_version: "0.1",
    plan_id: `manual-${now}`,
    description: "Manual structured plan from the MealCheck frontend.",
    days,
    shopping_list: rows.map((row) => ({
      food: row.food,
      quantity: row.quantity,
      unit: row.unit,
    })),
    prep_notes: prepNotes.split("\n").map((note) => note.trim()).filter(Boolean),
  };
}

function buildProviderConfig(provider: ProviderConfig): ProviderConfig {
  return {
    type: provider.type,
    base_url: provider.type === "openai_compatible" ? provider.base_url.trim().replace(/\/+$/, "") : "",
    model: provider.model.trim(),
    api_key: provider.api_key,
  };
}

function requireProviderConfig(provider: ProviderConfig): ProviderConfig {
  if (!provider.model.trim()) {
    throw new Error("Provider model is required.");
  }
  if (!provider.api_key.trim()) {
    throw new Error("Provider API key is required.");
  }
  return buildProviderConfig(provider);
}

export function buildQualificationPayload(args: {
  text: string;
  settings: SettingsDraft;
  provider: ProviderConfig;
}): QualifyMealPlanPayload {
  const text = args.text.trim();
  if (!text) {
    throw new Error("Candidate meal plan text is required.");
  }

  const payload: QualifyMealPlanPayload = {
    text,
    settings: normalizeSettings(args.settings),
  };
  if (args.provider.model.trim() || args.provider.api_key.trim()) {
    payload.provider = requireProviderConfig(args.provider);
  }
  return payload;
}

export function buildRunPayload(args: {
  mode: GenerationMode;
  settings: SettingsDraft;
  provider: ProviderConfig;
  generationPrompt: string;
  repairJSON: boolean;
}): RunPayload {
  const settings = normalizeSettings(args.settings);
  const provider = requireProviderConfig(args.provider);

  if (args.mode === "prompt_generation") {
    if (!args.generationPrompt.trim()) {
      throw new Error("Generation prompt is required.");
    }
    return {
      input_mode: args.mode,
      settings,
      provider,
      repair_json: args.repairJSON,
      generation_prompt: args.generationPrompt.trim(),
    };
  }

  return {
    input_mode: args.mode,
    settings,
    provider,
    repair_json: args.repairJSON,
  };
}
