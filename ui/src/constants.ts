import type { Constraints, ManualItem, Profile, ProviderConfig, ProviderType, ReportTab } from "./types";

export const DEFAULT_PROFILE: Profile = {
  age: 35,
  sex: "male",
  height_cm: 178,
  weight_kg: 82,
  activity_level: "moderate",
  goal: "maintain_weight",
  calorie_target_kcal: 2000,
  protein_target_g: 98,
};

export const DEFAULT_CONSTRAINTS: Constraints = {
  days: 3,
  meals_per_day: 3,
  allergies: ["peanuts"],
  excluded_foods: ["shellfish"],
  diet_pattern: "general",
  max_sodium_mg_per_day: 2300,
  max_added_sugar_g_per_meal: 10,
  max_saturated_fat_pct_calories: 10,
  calorie_tolerance_pct: 15,
  requires_shopping_list: true,
  requires_prep_safety_notes: true,
};

export const DEFAULT_PROVIDER: ProviderConfig = {
  type: "openai",
  base_url: "",
  model: "",
  api_key: "",
};

export const PROVIDER_OPTIONS: Array<{ type: ProviderType; label: string; modelHint: string }> = [
  { type: "openai", label: "OpenAI", modelHint: "OpenAI model ID" },
  { type: "anthropic", label: "Anthropic", modelHint: "Claude model ID" },
  { type: "gemini", label: "Gemini", modelHint: "Gemini model ID" },
  { type: "openai_compatible", label: "OpenAI-compatible", modelHint: "Provider model ID" },
];

export const MVP_FOODS = [
  "cooked oatmeal",
  "blueberries",
  "plain Greek yogurt",
  "chicken breast",
  "brown rice",
  "broccoli",
  "salmon",
  "sweet potato",
  "spinach",
  "olive oil",
  "egg",
  "apple",
  "black beans",
  "whole wheat bread",
  "peanut sauce",
  "instant ramen",
  "soy sauce",
] as const;

export const MEALS = ["breakfast", "lunch", "dinner", "snack"] as const;
export const UNITS = ["g", "oz", "cup", "tbsp", "tsp", "serving"] as const;
export const TABS: ReportTab[] = ["checks", "nutrition", "foods", "sources", "report"];

export const INITIAL_MANUAL_ITEMS: ManualItem[] = [
  { id: "item-1", day: 1, meal: "breakfast", food: "cooked oatmeal", quantity: 1, unit: "cup" },
  { id: "item-2", day: 1, meal: "lunch", food: "chicken breast", quantity: 6, unit: "oz" },
  { id: "item-3", day: 1, meal: "dinner", food: "broccoli", quantity: 1, unit: "cup" },
];

export const DEFAULT_GENERATION_PROMPT = "Create a simple three-day high-protein meal-prep plan. Avoid peanuts and shellfish.";
export const DEFAULT_PREP_NOTES = "Refrigerate cooked foods within 2 hours.\nReheat leftovers until steaming.";
export const DEFAULT_CANDIDATE_TEXT = "Day 1 breakfast: 1 cup cooked oatmeal, 1 banana, and 1 cup plain Greek yogurt.\nDay 1 lunch: 6 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.\nDay 1 dinner: 6 oz salmon, 1 medium sweet potato, and 1 cup spinach.";
