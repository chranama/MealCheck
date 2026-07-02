import type {
  ArtifactItem,
  ArtifactListResponse,
  CreateRunResponse,
  HealthResponse,
  LocalModelHealth,
  MealPlan,
  MealPlanQualificationResult,
  NormalizedPlanReviewArtifact,
  NormalizedPlanReviewRow,
  NormalizedPlanReviewTrustSignals,
  PublicStatusResponse,
  QualifyMealPlanResponse,
  RunDocument,
  RunProgress,
  RuntimeConfig,
} from "../types";

export class ContractParseError extends Error {
  path: string;

  constructor(path: string, message: string) {
    super(`${path}: ${message}`);
    this.name = "ContractParseError";
    this.path = path;
  }
}

export function parseRuntimeConfig(value: unknown): RuntimeConfig {
  const root = requireObject(value, "runtime config");
  const api = optionalObject(root.api, "runtime config.api");
  const features = optionalFeatureFlags(root.features, "runtime config.features");
  return {
    ...(api ? { api: { base_url: optionalString(api.base_url, "runtime config.api.base_url") } } : {}),
    ...(features ? { features } : {}),
  };
}

export function parseHealthResponse(value: unknown): HealthResponse {
  const root = requireObject(value, "health response");
  return {
    status: requireString(root.status, "health response.status"),
    store: requireString(root.store, "health response.store"),
    access_mode: optionalString(root.access_mode, "health response.access_mode"),
    hosted_mode: optionalString(root.hosted_mode, "health response.hosted_mode"),
    queued_runs: optionalNumber(root.queued_runs, "health response.queued_runs"),
    running_runs: optionalNumber(root.running_runs, "health response.running_runs"),
    queue_size: optionalNumber(root.queue_size, "health response.queue_size"),
    active_run_limit: optionalNumber(root.active_run_limit, "health response.active_run_limit"),
    retention_days: optionalNumber(root.retention_days, "health response.retention_days"),
    public_openai_compatible: optionalBoolean(root.public_openai_compatible, "health response.public_openai_compatible"),
    max_candidate_text_chars: optionalNumber(root.max_candidate_text_chars, "health response.max_candidate_text_chars"),
    max_generation_prompt_chars: optionalNumber(root.max_generation_prompt_chars, "health response.max_generation_prompt_chars"),
    local_model: parseOptionalLocalModelHealth(root.local_model, "health response.local_model"),
    policy: optionalUnknownRecord(root.policy, "health response.policy"),
  };
}

export function parseCreateRunResponse(value: unknown): CreateRunResponse {
  const root = requireObject(value, "create run response");
  return {
    run_id: requireString(root.run_id, "create run response.run_id"),
    status: requireString(root.status, "create run response.status"),
  };
}

export function parseRunDocument(value: unknown): RunDocument {
  const root = requireObject(value, "run document");
  const run = requireObject(root.run, "run document.run");
  return {
    run: {
      status: requireString(run.status, "run document.run.status"),
      error: optionalNullableString(run.error, "run document.run.error"),
      summary: optionalNullableString(run.summary, "run document.run.summary"),
    },
    progress: parseOptionalRunProgress(root.progress, "run document.progress"),
  };
}

export function parseArtifactListResponse(value: unknown): ArtifactListResponse {
  const root = requireObject(value, "artifact list response");
  return {
    artifacts: optionalArray(root.artifacts, "artifact list response.artifacts", parseArtifactItem),
  };
}

export function parseNormalizedPlanReviewArtifact(value: unknown): NormalizedPlanReviewArtifact {
  const root = requireObject(value, "normalized plan review");
  return {
    schema_version: requireString(root.schema_version, "normalized plan review.schema_version"),
    run_id: requireString(root.run_id, "normalized plan review.run_id"),
    created_at: requireString(root.created_at, "normalized plan review.created_at"),
    status: requireString(root.status, "normalized plan review.status"),
    requires_confirmation: requireBoolean(root.requires_confirmation, "normalized plan review.requires_confirmation"),
    trust_signals: parseReviewTrustSignals(root.trust_signals, "normalized plan review.trust_signals"),
    normalized_plan: parseMealPlan(root.normalized_plan, "normalized plan review.normalized_plan"),
    rows: requireArray(root.rows, "normalized plan review.rows", parseReviewRow),
  };
}

export function parseQualifyMealPlanResponse(value: unknown): QualifyMealPlanResponse {
  const root = requireObject(value, "qualify response");
  return {
    qualification: parseMealPlanQualificationResult(root.qualification, "qualify response.qualification"),
  };
}

export function parseMealPlanQualificationResult(value: unknown, path = "qualification"): MealPlanQualificationResult {
  const root = requireObject(value, path);
  return {
    schema_version: requireString(root.schema_version, `${path}.schema_version`),
    status: requireString(root.status, `${path}.status`),
    reason: requireString(root.reason, `${path}.reason`),
    missing_fields: optionalArray(root.missing_fields, `${path}.missing_fields`, requireString),
    normalized_plan: root.normalized_plan === undefined ? undefined : parseMealPlan(root.normalized_plan, `${path}.normalized_plan`),
    provider_used: requireBoolean(root.provider_used, `${path}.provider_used`),
    canonicalized: optionalBoolean(root.canonicalized, `${path}.canonicalized`),
  };
}

