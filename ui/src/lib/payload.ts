import type {
  Constraints,
  ConstraintsDraft,
  InputMode,
  ManualItem,
  MealPlan,
  Profile,
  ProviderConfig,
  RunPayload,
} from "../types";
import { csvValue } from "./format";

export function normalizeProfile(profile: Profile): Profile {
  return {
    age: Math.round(Number(profile.age)),
    sex: String(profile.sex).trim(),
    height_cm: Number(profile.height_cm),
    weight_kg: Number(profile.weight_kg),
    activity_level: String(profile.activity_level).trim(),
    goal: String(profile.goal).trim(),
    calorie_target_kcal: Math.round(Number(profile.calorie_target_kcal)),
    protein_target_g: Math.round(Number(profile.protein_target_g)),
  };
}

export function normalizeConstraints(constraints: ConstraintsDraft): Constraints {
  return {
    days: Math.round(Number(constraints.days)),
    meals_per_day: Math.round(Number(constraints.meals_per_day)),
    allergies: csvValue(constraints.allergies),
    excluded_foods: csvValue(constraints.excluded_foods),
    diet_pattern: String(constraints.diet_pattern).trim(),
    max_sodium_mg_per_day: Math.round(Number(constraints.max_sodium_mg_per_day)),
    max_added_sugar_g_per_meal: Number(constraints.max_added_sugar_g_per_meal),
    max_saturated_fat_pct_calories: Number(constraints.max_saturated_fat_pct_calories),
    calorie_tolerance_pct: Number(constraints.calorie_tolerance_pct),
    requires_shopping_list: Boolean(constraints.requires_shopping_list),
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

export function buildRunPayload(args: {
  mode: InputMode;
  profile: Profile;
  constraints: ConstraintsDraft;
  manualItems: ManualItem[];
  prepNotes: string;
  provider: ProviderConfig;
  generationPrompt: string;
  repairJSON: boolean;
}): RunPayload {
  const profile = normalizeProfile(args.profile);
  const constraints = normalizeConstraints(args.constraints);

  if (args.mode === "manual_structured") {
    return {
      input_mode: args.mode,
      profile,
      constraints,
      candidate_plan: buildManualPlan(args.manualItems, args.prepNotes),
    };
  }

  if (!args.provider.model.trim()) {
    throw new Error("Provider model is required.");
  }
  if (!args.provider.api_key.trim()) {
    throw new Error("Provider API key is required.");
  }

  const provider: ProviderConfig = {
    type: "openai_compatible",
    base_url: args.provider.base_url.trim(),
    model: args.provider.model.trim(),
    api_key: args.provider.api_key,
  };

  if (args.mode === "prompt_generation") {
    return {
      input_mode: args.mode,
      profile,
      constraints,
      provider,
      repair_json: args.repairJSON,
      generation_prompt: args.generationPrompt.trim(),
    };
  }

  return {
    input_mode: args.mode,
    profile,
    constraints,
    provider,
    repair_json: args.repairJSON,
  };
}
