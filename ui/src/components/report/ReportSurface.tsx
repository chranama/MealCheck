import type { ReactNode } from "react";
import type {
  NormalizedPlanReviewRow,
  RecommendationChange,
  RecommendationFoodItem,
  ReviewActionArtifact,
  ReportArtifacts,
  ReportTab,
  UnresolvedFood,
} from "../../types";
import { TABS } from "../../constants";
import {
  checkLabel,
  guidelineLabel,
  isMealPlanCheckID,
  readableID,
  reasonLabel,
  round,
  sourceChip,
  sourceClaimLabel,
  valueText,
} from "../../lib/format";
import { buildSourceInspection, recoveryActionForUnresolvedReason } from "../../lib/source_inspection";
import { Metric } from "../common/FormControls";

export function ReportSurface({
  activeTab,
  setActiveTab,
  artifacts,
}: {
  activeTab: ReportTab;
  setActiveTab: (tab: ReportTab) => void;
  artifacts: ReportArtifacts;
}) {
  return (
    <>
      <nav className="tabbar" id="report-tabs" aria-label="Report views" role="tablist">
        {TABS.map((tab) => (
          <button
            aria-selected={activeTab === tab}
            className={`tab-button${activeTab === tab ? " is-active" : ""}`}
            data-tab={tab}
            key={tab}
            onClick={() => setActiveTab(tab)}
            role="tab"
            type="button"
          >
            {readableID(tab)}
          </button>
        ))}
      </nav>
      <section className="panel" id="tab-panel" tabIndex={-1}>
        {activeTab === "checks" ? <ChecksPanel artifacts={artifacts} /> : null}
        {activeTab === "nutrition" ? <NutritionPanel artifacts={artifacts} /> : null}
        {activeTab === "foods" ? <FoodsPanel artifacts={artifacts} /> : null}
        {activeTab === "recommendation" ? <RecommendationPanel artifacts={artifacts} /> : null}
        {activeTab === "normalization" ? <NormalizationPanel artifacts={artifacts} /> : null}
        {activeTab === "sources" ? <SourcesPanel artifacts={artifacts} /> : null}
        {activeTab === "report" ? <ReportDownloadPanel artifacts={artifacts} /> : null}
      </section>
    </>
  );
}

function ChecksPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const { decision, citations } = artifacts;
  const mealPlanChecks = (decision.checks || []).filter((check) => isMealPlanCheckID(check.check_id));
  return (
    <>
      <h2>Meal Plan Checks</h2>
      <div className="check-list">
        {mealPlanChecks.map((check) => {
          const sourceChips = (check.source_refs || []).map((sourceID) => sourceChip(citations, sourceID));
          const affected = [
            ...(check.affected_days || []).map((day) => `Day ${day}`),
            ...(check.affected_meals || []).map((meal) => readableID(meal)),
          ];
          return (
            <article className="check-card" key={check.check_id}>
              <header>
                <h3 className="check-title">{checkLabel(check.check_id)}</h3>
                <span className={`status-pill status-pill--${check.status}`}>{check.status}</span>
              </header>
              <p className="check-message">{check.message}</p>
              <ChipRow values={[...affected, ...sourceChips]} />
              <EvidenceBlock evidence={check.evidence} />
            </article>
          );
        })}
      </div>
    </>
  );
}

function NutritionPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const { totals, report } = artifacts;
  const sodiumLimit = report.constraint_summary.max_sodium_mg_per_day || 2300;
  const calorieTarget = report.profile_summary.calorie_target_kcal || 2000;
  return (
    <>
      <h2>Daily Nutrition Totals</h2>
      <div className="nutrition-grid">
        {totals.map((total) => {
          const nutrients = total.nutrients;
          return (
            <article className="day-card" key={total.day}>
              <div className="day-number">Day {total.day}</div>
              <div className="nutrient-stack">
                <NutrientBar
                  flagged={nutrients.energy_kcal > calorieTarget * 1.15 || nutrients.energy_kcal < calorieTarget * 0.85}
                  label="Calories"
                  target={calorieTarget}
                  unit="kcal"
                  value={nutrients.energy_kcal}
                />
                <NutrientBar label="Protein" target={report.profile_summary.protein_target_g || 98.4} unit="g" value={nutrients.protein_g} />
                <NutrientBar flagged={nutrients.sodium_mg >= sodiumLimit} label="Sodium" target={sodiumLimit} unit="mg" value={nutrients.sodium_mg} />
                <NutrientBar label="Added Sugar" target={30} unit="g" value={nutrients.added_sugar_g} />
                <NutrientBar
                  flagged={total.saturated_fat_pct_calories > (report.constraint_summary.max_saturated_fat_pct_calories || 10)}
                  label="Sat Fat"
                  target={report.constraint_summary.max_saturated_fat_pct_calories || 10}
                  unit="% kcal"
                  value={total.saturated_fat_pct_calories}
                />
              </div>
            </article>
          );
        })}
      </div>
    </>
  );
}

function FoodsPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const { resolved, unresolved, excludedUnresolved } = artifacts;
  const reviewRows = artifacts.normalizationReview?.rows || [];
  const unresolvedRows = unresolved.map((item) => ({
    ...item,
    ...unresolvedSourceLink(item, reviewRows),
    recovery_action: recoveryActionForUnresolvedReason(item.unresolved_reason),
  }));
  const unresolvedSummaryRows = unresolvedSummary(unresolved);
  const unresolvedFields = reviewRows.length
    ? ["day", "meal", "source_item_id", "source_text", "food", "quantity", "unit", "quantity_text", "unresolved_reason", "recovery_action"]
    : ["day", "meal", "food", "quantity", "unit", "quantity_text", "unresolved_reason", "recovery_action"];
  const excludedRows = (excludedUnresolved || []).map((item) => ({
    day: item.day,
    meal: item.meal,
    food: item.food,
    quantity: `${round(item.quantity)} ${item.unit}`,
    deterministic_grams: round(item.deterministic_grams),
    unresolved_reason: item.unresolved_reason,
    exclusion_reason: item.exclusion_reason,
  }));
  const resolvedRows = resolved.map((item) => ({
    day: item.day,
    meal: item.meal,
    food: item.food,
    grams: round(item.grams),
    calories: round(item.nutrients.energy_kcal),
    sodium_mg: round(item.nutrients.sodium_mg),
  }));
  return (
    <>
      <h2>Food Resolution</h2>
      <div className="food-sections">
        <article className="food-card">
          <header>
            <h3 className="food-title">Unresolved Summary</h3>
            <span className="status-pill status-pill--info">{unresolvedSummaryRows.length}</span>
          </header>
          {unresolvedSummaryRows.length === 0 ? (
            <p className="empty-state">None.</p>
          ) : (
            <DataTable rows={unresolvedSummaryRows} fields={["reason", "count", "affected", "recovery_action"]} />
          )}
        </article>
        <article className="food-card">
          <header>
            <h3 className="food-title">Unresolved Foods</h3>
            <span className="status-pill status-pill--block">{unresolved.length}</span>
          </header>
          {unresolved.length === 0 ? (
            <p className="empty-state">None.</p>
          ) : (
            <DataTable rows={unresolvedRows as Record<string, unknown>[]} fields={unresolvedFields} />
          )}
        </article>
        <article className="food-card">
          <header>
            <h3 className="food-title">Excluded From Totals</h3>
            <span className="status-pill status-pill--warn">{excludedRows.length}</span>
          </header>
          {excludedRows.length === 0 ? (
            <p className="empty-state">None.</p>
          ) : (
            <DataTable rows={excludedRows} fields={["day", "meal", "food", "quantity", "deterministic_grams", "unresolved_reason", "exclusion_reason"]} />
          )}
        </article>
        <article className="food-card">
          <header>
            <h3 className="food-title">Resolved Foods</h3>
            <span className="status-pill status-pill--pass">{resolved.length}</span>
          </header>
          <DataTable rows={resolvedRows} fields={["day", "meal", "food", "grams", "calories", "sodium_mg"]} />
        </article>
      </div>
    </>
  );
}

function RecommendationPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const recommendation = artifacts.recommendation || null;
  if (!recommendation) {
    return (
      <>
        <h2>Recommendation</h2>
        <p className="empty-state">No recommendation artifact is available for this report.</p>
      </>
    );
  }

  const changes = recommendation.changes || [];
  const projectedDecision = recommendation.projected_decision?.decision || "-";
  const statusTone = recommendationStatusTone(recommendation.status);
  const rows = recommendationChangeRows(changes, artifacts.normalizationReview?.rows || []);
  const blockingRows = (recommendation.blocking_checks || []).map((checkID) => ({
    check: checkLabel(checkID),
    check_id: checkID,
  }));

  return (
    <>
      <h2>Recommendation</h2>
      <div className="metric-grid recommendation-metrics">
        <Metric label="Status" value={readableID(recommendation.status)} />
        <Metric label="Source decision" value={readableID(recommendation.source_decision)} />
        <Metric label="Projected decision" value={projectedDecision === "-" ? "-" : readableID(projectedDecision)} />
        <Metric label="Changes" value={String(changes.length)} />
      </div>
      <div className="food-sections recommendation-sections">
        <article className="food-card recommendation-card">
          <header>
            <h3 className="food-title">{recommendation.status === "available" ? "Verified Changes" : "No Verified Change"}</h3>
            <span className={`status-pill status-pill--${statusTone}`}>{readableID(recommendation.status)}</span>
          </header>
          <p className="check-message">{recommendation.reason}</p>
          <ChipRow values={[`Source plan ${recommendation.source_plan_id}`]} />
        </article>
        {recommendation.status === "available" ? (
          <article className="food-card recommendation-card">
            <header>
              <h3 className="food-title">Change List</h3>
              <span className="status-pill status-pill--pass">{changes.length}</span>
            </header>
            {rows.length ? (
              <DataTable rows={rows} fields={["operation", "day", "meal", "source_item_id", "source_text", "from", "to", "prep_note", "reason", "addresses_checks"]} />
            ) : (
              <p className="empty-state">None.</p>
            )}
          </article>
        ) : (
          <article className="food-card recommendation-card">
            <header>
              <h3 className="food-title">Blocking Checks</h3>
              <span className="status-pill status-pill--info">{blockingRows.length}</span>
            </header>
            {blockingRows.length ? (
              <DataTable rows={blockingRows} fields={["check", "check_id"]} />
            ) : (
              <p className="empty-state">None.</p>
            )}
          </article>
        )}
      </div>
    </>
  );
}

function recommendationChangeRows(changes: RecommendationChange[], reviewRows: NormalizedPlanReviewRow[]): Record<string, unknown>[] {
  return changes.map((change) => {
    const source = recommendationSourceLink(change, reviewRows);
    return {
      operation: readableID(change.operation),
      day: change.day ? `Day ${change.day}` : "-",
      meal: change.meal ? readableID(change.meal) : "-",
      source_item_id: source.source_item_id || "-",
      source_text: source.source_text || "-",
      from: recommendationFoodText(change.from),
      to: recommendationFoodText(change.to),
      prep_note: change.prep_note || "-",
      reason: change.reason,
      addresses_checks: (change.addresses_checks || []).map(checkLabel).join(", ") || "-",
    };
  });
}

function recommendationSourceLink(change: RecommendationChange, rows: NormalizedPlanReviewRow[]): Record<string, unknown> {
  if (!change.from || !change.day || !change.meal) return {};
  const candidates = rows.filter((row) => (
    row.day === change.day
    && mealMatches(row, change.meal || "")
    && lower(row.normalized_food) === lower(change.from?.food)
  ));
  const row = candidates[0];
  if (!row) return {};
  return {
    source_item_id: row.source_item_id,
    source_text: row.source_text,
  };
}

function recommendationFoodText(item?: RecommendationFoodItem): string {
  if (!item) return "-";
  const quantity = item.quantity !== undefined && item.quantity !== null
    ? [valueText(item.quantity), item.unit].filter(Boolean).join(" ")
    : item.quantity_text || "";
  const details = [quantity, item.preparation, item.brand].filter(Boolean).join(", ");
  return details ? `${item.food} (${details})` : item.food;
}

function recommendationStatusTone(status: string): string {
  if (status === "available") return "pass";
  if (status === "unavailable") return "info";
  return "warn";
}

function NormalizationPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const review = artifacts.normalizationReview || null;
  const extraction = artifacts.localModelExtraction || null;
  const events = artifacts.normalizationEvents || [];
  const actions = artifacts.reviewActions || [];
  if (!review && !extraction && events.length === 0 && actions.length === 0) {
    return (
      <>
        <h2>Normalization Trace</h2>
        <p className="empty-state">No normalization trace artifacts are available for this report.</p>
      </>
    );
  }

  const sourceRows = sourceInventoryRows(artifacts);
  const normalizedRows = (review?.rows || []).map((row) => ({
    day: row.day,
    meal: readableID(row.meal_label || row.meal_code),
    source_item_id: row.source_item_id,
    source_text: row.source_text,
    normalized_food: row.normalized_food || "-",
    quantity: reviewQuantityText(row),
    status: row.resolved ? "Resolved" : reasonLabel(row.unresolved_reason || "unresolved"),
  }));
  const repairRows = normalizationRepairRows(artifacts);
  const eventRows = events.map((event) => ({
    event: readableID(event.type),
    message: event.message,
    created_at: event.created_at,
  }));
  const actionRows = actions.map((action) => ({
    action: readableID(action.action),
    source_item_id: action.source_item_id ?? "-",
    source_text: action.source_text || "-",
    before: reviewActionItemText(action.before),
    after: reviewActionItemText(action.after),
    reason: action.reason || "-",
    created_at: action.created_at,
  }));

  return (
    <>
      <h2>Normalization Trace</h2>
      <div className="metric-grid normalization-metrics">
        <Metric label="Source items" value={String(review?.trust_signals.source_item_count ?? extraction?.source_item_count ?? sourceRows.length)} />
        <Metric label="Normalized rows" value={String(review?.trust_signals.normalized_row_count ?? normalizedRows.length)} />
        <Metric label="Unresolved" value={String(review?.trust_signals.unresolved_item_count ?? unresolvedReviewRowCount(review?.rows || []))} />
        <Metric label="Repairs" value={String(review?.trust_signals.repair_count ?? repairRows.length)} />
        <Metric label="Chunks" value={String(extraction?.chunk_count ?? extraction?.chunks?.length ?? 0)} />
      </div>
      <div className="food-sections normalization-sections">
        <TraceSection title="Source Inventory" count={sourceRows.length}>
          {sourceRows.length ? (
            <DataTable rows={sourceRows} fields={["day", "meal", "source_item_id", "source_text", "parse_status"]} />
          ) : (
            <p className="empty-state">None.</p>
          )}
        </TraceSection>
        <TraceSection title="Normalized Rows" count={normalizedRows.length}>
          {normalizedRows.length ? (
            <DataTable rows={normalizedRows} fields={["day", "meal", "source_item_id", "source_text", "normalized_food", "quantity", "status"]} />
          ) : (
            <p className="empty-state">None.</p>
          )}
        </TraceSection>
        <TraceSection title="Repairs" count={repairRows.length}>
          {repairRows.length ? (
            <DataTable rows={repairRows} fields={["day", "meal", "source_item_id", "source_text", "field", "from", "to", "reason"]} />
          ) : (
            <p className="empty-state">None.</p>
          )}
        </TraceSection>
        <TraceSection title="Review Actions" count={actionRows.length}>
          {actionRows.length ? (
            <DataTable rows={actionRows} fields={["action", "source_item_id", "source_text", "before", "after", "reason", "created_at"]} />
          ) : (
            <p className="empty-state">None.</p>
          )}
        </TraceSection>
        <TraceSection title="Normalization Events" count={eventRows.length}>
          {eventRows.length ? (
            <DataTable rows={eventRows} fields={["event", "message", "created_at"]} />
          ) : (
            <p className="empty-state">None.</p>
          )}
        </TraceSection>
      </div>
    </>
  );
}

function reviewActionItemText(item: ReviewActionArtifact["before"]): string {
  if (!item) return "-";
  const food = String(item.food || "").trim();
  const quantityText = String(item.quantity_text || "").trim();
  const quantity = item.quantity;
  const unit = String(item.unit || "").trim();
  const amount = quantityText || (quantity !== undefined && quantity !== null ? [valueText(quantity), unit].filter(Boolean).join(" ") : "");
  const status = String(item.unresolved_reason || item.resolution_status || "").trim();
  const parts = [food || "-", amount ? `(${amount})` : ""].filter(Boolean);
  if (status) parts.push(reasonLabel(status));
  return parts.join(" ");
}

function TraceSection({ title, count, children }: { title: string; count: number; children: ReactNode }) {
  return (
    <article className="food-card trace-card">
      <header>
        <h3 className="food-title">{title}</h3>
        <span className="status-pill status-pill--info">{count}</span>
      </header>
      {children}
    </article>
  );
}

