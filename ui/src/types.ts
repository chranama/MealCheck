import type { ReactNode } from "react";

export type GenerationMode = "profile_generation" | "prompt_generation";
export type InputMode = GenerationMode;
export type QualificationStatus =
  | "not_meal_plan"
  | "meal_plan_too_vague"
  | "recipe_or_menu_needs_decomposition"
  | "meal_plan_unsupported_units"
  | "meal_plan_outside_hosted_contract"
  | "eligible_for_verification"
  | "eligible_with_unresolved_items"
  | string;
export type ReportTab = "checks" | "nutrition" | "foods" | "recommendation" | "normalization" | "sources" | "report";
export type RunStatus = "idle" | "queued" | "running" | "completed" | "failed" | "deleted" | string;
export type CheckStatus = "pass" | "warn" | "block" | string;
export type AccessMode = "public_byok" | "invite_required" | string;
export type HostedMode = "byok" | "local_model" | string;
export type PublicStatusState =
  | "operational"
  | "degraded_performance"
  | "partial_outage"
  | "major_outage"
  | "maintenance"
  | "unknown"
  | string;

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
  accessMode: AccessMode;
  hostedMode: HostedMode;
  publicOpenAICompatible: boolean;
  maxCandidateTextChars?: number;
  maxGenerationPromptChars?: number;
  localModel?: LocalModelHealth;
};

export type HealthResponse = {
  status: string;
  store: string;
  access_mode?: AccessMode;
  hosted_mode?: HostedMode;
  queued_runs?: number;
  running_runs?: number;
  queue_size?: number;
  active_run_limit?: number;
  retention_days?: number;
  public_openai_compatible?: boolean;
  max_candidate_text_chars?: number;
  max_generation_prompt_chars?: number;
  local_model?: LocalModelHealth;
  policy?: Record<string, unknown>;
};

export type LocalModelHealth = {
  enabled?: boolean;
  ready?: boolean;
  model?: string;
  max_input_chars?: number;
  max_source_items?: number;
  max_output_tokens?: number;
  timeout_sec?: number;
  supported_days?: number;
  supported_meals_per_day?: number;
  error?: string;
};

export type PublicStatusResponse = {
  schema_version: string;
  generated_at: string;
  overall: StatusSummary;
  components: StatusComponent[];
  recent_incidents: StatusIncident[];
  links?: StatusLinks;
};

export type StatusSummary = {
  state: PublicStatusState;
  message: string;
};

export type StatusComponent = {
  id: string;
  name: string;
  state: PublicStatusState;
  message?: string;
};

export type StatusIncident = {
  id: string;
  title: string;
  state: PublicStatusState;
  started_at: string;
  resolved_at?: string;
  updates?: StatusUpdate[];
};

export type StatusUpdate = {
  state: PublicStatusState;
  message: string;
  created_at: string;
};

export type StatusLinks = {
  sample_report?: string;
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
  failed_checks?: string[];
  recommended_action?: string;
  guideline_pack_id?: string;
  artifact_paths?: Record<string, string>;
  unresolved_items?: UnresolvedFood[];
  excluded_unresolved_items?: ExcludedUnresolvedFood[];
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
  quantity?: number;
  unit?: string;
  quantity_text?: string;
  unresolved_reason?: string;
  [key: string]: unknown;
};

