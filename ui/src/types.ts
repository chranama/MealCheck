import type { ReactNode } from "react";

export type InputMode = "manual_structured" | "profile_generation" | "prompt_generation";
export type ViewMode = "demo" | "live";
export type ReportTab = "checks" | "nutrition" | "foods" | "sources" | "report";
export type RunStatus = "idle" | "queued" | "running" | "completed" | "failed" | "deleted" | string;
export type CheckStatus = "pass" | "warn" | "block" | string;

export type RuntimeConfig = {
  api?: {
    base_url?: string;
  };
  features?: Record<string, boolean | string | number | null>;
};

export type BackendState = {
  online: boolean;
  label: string;
  kind: "online" | "offline" | "idle";
};

export type DemoRun = {
  id: string;
  title: string;
  summary: string;
  base_path: string;
};

export type DemoIndex = {
  schema_version?: string;
  demo_runs?: DemoRun[];
};

export type DecisionCheck = {
  check_id: string;
  status: CheckStatus;
  message: string;
  affected_days?: number[];
  affected_meals?: string[];
  source_refs?: string[];
  evidence?: unknown[];
};

export type Decision = {
  decision: string;
  risk_level: string;
  summary: string;
  checks?: DecisionCheck[];
};

export type Report = {
  guideline_pack_id: string;
  constraint_summary: {
    max_sodium_mg_per_day?: number;
    max_saturated_fat_pct_calories?: number;
    [key: string]: unknown;
  };
  profile_summary: {
    calorie_target_kcal?: number;
    protein_target_g?: number;
    [key: string]: unknown;
  };
  [key: string]: unknown;
};

export type NutrientTotals = {
  energy_kcal: number;
  protein_g: number;
  sodium_mg: number;
  added_sugar_g: number;
  [key: string]: number;
};

export type DailyTotal = {
  day: number;
  nutrients: NutrientTotals;
  saturated_fat_pct_calories: number;
};

export type ResolvedFood = {
  day: number;
  meal: string;
  food: string;
  grams: number;
  nutrients: NutrientTotals;
};

export type UnresolvedFood = {
  day: number;
  meal: string;
  food: string;
  quantity_text?: string;
  unresolved_reason?: string;
  [key: string]: unknown;
};

export type SourceClaim = {
  claim_id: string;
  summary?: string;
  source_locator?: string;
};

export type CitationSource = {
  source_id: string;
  title: string;
  publisher?: string;
  url: string;
  claims_used?: SourceClaim[];
};

export type Citations = {
  sources?: CitationSource[];
};

export type Manifest = {
  mode: string;
  artifacts: string[];
};

export type ArtifactItem = {
  path: string;
  type: string;
  url: string;
};

export type ReportArtifacts = {
  apiBase?: string;
  base: string;
  decision: Decision;
  report: Report;
  totals: DailyTotal[];
  resolved: ResolvedFood[];
  unresolved: UnresolvedFood[];
  manifest: Manifest;
  pack?: unknown;
  citations: Citations;
  artifactItems?: ArtifactItem[] | null;
};

export type RunEvent = {
  type: string;
  message: string;
};

export type LiveState = {
  runID: string;
  status: RunStatus;
  message: string;
  events: RunEvent[];
  artifactItems: ArtifactItem[];
};

export type RunDocument = {
  run: {
    status: RunStatus;
    error?: string | null;
    summary?: string | null;
  };
};

export type ArtifactListResponse = {
  artifacts?: ArtifactItem[];
};

export type CreateRunResponse = {
  run_id: string;
  status: RunStatus;
};

export type Profile = {
  age: number;
  sex: "male" | "female" | string;
  height_cm: number;
  weight_kg: number;
  activity_level: string;
  goal: string;
  calorie_target_kcal: number;
  protein_target_g: number;
};

export type Constraints = {
  days: number;
  meals_per_day: number;
  allergies: string[];
  excluded_foods: string[];
  diet_pattern: string;
  max_sodium_mg_per_day: number;
  max_added_sugar_g_per_meal: number;
  max_saturated_fat_pct_calories: number;
  calorie_tolerance_pct: number;
  requires_shopping_list: boolean;
  requires_prep_safety_notes: boolean;
};

export type ConstraintsDraft = Omit<Constraints, "allergies" | "excluded_foods"> & {
  allergies: string;
  excluded_foods: string;
};

export type ProviderType = "openai" | "anthropic" | "gemini" | "openai_compatible";

export type ProviderConfig = {
  type: ProviderType;
  base_url: string;
  model: string;
  api_key: string;
};

export type ManualItem = {
  id: string;
  day: number;
  meal: string;
  food: string;
  quantity: number;
  unit: string;
};

export type MealPlanItem = {
  food: string;
  quantity: number;
  unit: string;
};

export type MealPlan = {
  schema_version: string;
  plan_id: string;
  description: string;
  days: Array<{
    day: number;
    meals: Array<{
      name: string;
      items: MealPlanItem[];
    }>;
  }>;
  shopping_list: MealPlanItem[];
  prep_notes: string[];
};

export type RunPayload =
  | {
      input_mode: "manual_structured";
      profile: Profile;
      constraints: Constraints;
      candidate_plan: MealPlan;
    }
  | {
      input_mode: "profile_generation";
      profile: Profile;
      constraints: Constraints;
      provider: ProviderConfig;
      repair_json: boolean;
    }
  | {
      input_mode: "prompt_generation";
      profile: Profile;
      constraints: Constraints;
      provider: ProviderConfig;
      repair_json: boolean;
      generation_prompt: string;
    };

export type FieldProps = {
  label: string;
  children: ReactNode;
};