function sourceInventoryRows(artifacts: ReportArtifacts): Record<string, unknown>[] {
  const rows = new Map<number, Record<string, unknown>>();
  for (const row of artifacts.normalizationReview?.rows || []) {
    if (rows.has(row.source_item_id)) continue;
    rows.set(row.source_item_id, {
      day: row.day,
      meal: readableID(row.meal_label || row.meal_code),
      source_item_id: row.source_item_id,
      source_text: row.source_text,
      parse_status: row.source_parse_status || "-",
    });
  }
  for (const chunk of artifacts.localModelExtraction?.chunks || []) {
    for (const source of chunk.source_items || []) {
      if (rows.has(source.id)) continue;
      rows.set(source.id, {
        day: source.day || chunk.day,
        meal: readableID(chunk.meal_label || source.meal_code || chunk.meal_code),
        source_item_id: source.id,
        source_text: source.text,
        parse_status: source.parse_status || "-",
      });
    }
  }
  return [...rows.values()].sort((a, b) => Number(a.source_item_id) - Number(b.source_item_id));
}

function normalizationRepairRows(artifacts: ReportArtifacts): Record<string, unknown>[] {
  const sourceRows = sourceInventoryRows(artifacts);
  const sourceByID = new Map(sourceRows.map((row) => [Number(row.source_item_id), row]));
  const rows: Record<string, unknown>[] = [];
  for (const chunk of artifacts.localModelExtraction?.chunks || []) {
    for (const repair of chunk.reconciliation?.repairs || []) {
      const source = sourceByID.get(repair.source_item_id);
      rows.push({
        day: source?.day || chunk.day,
        meal: source?.meal || readableID(chunk.meal_label || chunk.meal_code),
        source_item_id: repair.source_item_id,
        source_text: source?.source_text || "-",
        field: readableID(repair.field),
        from: repair.from || "-",
        to: repair.to || "-",
        reason: readableID(repair.reason),
      });
    }
  }
  return rows;
}

function unresolvedSourceLink(item: UnresolvedFood, rows: NormalizedPlanReviewRow[]): Record<string, unknown> {
  const row = matchReviewRowForUnresolved(item, rows);
  if (!row) return {};
  return {
    source_item_id: row.source_item_id,
    source_text: row.source_text,
  };
}

function matchReviewRowForUnresolved(item: UnresolvedFood, rows: NormalizedPlanReviewRow[]): NormalizedPlanReviewRow | null {
  const candidates = rows.filter((row) => {
    if (row.resolved) return false;
    if (row.day !== item.day) return false;
    if (!mealMatches(row, item.meal)) return false;
    if (lower(row.normalized_food) !== lower(item.food)) return false;
    if (item.unresolved_reason && row.unresolved_reason !== item.unresolved_reason) return false;
    return true;
  });
  if (candidates.length <= 1) return candidates[0] || null;
  return candidates.find((row) => row.quantity_text === item.quantity_text)
    || candidates.find((row) => row.unit === item.unit && row.quantity === item.quantity)
    || candidates[0]
    || null;
}

function mealMatches(row: NormalizedPlanReviewRow, meal: string): boolean {
  const target = lower(meal);
  return [row.meal_label, row.meal_code, readableID(row.meal_code)].some((value) => lower(value) === target);
}

function lower(value: unknown): string {
  return String(value || "").trim().toLowerCase();
}

function unresolvedReviewRowCount(rows: NormalizedPlanReviewRow[]): number {
  return rows.filter((row) => !row.resolved || row.unresolved_reason).length;
}

function reviewQuantityText(row: NormalizedPlanReviewRow): string {
  if (row.quantity_text) return row.quantity_text;
  if (row.quantity !== undefined && row.quantity !== null) {
    return [valueText(row.quantity), row.unit].filter(Boolean).join(" ");
  }
  return "-";
}