export function parsePublicStatusResponse(value: unknown): PublicStatusResponse {
  const root = requireObject(value, "public status response");
  const overall = requireObject(root.overall, "public status response.overall");
  return {
    schema_version: requireString(root.schema_version, "public status response.schema_version"),
    generated_at: requireString(root.generated_at, "public status response.generated_at"),
    overall: {
      state: requireString(overall.state, "public status response.overall.state"),
      message: requireString(overall.message, "public status response.overall.message"),
    },
    components: requireArray(root.components, "public status response.components", (component, path) => {
      const row = requireObject(component, path);
      return {
        id: requireString(row.id, `${path}.id`),
        name: requireString(row.name, `${path}.name`),
        state: requireString(row.state, `${path}.state`),
        message: optionalString(row.message, `${path}.message`),
      };
    }),
    recent_incidents: optionalArray(root.recent_incidents, "public status response.recent_incidents", (incident, path) => {
      const row = requireObject(incident, path);
      return {
        id: requireString(row.id, `${path}.id`),
        title: requireString(row.title, `${path}.title`),
        state: requireString(row.state, `${path}.state`),
        started_at: requireString(row.started_at, `${path}.started_at`),
        resolved_at: optionalString(row.resolved_at, `${path}.resolved_at`),
        updates: optionalArray(row.updates, `${path}.updates`, (update, updatePath) => {
          const updateRow = requireObject(update, updatePath);
          return {
            state: requireString(updateRow.state, `${updatePath}.state`),
            message: requireString(updateRow.message, `${updatePath}.message`),
            created_at: requireString(updateRow.created_at, `${updatePath}.created_at`),
          };
        }),
      };
    }) ?? [],
    links: root.links === undefined ? undefined : {
      sample_report: optionalString(requireObject(root.links, "public status response.links").sample_report, "public status response.links.sample_report"),
    },
  };
}

function parseOptionalLocalModelHealth(value: unknown, path: string): LocalModelHealth | undefined {
  if (value === undefined || value === null) return undefined;
  const root = requireObject(value, path);
  return {
    enabled: optionalBoolean(root.enabled, `${path}.enabled`),
    ready: optionalBoolean(root.ready, `${path}.ready`),
    model: optionalString(root.model, `${path}.model`),
    max_input_chars: optionalNumber(root.max_input_chars, `${path}.max_input_chars`),
    max_source_items: optionalNumber(root.max_source_items, `${path}.max_source_items`),
    max_output_tokens: optionalNumber(root.max_output_tokens, `${path}.max_output_tokens`),
    timeout_sec: optionalNumber(root.timeout_sec, `${path}.timeout_sec`),
    supported_days: optionalNumber(root.supported_days, `${path}.supported_days`),
    supported_meals_per_day: optionalNumber(root.supported_meals_per_day, `${path}.supported_meals_per_day`),
    error: optionalString(root.error, `${path}.error`),
  };
}

function parseOptionalRunProgress(value: unknown, path: string): RunProgress | null | undefined {
  if (value === undefined) return undefined;
  if (value === null) return null;
  const root = requireObject(value, path);
  return {
    state: requireString(root.state, `${path}.state`),
    label: requireString(root.label, `${path}.label`),
    message: requireString(root.message, `${path}.message`),
    last_event: optionalString(root.last_event, `${path}.last_event`),
    recovery: root.recovery === undefined ? undefined : root.recovery as RunProgress["recovery"],
    updated_at: requireString(root.updated_at, `${path}.updated_at`),
    finished_at: optionalString(root.finished_at, `${path}.finished_at`),
  };
}

function parseArtifactItem(value: unknown, path: string): ArtifactItem {
  const root = requireObject(value, path);
  return {
    path: requireString(root.path, `${path}.path`),
    type: requireString(root.type, `${path}.type`),
    url: requireString(root.url, `${path}.url`),
  };
}

function parseReviewTrustSignals(value: unknown, path: string): NormalizedPlanReviewTrustSignals {
  const root = requireObject(value, path);
  return {
    source_item_count: requireNumber(root.source_item_count, `${path}.source_item_count`),
    normalized_row_count: requireNumber(root.normalized_row_count, `${path}.normalized_row_count`),
    unresolved_item_count: requireNumber(root.unresolved_item_count, `${path}.unresolved_item_count`),
    repair_count: requireNumber(root.repair_count, `${path}.repair_count`),
    failed_chunk_count: requireNumber(root.failed_chunk_count, `${path}.failed_chunk_count`),
  };
}