export type ExcludedUnresolvedFood = {
  day: number;
  meal: string;
  food: string;
  quantity: number;
  unit: string;
  deterministic_grams: number;
  unresolved_reason?: string;
  exclusion_reason?: string;
  policy_id?: string;
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

export type RecommendationStatus = "available" | "unavailable" | string;

export type RecommendationFoodItem = {
  food: string;
  quantity?: number;
  unit?: string;
  quantity_text?: string;
  preparation?: string;
  brand?: string;
  resolution_status?: string;
  unresolved_reason?: string;
  [key: string]: unknown;
};

export type RecommendationChange = {
  operation: string;
  day?: number;
  meal?: string;
  from?: RecommendationFoodItem;
  to?: RecommendationFoodItem;
  prep_note?: string;
  reason: string;
  addresses_checks: string[];
};

export type RecommendationDocument = {
  schema_version: string;
  status: RecommendationStatus;
  reason: string;
  source_decision: string;
  source_plan_id: string;
  blocking_checks?: string[];
  changes?: RecommendationChange[];
  modified_plan?: MealPlan;
  projected_decision?: Decision;
};

export type ReportArtifacts = {
  apiBase?: string;
  base: string;
  decision: Decision;
  report: Report;
  totals: DailyTotal[];
  resolved: ResolvedFood[];
  unresolved: UnresolvedFood[];
  excludedUnresolved: ExcludedUnresolvedFood[];
  manifest: Manifest;
  pack?: unknown;
  citations: Citations;
  artifactItems?: ArtifactItem[] | null;
  recommendation?: RecommendationDocument | null;
  normalizationReview?: NormalizedPlanReviewArtifact | null;
  normalizationEvents?: NormalizationEvent[] | null;
  localModelExtraction?: LocalModelExtractionArtifact | null;
  reviewActions?: ReviewActionArtifact[] | null;
};

export type RunEvent = {
  type: string;
  message: string;
};

export type RecoveryAction = {
  label: string;
  href: string;
};

export type RecoveryNotice = {
  title: string;
  message: string;
  tone: "info" | "pass" | "warn" | "block";
  steps?: string[];
  action?: RecoveryAction;
};

export type RunProgress = {
  state: string;
  label: string;
  message: string;
  last_event?: string;
  recovery?: RecoveryNotice | null;
  updated_at: string;
  finished_at?: string;
};

export type LiveState = {
  runID: string;
  status: RunStatus;
  message: string;
  events: RunEvent[];
  artifactItems: ArtifactItem[];
  progress?: RunProgress | null;
};

export type NormalizedPlanReviewTrustSignals = {
  source_item_count: number;
  normalized_row_count: number;
  unresolved_item_count: number;
  repair_count: number;
  failed_chunk_count: number;
};

export type NormalizedPlanReviewRow = {
  day: number;
  meal_code: string;
  meal_label?: string;
  source_item_id: number;
  source_text: string;
  source_parse_status?: string;
  normalized_food?: string;
  resolved: boolean;
  quantity?: number;
  unit?: string;
  quantity_text?: string;
  unresolved_reason?: string;
};

export type NormalizedPlanCorrectionPayload = {
  row_index: number;
  source_item_id: number;
  food: string;
  quantity?: number;
  unit?: string;
  quantity_text?: string;
  resolution_status?: "unresolved";
  unresolved_reason?: string;
  reason?: string;
};

export type NormalizedPlanReviewArtifact = {
  schema_version: string;
  run_id: string;
  created_at: string;
  status: string;
  requires_confirmation: boolean;
  trust_signals: NormalizedPlanReviewTrustSignals;
  normalized_plan: MealPlan;
  rows: NormalizedPlanReviewRow[];
};

export type NormalizedPlanReviewState = {
  status: "idle" | "loading" | "ready" | "submitting" | "failed";
  message: string;
  artifact: NormalizedPlanReviewArtifact | null;
};

export type NormalizationEvent = {
  type: string;
  message: string;
  created_at: string;
};

export type ReviewActionArtifact = {
  schema_version: string;
  run_id: string;
  action: string;
  reason?: string;
  created_at: string;
  row_index?: number;
  source_item_id?: number;
  source_text?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
};

export type LocalModelExtractionArtifact = {
  schema_version: string;
  created_at: string;
  plan_id: string;
  chunk_count: number;
  source_item_count: number;
  stage_timings: LocalModelExtractionStageTimings;
  repeat_run_instability?: {
    measured: boolean;
    reason?: string;
  };
  failure_stage?: string;
  error?: string;
  chunks: LocalModelChunkArtifact[];
};

export type LocalModelExtractionStageTimings = {
  chunking_ms?: number;
  expansion_ms?: number;
  completeness_check_ms?: number;
  total_ms?: number;
};

export type LocalModelChunkArtifact = {
  index: number;
  day: number;
  meal_code: string;
  meal_label?: string;
  meal_text: string;
  source_item_ids?: number[];
  source_items: LocalModelChunkSourceItemArtifact[];
  decoded_rows?: LocalModelChunkDecodedRowArtifact[];
  reconciliation: LocalModelChunkReconciliationArtifact;
  stage_timings?: Record<string, number>;
  failure_stage?: string;
  error?: string;
};

export type LocalModelChunkSourceItemArtifact = {
  id: number;
  day: number;
  meal_code: string;
  text: string;
  parse_status: string;
};

export type LocalModelChunkDecodedRowArtifact = {
  source_item_id: number;
  day: number;
  meal_code: string;
  food: string;
  resolved: boolean;
  quantity?: number;
  unit?: string;
  quantity_text?: string;
  unresolved_reason?: string;
};

export type LocalModelChunkReconciliationArtifact = {
  decoded_source_item_ids?: number[];
  repair_count: number;
  repairs?: LocalLlamaNormalizationRepair[];
};

export type LocalLlamaNormalizationRepair = {
  source_item_id: number;
  field: string;
  from?: string;
  to?: string;
  reason: string;
};

export type QualificationState = {
  status: "idle" | "checking" | "completed" | "failed";
  message: string;
  result?: MealPlanQualificationResult | null;
};

export type RunDocument = {
  run: {
    status: RunStatus;
    error?: string | null;
    summary?: string | null;
  };
  progress?: RunProgress | null;
};

export type ArtifactListResponse = {
  artifacts?: ArtifactItem[];
};

export type CreateRunResponse = {
  run_id: string;
  status: RunStatus;
};

export type QualifyMealPlanPayload = {
  text: string;
  settings: Settings;
  provider?: ProviderConfig;
};

export type MealPlanQualificationResult = {
  schema_version: string;
  status: QualificationStatus;
  reason: string;
  missing_fields?: string[];
  normalized_plan?: MealPlan;
  provider_used: boolean;
  canonicalized?: boolean;
};

export type QualifyMealPlanResponse = {
  qualification: MealPlanQualificationResult;
};

export type Settings = {
  nutrition_targets: NutritionTargets;
  verification_constraints: VerificationConstraints;
};

export type NutritionTargets = {
  calorie_target_kcal: number;
  protein_target_g: number;
};

export type VerificationConstraints = {
  days?: number;
  meals_per_day?: number;
  allergies: string[];
  excluded_foods: string[];
  max_sodium_mg_per_day: number;
  max_added_sugar_g_per_meal: number;
  max_saturated_fat_pct_calories: number;
  calorie_tolerance_pct: number;
  requires_prep_safety_notes: boolean;
  unresolved_policy?: UnresolvedPolicy;
};

export type UnresolvedPolicy = {
  de_minimis_enabled?: boolean;
  max_item_grams?: number;
  max_total_grams_per_day?: number;
  max_items_per_day?: number;
};

export type VerificationConstraintsDraft = Omit<VerificationConstraints, "allergies" | "excluded_foods"> & {
  allergies: string;
  excluded_foods: string;
};

export type SettingsDraft = {
  nutrition_targets: NutritionTargets;
  verification_constraints: VerificationConstraintsDraft;
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
      input_mode: "profile_generation";
      settings: Settings;
      provider: ProviderConfig;
      repair_json: boolean;
    }
  | {
      input_mode: "prompt_generation";
      settings: Settings;
      provider: ProviderConfig;
      repair_json: boolean;
      generation_prompt: string;
    }
  | {
      input_mode: "local_model";
      settings: Settings;
      candidate_text: string;
    };

export type FieldProps = {
  label: string;
  children: ReactNode;
};
