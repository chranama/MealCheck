import type { Citations, RunStatus } from "../types";

export function readableID(entry: unknown): string {
  return String(entry || "")
    .replace(/_/g, " ")
    .replace(/-/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function round(entry: unknown): number {
  return Math.round(Number(entry) * 10) / 10;
}

export function valueText(entry: unknown): string {
  if (entry === undefined || entry === null || entry === "") {
    return "-";
  }
  if (typeof entry === "number") {
    return String(round(entry));
  }
  return String(entry);
}

export function artifactType(path: string): string {
  if (path.endsWith(".pdf")) return "PDF";
  if (path.endsWith(".jsonl")) return "JSONL";
  if (path.endsWith(".json")) return "JSON";
  if (path.endsWith(".html")) return "HTML";
  if (path.endsWith(".md")) return "Markdown";
  return "File";
}

export function liveStatusTone(status: RunStatus): string {
  if (status === "completed") return "pass";
  if (status === "failed" || status === "deleted") return "block";
  if (status === "queued" || status === "running") return "warn";
  return "info";
}

export function modeLabel(mode: string): string {
  if (mode === "manual_structured") return "Manual";
  if (mode === "profile_generation") return "Targets";
  return "Prompt";
}

export function sourceChip(citations: Citations, sourceID: string): string {
  const source = citations?.sources?.find((candidate) => candidate.source_id === sourceID);
  return source ? source.title : readableID(sourceID);
}

export function csvValue(value: unknown): string[] {
  return String(value || "").split(",").map((entry) => entry.trim()).filter(Boolean);
}

export function guidelineLabel(entry: unknown): string {
  const id = String(entry || "");
  if (id === "dga-2025-2030-us-adult-general-v1") {
    return "Dietary Guidelines for Americans, 2025-2030";
  }
  return readableID(id);
}

export function checkLabel(entry: unknown): string {
  const id = String(entry || "");
  const labels: Record<string, string> = {
    required_meals_present: "Required meals present",
    quantities_resolvable: "Food quantities are clear",
    allergens_absent: "Declared allergens are absent",
    excluded_foods_absent: "Excluded foods are absent",
    calories_within_tolerance: "Calories are within target range",
    sodium_under_limit: "Sodium is under the daily limit",
    added_sugar_under_limit: "Added sugar is under the meal limit",
    saturated_fat_under_limit: "Saturated fat is under the daily limit",
    protein_minimum_met: "Protein target is met",
    food_group_coverage: "Food group coverage is present",
    prep_safety_mentions_present: "Prep safety notes are present",
    sodium_limit: "Sodium is under the daily limit",
  };
  return labels[id] || readableID(id);
}

export function sourceClaimLabel(entry: unknown): string {
  const id = String(entry || "");
  const labels: Record<string, string> = {
    "dga-protein-grams-per-kg": "Protein target guidance",
    "dga-saturated-fat-pct-calories": "Saturated fat limit",
    "dga-added-sugar-grams-per-meal": "Added sugar limit",
    "dga-sodium-mg-per-day": "Sodium daily limit",
    "dga-food-group-serving-goals": "Food group serving goals",
    "dga-dairy-servings-2000-kcal": "Dairy serving goal",
    "dga-vegetables-servings-2000-kcal": "Vegetable serving goal",
    "dga-fruits-servings-2000-kcal": "Fruit serving goal",
    "dga-whole-grains-servings-2000-kcal": "Whole grain serving goal",
    "dga-grain-snack-added-sugar-limit": "Grain snack added sugar limit",
    "dga-dairy-snack-added-sugar-limit": "Dairy snack added sugar limit",
    "fda-nine-major-allergens": "Major food allergens",
    "fda-no-allergen-threshold": "Allergen threshold policy",
    "foodsafety-clean-separate-cook-chill": "Core food-safety steps",
    "foodsafety-refrigerate-leftovers": "Leftover refrigeration timing",
    "foodsafety-wash-hands-20-seconds": "Hand-washing guidance",
    "foodsafety-hot-holding-140f": "Hot holding temperature",
    "foodsafety-danger-zone-40-140f": "Food temperature danger zone",
    "foodsafety-microwave-165f": "Microwave cooking temperature",
    "foodsafety-refrigerator-freezer-temperatures": "Refrigerator and freezer temperatures",
    "dri-profile-inputs": "Target-based nutrient estimates",
    "fdc-api-search-details": "FoodData Central search details",
  };
  return labels[id] || readableID(id);
}

export function reasonLabel(entry: unknown): string {
  const id = String(entry || "");
  if (id.startsWith("missing_conversion:")) {
    return `an unsupported ${id.split(":")[1] || "unit"} conversion`;
  }
  const labels: Record<string, string> = {
    ambiguous_food: "an ambiguous food name",
    composed_food_needs_decomposition: "a mixed dish that needs ingredient details",
    missing_conversion: "an unsupported unit conversion",
    non_food_text: "non-food text",
    preparation_unclear: "unclear preparation details",
    restaurant_or_branded_food: "a restaurant or branded food MealCheck cannot resolve yet",
    unknown_food: "an unknown food",
    unsupported_unit: "an unsupported unit",
    vague_quantity: "a vague quantity",
  };
  return labels[id] || readableID(id).toLowerCase();
}

export function isMealPlanCheckID(entry: unknown): boolean {
  const id = String(entry || "").toLowerCase();
  return !/(schema|contract|json|manifest|artifact)/.test(id);
}