function unresolvedSummary(items: UnresolvedFood[]): Record<string, unknown>[] {
  const groups = new Map<string, { reason: string; count: number; affected: Set<string>; recovery: string }>();
  for (const item of items) {
    const reason = String(item.unresolved_reason || "unresolved");
    const group = groups.get(reason) || {
      reason,
      count: 0,
      affected: new Set<string>(),
      recovery: recoveryActionForUnresolvedReason(reason),
    };
    group.count += 1;
    group.affected.add([`Day ${item.day}`, readableID(item.meal)].filter(Boolean).join(" "));
    groups.set(reason, group);
  }
  return [...groups.values()]
    .sort((a, b) => b.count - a.count || a.reason.localeCompare(b.reason))
    .map((group) => ({
      reason: reasonLabel(group.reason),
      count: group.count,
      affected: [...group.affected].sort().join(", "),
      recovery_action: group.recovery,
    }));
}

function SourcesPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const inspection = buildSourceInspection(artifacts);
  return (
    <>
      <h2>Source Inspection</h2>
      <div className="metric-grid source-metrics">
        <Metric label="Citation Sources" value={String(inspection.summary.citationSourceCount)} />
        <Metric label="Check References" value={String(inspection.summary.checkSourceRefCount)} />
        <Metric label="Food Trace Rows" value={String(inspection.summary.foodTraceCount)} />
        <Metric label="Missing Refs" value={String(inspection.summary.missingSourceRefCount)} />
      </div>
      <div className="source-inspection-stack">
        <TraceSection title="Check Source Trace" count={inspection.checkRows.length}>
          {inspection.checkRows.length ? (
            <DataTable rows={inspection.checkRows} fields={["check", "status", "affected", "source_refs", "sources", "missing_source_refs", "message"]} />
          ) : (
            <p className="empty-state">No check source references are available.</p>
          )}
        </TraceSection>
        <TraceSection title="Food Source Trace" count={inspection.foodRows.length}>
          {inspection.foodRows.length ? (
            <DataTable
              rows={inspection.foodRows}
              fields={["day", "meal", "source_item_id", "source_text", "food", "quantity", "status", "reason", "recovery_action", "grams"]}
            />
          ) : (
            <p className="empty-state">No food trace rows are available.</p>
          )}
        </TraceSection>
        {inspection.missingSourceRows.length ? (
          <TraceSection title="Missing Source References" count={inspection.missingSourceRows.length}>
            <DataTable rows={inspection.missingSourceRows} fields={["source_ref", "referenced_by_checks"]} />
          </TraceSection>
        ) : null}
        <div className="source-subsection">
          <div className="source-subsection-header">
            <h3 className="food-title">Guideline Citations</h3>
            <span className="status-pill status-pill--info">{inspection.citationRows.length}</span>
          </div>
          {inspection.citationRows.length === 0 ? <p className="empty-state">No citations available.</p> : null}
        </div>
        <div className="source-list">
          {inspection.citationRows.map((source) => (
            <article className="source-card" key={source.source_id}>
              <header>
                <div className="source-heading">
                  <span className="source-identity-mark" aria-hidden="true" />
                  <div>
                    <h3 className="source-title">{source.title}</h3>
                    <p className="source-meta">{source.publisher || source.source_id}</p>
                  </div>
                </div>
                <a href={source.url} target="_blank" rel="noreferrer">Open source</a>
              </header>
              <ChipRow values={[source.source_id, source.referenced_by_checks === "-" ? "" : `Used by ${source.referenced_by_checks}`].filter(Boolean)} />
              {source.claims.length ? (
                <ul className="source-claim-list">
                  {source.claims.map((claim) => (
                    <li key={claim.claim_id}>
                      <strong>{sourceClaimLabel(claim.claim_id)}</strong>
                      {claim.summary ? <span>{claim.summary}</span> : null}
                      {claim.source_locator ? <small>{claim.source_locator}</small> : null}
                    </li>
                  ))}
                </ul>
              ) : null}
            </article>
          ))}
        </div>
      </div>
    </>
  );
}

function ReportDownloadPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const report = reportDownload(artifacts);
  return (
    <>
      <h2>Report</h2>
      <article className="report-download-card">
        <div>
          <h3>MealCheck report PDF</h3>
          <p>Download one shareable report with the decision, checks requiring attention, unresolved foods, daily totals, and disclaimer.</p>
          <p className="artifact-meta">{guidelineLabel(artifacts.report.guideline_pack_id)}</p>
        </div>
        {report ? (
          <a className="action-button action-button--primary report-download-link" download={report.downloadName} href={report.href} target={report.target} rel={report.rel}>
            {report.label}
          </a>
        ) : (
          <p className="empty-state">No report download is available for this run yet.</p>
        )}
      </article>
    </>
  );
}

