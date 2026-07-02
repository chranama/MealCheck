import type {
  CitationSource,
  DecisionCheck,
  ExcludedUnresolvedFood,
  NormalizedPlanReviewRow,
  ReportArtifacts,
  ResolvedFood,
  SourceClaim,
  UnresolvedFood,
} from "../types";
import { checkLabel, isMealPlanCheckID, readableID, reasonLabel, round, sourceClaimLabel, valueText } from "./format";

export type CheckSourceInspectionRow = Record<string, unknown> & {
  check_id: string;
  check: string;
  status: string;
  affected: string;
  source_refs: string;
  sources: string;
  missing_source_refs: string;
  message: string;
};

export type FoodSourceInspectionRow = Record<string, unknown> & {
  day: number;
  meal: string;
  source_item_id: number | string;
  source_text: string;
  food: string;
  quantity: string;
  status: string;
  reason: string;
  recovery_action: string;
  grams: number | string;
  artifact: string;
};

export type CitationInspectionRow = Record<string, unknown> & {
  source_id: string;
  title: string;
  publisher: string;
  url: string;
  claim_count: number;
  claims: SourceClaim[];
  claim_labels: string;
  referenced_by_checks: string;
};

export type MissingSourceInspectionRow = Record<string, unknown> & {
  source_ref: string;
  referenced_by_checks: string;
};

export type SourceInspectionSummary = {
  citationSourceCount: number;
  checkSourceRefCount: number;
  foodTraceCount: number;
  missingSourceRefCount: number;
};

export type SourceInspection = {
  summary: SourceInspectionSummary;
  checkRows: CheckSourceInspectionRow[];
  foodRows: FoodSourceInspectionRow[];
  citationRows: CitationInspectionRow[];
  missingSourceRows: MissingSourceInspectionRow[];
};

export function buildSourceInspection(artifacts: ReportArtifacts): SourceInspection {
  const sources = artifacts.citations?.sources || [];
  const sourceByID = new Map(sources.map((source) => [source.source_id, source]));
  const checks = (artifacts.decision.checks || []).filter((check) => isMealPlanCheckID(check.check_id));
  const referencedByCheck = new Map<string, Set<string>>();

  const checkRows = checks.map((check) => checkSourceRow(check, sourceByID, referencedByCheck));
  const citationRows = sources.map((source) => citationRow(source, referencedByCheck));
  const missingSourceRows = [...referencedByCheck.entries()]
    .filter(([sourceID]) => !sourceByID.has(sourceID))
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([sourceID, checkNames]) => ({
      source_ref: sourceID,
      referenced_by_checks: [...checkNames].sort().join(", "),
    }));
  const foodRows = buildFoodRows(artifacts);

  return {
    summary: {
      citationSourceCount: citationRows.length,
      checkSourceRefCount: checkRows.reduce((sum, row) => sum + csvCount(row.source_refs), 0),
      foodTraceCount: foodRows.length,
      missingSourceRefCount: missingSourceRows.length,
    },
    checkRows,
    foodRows,
    citationRows,
    missingSourceRows,
  };
}

function checkSourceRow(
  check: DecisionCheck,
  sourceByID: Map<string, CitationSource>,
  referencedByCheck: Map<string, Set<string>>,
): CheckSourceInspectionRow {
  const refs = unique(check.source_refs || []);
  const checkName = checkLabel(check.check_id);
  for (const sourceID of refs) {
    const checks = referencedByCheck.get(sourceID) || new Set<string>();
    checks.add(checkName);
    referencedByCheck.set(sourceID, checks);
  }
  const missingRefs = refs.filter((sourceID) => !sourceByID.has(sourceID));
  return {
    check_id: check.check_id,
    check: checkName,
    status: readableID(check.status),
    affected: affectedLabel(check),
    source_refs: refs.join(", ") || "-",
    sources: refs.map((sourceID) => sourceByID.get(sourceID)?.title || readableID(sourceID)).join(", ") || "-",
    missing_source_refs: missingRefs.join(", ") || "-",
    message: check.message,
  };
}

function citationRow(source: CitationSource, referencedByCheck: Map<string, Set<string>>): CitationInspectionRow {
  const claims = source.claims_used || [];
  return {
    source_id: source.source_id,
    title: source.title,
    publisher: source.publisher || source.source_id,
    url: source.url,
    claim_count: claims.length,
    claims,
    claim_labels: claims.map((claim) => sourceClaimLabel(claim.claim_id)).join(", ") || "-",
    referenced_by_checks: [...(referencedByCheck.get(source.source_id) || [])].sort().join(", ") || "-",
  };
}