function parseReviewRow(value: unknown, path: string): NormalizedPlanReviewRow {
  const root = requireObject(value, path);
  return {
    day: requireNumber(root.day, `${path}.day`),
    meal_code: requireString(root.meal_code, `${path}.meal_code`),
    meal_label: optionalString(root.meal_label, `${path}.meal_label`),
    source_item_id: requireNumber(root.source_item_id, `${path}.source_item_id`),
    source_text: requireString(root.source_text, `${path}.source_text`),
    source_parse_status: optionalString(root.source_parse_status, `${path}.source_parse_status`),
    normalized_food: optionalString(root.normalized_food, `${path}.normalized_food`),
    resolved: requireBoolean(root.resolved, `${path}.resolved`),
    quantity: optionalNumber(root.quantity, `${path}.quantity`),
    unit: optionalString(root.unit, `${path}.unit`),
    quantity_text: optionalString(root.quantity_text, `${path}.quantity_text`),
    unresolved_reason: optionalString(root.unresolved_reason, `${path}.unresolved_reason`),
  };
}

function parseMealPlan(value: unknown, path: string): MealPlan {
  const root = requireObject(value, path);
  return {
    schema_version: requireString(root.schema_version, `${path}.schema_version`),
    plan_id: requireString(root.plan_id, `${path}.plan_id`),
    description: requireString(root.description, `${path}.description`),
    days: requireArray(root.days, `${path}.days`, (day, dayPath) => {
      const dayRoot = requireObject(day, dayPath);
      return {
        day: requireNumber(dayRoot.day, `${dayPath}.day`),
        meals: requireArray(dayRoot.meals, `${dayPath}.meals`, (meal, mealPath) => {
          const mealRoot = requireObject(meal, mealPath);
          return {
            name: requireString(mealRoot.name, `${mealPath}.name`),
            items: requireArray(mealRoot.items, `${mealPath}.items`, (item, itemPath) => {
              const itemRoot = requireObject(item, itemPath);
              requireString(itemRoot.food, `${itemPath}.food`);
              return itemRoot as MealPlan["days"][number]["meals"][number]["items"][number];
            }),
          };
        }),
      };
    }),
    shopping_list: optionalArray(root.shopping_list, `${path}.shopping_list`, (item, itemPath) => {
      const itemRoot = requireObject(item, itemPath);
      requireString(itemRoot.food, `${itemPath}.food`);
      return itemRoot as MealPlan["shopping_list"][number];
    }) ?? [],
    prep_notes: optionalArray(root.prep_notes, `${path}.prep_notes`, requireString) ?? [],
  };
}

function optionalFeatureFlags(value: unknown, path: string): Record<string, boolean | string | number | null> | undefined {
  if (value === undefined || value === null) return undefined;
  const root = requireObject(value, path);
  const flags: Record<string, boolean | string | number | null> = {};
  Object.entries(root).forEach(([key, entry]) => {
    if (entry === null || typeof entry === "boolean" || typeof entry === "string" || typeof entry === "number") {
      flags[key] = entry;
    }
  });
  return flags;
}

function optionalUnknownRecord(value: unknown, path: string): Record<string, unknown> | undefined {
  if (value === undefined || value === null) return undefined;
  return requireObject(value, path);
}

function requireObject(value: unknown, path: string): Record<string, unknown> {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  throw new ContractParseError(path, "expected object");
}

function optionalObject(value: unknown, path: string): Record<string, unknown> | undefined {
  if (value === undefined || value === null) return undefined;
  return requireObject(value, path);
}

function requireArray<T>(value: unknown, path: string, parser: (entry: unknown, path: string) => T): T[] {
  if (!Array.isArray(value)) {
    throw new ContractParseError(path, "expected array");
  }
  return value.map((entry, index) => parser(entry, `${path}[${index}]`));
}

function optionalArray<T>(value: unknown, path: string, parser: (entry: unknown, path: string) => T): T[] | undefined {
  if (value === undefined || value === null) return undefined;
  return requireArray(value, path, parser);
}

function requireString(value: unknown, path: string): string {
  if (typeof value === "string") return value;
  throw new ContractParseError(path, "expected string");
}

function optionalString(value: unknown, path: string): string | undefined {
  if (value === undefined || value === null) return undefined;
  return requireString(value, path);
}

function optionalNullableString(value: unknown, path: string): string | null | undefined {
  if (value === undefined) return undefined;
  if (value === null) return null;
  return requireString(value, path);
}

function requireNumber(value: unknown, path: string): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  throw new ContractParseError(path, "expected finite number");
}

function optionalNumber(value: unknown, path: string): number | undefined {
  if (value === undefined || value === null) return undefined;
  return requireNumber(value, path);
}

function requireBoolean(value: unknown, path: string): boolean {
  if (typeof value === "boolean") return value;
  throw new ContractParseError(path, "expected boolean");
}

function optionalBoolean(value: unknown, path: string): boolean | undefined {
  if (value === undefined || value === null) return undefined;
  return requireBoolean(value, path);
}