function reportDownload(artifacts: ReportArtifacts) {
  const items = artifacts.artifactItems?.length
    ? artifacts.artifactItems.map((item) => ({ href: `${artifacts.apiBase}${item.url}`, path: item.path }))
    : artifacts.manifest.artifacts.map((path) => ({ href: `${artifacts.base}/${path}`, path }));
  const item = items.find((candidate) => candidate.path === "report.pdf")
    || items.find((candidate) => candidate.path === "report.html");
  if (!item) return null;
  return {
    href: item.href,
    label: item.path.endsWith(".pdf") ? "Download report PDF" : "Open printable report",
    downloadName: item.path.endsWith(".pdf") ? "mealcheck-report.pdf" : undefined,
    target: item.path.endsWith(".pdf") ? undefined : "_blank",
    rel: item.path.endsWith(".pdf") ? undefined : "noreferrer",
  };
}

function NutrientBar({
  label,
  value,
  target,
  unit,
  flagged = false,
}: {
  label: string;
  value: number;
  target: number;
  unit: string;
  flagged?: boolean;
}) {
  const width = target > 0 ? Math.max(2, Math.min(100, (Number(value) / target) * 100)) : 2;
  return (
    <div className="nutrient-row">
      <span className="metric-label">{label}</span>
      <div className="bar-track" role="img" aria-label={`${label} ${round(value)} ${unit}`}>
        <div className={`bar-fill${flagged ? " bar-fill--warn" : ""}`} style={{ width: `${width}%` }} />
      </div>
      <strong>{round(value)} {unit}</strong>
    </div>
  );
}

function DataTable({ rows, fields }: { rows: Record<string, unknown>[]; fields: string[] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            {fields.map((field) => <th key={field}>{readableID(field)}</th>)}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={`${String(row.food || row.meal || "row")}-${index}`}>
              {fields.map((field) => <td key={field}>{valueText(row[field])}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EvidenceBlock({ evidence }: { evidence?: unknown[] }) {
  if (!evidence || evidence.length === 0) return null;
  return (
    <details className="evidence">
      <summary>Why this was flagged</summary>
      <ul className="evidence-list">
        {evidence.map((entry, index) => <li key={index}>{evidenceText(entry)}</li>)}
      </ul>
    </details>
  );
}

function evidenceText(entry: unknown): string {
  if (!entry || typeof entry !== "object" || Array.isArray(entry)) {
    return valueText(entry);
  }
  const record = entry as Record<string, unknown>;
  const dayMeal = [record.day ? `Day ${record.day}` : "", record.meal ? readableID(record.meal) : ""].filter(Boolean).join(" ");
  const food = record.food ? String(record.food) : "";
  if (record.matched_allergen) {
    return `${prefixEvidence(dayMeal, food)}matches the declared ${record.matched_allergen} allergy.`;
  }
  if (record.unresolved_reason) {
    const quantity = record.quantity_text ? ` (${record.quantity_text})` : "";
    return `${prefixEvidence(dayMeal, food)}has ${reasonLabel(record.unresolved_reason)}${quantity}.`;
  }
  if (record.sodium_mg != null && record.limit_mg != null) {
    return `${prefixEvidence(dayMeal, food)}sodium is ${round(record.sodium_mg)} mg, above the ${round(record.limit_mg)} mg daily limit.`;
  }
  if (record.energy_kcal != null && record.target_kcal != null) {
    const tolerance = record.tolerance_pct != null ? ` with a ${round(record.tolerance_pct)}% tolerance` : "";
    return `${prefixEvidence(dayMeal, food)}calories are ${round(record.energy_kcal)} kcal against the ${round(record.target_kcal)} kcal target${tolerance}.`;
  }
  return Object.entries(record)
    .map(([key, value]) => `${readableID(key)}: ${valueText(value)}`)
    .join("; ");
}

function prefixEvidence(dayMeal: string, food: string): string {
  const location = [dayMeal, food].filter(Boolean).join(": ");
  return location ? `${location} ` : "";
}

function ChipRow({ values }: { values: string[] }) {
  if (!values.length) return null;
  return (
    <div className="chip-row">
      {values.map((entry) => <span className="chip" key={entry}>{entry}</span>)}
    </div>
  );
}