function buildFoodRows(artifacts: ReportArtifacts): FoodSourceInspectionRow[] {
  const resolved = artifacts.resolved || [];
  const unresolved = artifacts.unresolved || [];
  const excluded = artifacts.excludedUnresolved || [];
  const reviewRows = artifacts.normalizationReview?.rows || [];
  const usedResolved = new Set<number>();
  const usedUnresolved = new Set<number>();
  const usedExcluded = new Set<number>();
  const rows: FoodSourceInspectionRow[] = [];

  for (const row of reviewRows) {
    const excludedMatch = findExcludedForReviewRow(row, excluded, usedExcluded);
    if (excludedMatch.index >= 0) usedExcluded.add(excludedMatch.index);
    const unresolvedMatch = findUnresolvedForReviewRow(row, unresolved, usedUnresolved);
    if (unresolvedMatch.index >= 0) usedUnresolved.add(unresolvedMatch.index);
    const resolvedMatch = findResolvedForReviewRow(row, resolved, usedResolved);
    if (resolvedMatch.index >= 0) usedResolved.add(resolvedMatch.index);
    rows.push(foodRowFromReview(row, resolvedMatch.item, unresolvedMatch.item, excludedMatch.item));
  }

  for (const [index, item] of resolved.entries()) {
    if (usedResolved.has(index)) continue;
    rows.push(foodRowFromResolved(item));
  }
  for (const [index, item] of unresolved.entries()) {
    if (usedUnresolved.has(index)) continue;
    rows.push(foodRowFromUnresolved(item));
  }
  for (const [index, item] of excluded.entries()) {
    if (usedExcluded.has(index)) continue;
    rows.push(foodRowFromExcluded(item));
  }

  return rows.sort(compareFoodRows);
}

function foodRowFromReview(
  row: NormalizedPlanReviewRow,
  resolved?: ResolvedFood,
  unresolved?: UnresolvedFood,
  excluded?: ExcludedUnresolvedFood,
): FoodSourceInspectionRow {
  const unresolvedReason = excluded?.unresolved_reason || unresolved?.unresolved_reason || row.unresolved_reason;
  const status = excluded ? "Excluded From Totals" : unresolvedReason || !row.resolved ? "Unresolved" : "Resolved";
  const reason = excluded
    ? [reasonLabel(unresolvedReason || "unresolved"), excluded.exclusion_reason ? readableID(excluded.exclusion_reason) : ""].filter(Boolean).join("; ")
    : unresolvedReason
      ? reasonLabel(unresolvedReason)
      : "-";
  return {
    day: row.day,
    meal: readableID(row.meal_label || row.meal_code),
    source_item_id: row.source_item_id,
    source_text: row.source_text,
    food: row.normalized_food || unresolved?.food || excluded?.food || resolved?.food || "-",
    quantity: reviewQuantityText(row) || itemQuantityText(unresolved || excluded) || "-",
    status,
    reason,
    recovery_action: unresolvedReason ? recoveryActionForUnresolvedReason(unresolvedReason) : "-",
    grams: resolved ? round(resolved.grams) : excluded ? round(excluded.deterministic_grams) : "-",
    artifact: "review/normalized-plan-review.json",
  };
}

function foodRowFromResolved(item: ResolvedFood): FoodSourceInspectionRow {
  return {
    day: item.day,
    meal: readableID(item.meal),
    source_item_id: "-",
    source_text: "-",
    food: item.food,
    quantity: "-",
    status: "Resolved",
    reason: "-",
    recovery_action: "-",
    grams: round(item.grams),
    artifact: "resolved-foods.json",
  };
}

function foodRowFromUnresolved(item: UnresolvedFood): FoodSourceInspectionRow {
  const reason = item.unresolved_reason || "unresolved";
  return {
    day: item.day,
    meal: readableID(item.meal),
    source_item_id: "-",
    source_text: "-",
    food: item.food,
    quantity: itemQuantityText(item) || "-",
    status: "Unresolved",
    reason: reasonLabel(reason),
    recovery_action: recoveryActionForUnresolvedReason(reason),
    grams: "-",
    artifact: "unresolved-foods.json",
  };
}

function foodRowFromExcluded(item: ExcludedUnresolvedFood): FoodSourceInspectionRow {
  const reason = item.unresolved_reason || "unresolved";
  return {
    day: item.day,
    meal: readableID(item.meal),
    source_item_id: "-",
    source_text: "-",
    food: item.food,
    quantity: itemQuantityText(item) || "-",
    status: "Excluded From Totals",
    reason: [reasonLabel(reason), item.exclusion_reason ? readableID(item.exclusion_reason) : ""].filter(Boolean).join("; "),
    recovery_action: recoveryActionForUnresolvedReason(reason),
    grams: round(item.deterministic_grams),
    artifact: "excluded-unresolved-foods.json",
  };
}

function findResolvedForReviewRow(
  row: NormalizedPlanReviewRow,
  items: ResolvedFood[],
  used: Set<number>,
): { index: number; item?: ResolvedFood } {
  if (!row.resolved) return { index: -1 };
  return findIndexed(items, used, (item) => sameFoodLocation(row, item) && lower(row.normalized_food) === lower(item.food));
}

function findUnresolvedForReviewRow(
  row: NormalizedPlanReviewRow,
  items: UnresolvedFood[],
  used: Set<number>,
): { index: number; item?: UnresolvedFood } {
  return findIndexed(items, used, (item) => {
    if (!sameFoodLocation(row, item)) return false;
    if (lower(row.normalized_food) !== lower(item.food)) return false;
    if (row.unresolved_reason && item.unresolved_reason && row.unresolved_reason !== item.unresolved_reason) return false;
    return true;
  });
}

function findExcludedForReviewRow(
  row: NormalizedPlanReviewRow,
  items: ExcludedUnresolvedFood[],
  used: Set<number>,
): { index: number; item?: ExcludedUnresolvedFood } {
  return findIndexed(items, used, (item) => {
    if (!sameFoodLocation(row, item)) return false;
    if (lower(row.normalized_food) !== lower(item.food)) return false;
    if (row.unresolved_reason && item.unresolved_reason && row.unresolved_reason !== item.unresolved_reason) return false;
    return true;
  });
}

function findIndexed<T>(items: T[], used: Set<number>, matches: (item: T) => boolean): { index: number; item?: T } {
  const index = items.findIndex((item, candidateIndex) => !used.has(candidateIndex) && matches(item));
  return index >= 0 ? { index, item: items[index] } : { index: -1 };
}

function sameFoodLocation(row: NormalizedPlanReviewRow, item: { day: number; meal: string }): boolean {
  return row.day === item.day && mealMatches(row, item.meal);
}

function mealMatches(row: NormalizedPlanReviewRow, meal: string): boolean {
  const target = lower(meal);
  return [row.meal_label, row.meal_code, readableID(row.meal_code)].some((value) => lower(value) === target);
}

function affectedLabel(check: DecisionCheck): string {
  return [
    ...(check.affected_days || []).map((day) => `Day ${day}`),
    ...(check.affected_meals || []).map((meal) => readableID(meal)),
  ].join(", ") || "-";
}

function reviewQuantityText(row: NormalizedPlanReviewRow): string {
  if (row.quantity_text) return row.quantity_text;
  if (row.quantity !== undefined && row.quantity !== null) {
    return [valueText(row.quantity), row.unit].filter(Boolean).join(" ");
  }
  return "";
}

function itemQuantityText(item?: { quantity?: number; unit?: string; quantity_text?: string }): string {
  if (!item) return "";
  if (item.quantity_text) return item.quantity_text;
  if (item.quantity !== undefined && item.quantity !== null) {
    return [valueText(item.quantity), item.unit].filter(Boolean).join(" ");
  }
  return "";
}

export function recoveryActionForUnresolvedReason(reason: unknown): string {
  const id = String(reason || "");
  if (id.startsWith("missing_conversion:")) return "Use a supported measured unit or add a reviewed conversion.";
  const actions: Record<string, string> = {
    ambiguous_food: "Choose a more specific food.",
    branded_food_unavailable: "Use a supported generic ingredient or an exact reviewed catalog item.",
    composed_food_needs_decomposition: "Break this mixed dish into ingredients.",
    model_normalization_failed: "Rewrite this item with a clear food, quantity, and unit, then rerun MealCheck.",
    non_food_text: "Remove non-food text from the meal plan.",
    preparation_unclear: "Specify preparation details such as baked, boiled, fried, or added fat.",
    restaurant_or_branded_food: "Use supported generic ingredients or an exact reviewed catalog item.",
    unknown_food: "Use a supported catalog food or request catalog expansion.",
    unsupported_unit: "Use grams, ounces, cups, tablespoons, teaspoons, slices, or servings.",
    vague_quantity: "Add a measured quantity and unit.",
  };
  return actions[id] || "Clarify this item and rerun MealCheck.";
}

function compareFoodRows(a: FoodSourceInspectionRow, b: FoodSourceInspectionRow): number {
  return a.day - b.day
    || mealOrder(a.meal) - mealOrder(b.meal)
    || Number(a.source_item_id || 9999) - Number(b.source_item_id || 9999)
    || a.food.localeCompare(b.food);
}

function mealOrder(meal: string): number {
  const id = lower(meal);
  const order: Record<string, number> = { breakfast: 1, b: 1, lunch: 2, l: 2, dinner: 3, d: 3, snack: 4, s: 4 };
  return order[id] || 99;
}

function csvCount(value: string): number {
  if (!value || value === "-") return 0;
  return value.split(",").map((entry) => entry.trim()).filter(Boolean).length;
}

function lower(value: unknown): string {
  return String(value || "").trim().toLowerCase();
}

function unique(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}
