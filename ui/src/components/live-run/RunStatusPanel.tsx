import { readableID } from "../../lib/format";
import { recoveryFromRunFailure } from "../../lib/recovery";
import type { LiveState, MealPlanQualificationResult, QualificationState } from "../../types";
import { RecoveryNoticeView } from "../common/RecoveryNotice";

export function RunStatusPanel({ live, qualification }: { live: LiveState; qualification: QualificationState }) {
  const hasEvents = live.events.length > 0;
  const hasReport = live.status !== "deleted" && live.artifactItems.length > 0;
  const runRecovery = recoveryFromRunFailure(live.status, live.message);
  return (
    <section className="panel live-panel results-panel" id="live-status-panel">
      <div className="panel-heading">
        <h2>Results</h2>
      </div>
      <div className="status-stack">
        <p className="summary-text">{live.message || "Your report will appear here after you create a check."}</p>
        <QualificationNotice qualification={qualification} />
        {runRecovery ? <RecoveryNoticeView notice={runRecovery} role="alert" /> : null}
        {hasReport ? (
          <div className="notice notice--pass live-report-ready" role="status">
            <strong>Report available</strong>
            <p>Decision details are available below.</p>
          </div>
        ) : null}
      </div>
      {hasEvents ? (
        <details className="activity-details">
          <summary>Activity details</summary>
          <div className="event-list">
            {live.events.map((event, index) => (
              <div className="event-row" key={`${event.type}-${index}`}>
                <strong>{readableID(event.type)}</strong>
                <span>{event.message}</span>
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </section>
  );
}

function QualificationNotice({ qualification }: { qualification: QualificationState }) {
  if (qualification.status === "idle") return null;
  const result = qualification.result || null;
  return (
    <div className={`notice notice--${qualificationTone(result, qualification.status)}`} role="status">
      <strong>Qualification</strong>
      <p>{qualification.message}</p>
      {result ? (
        <dl className="qualification-facts">
          <div>
            <dt>Result</dt>
            <dd>{readableID(result.status)}</dd>
          </div>
          <div>
            <dt>Provider</dt>
            <dd>{result.provider_used ? "Used" : "Not used"}</dd>
          </div>
          {normalizedPlanSummary(result) ? (
            <div>
              <dt>Plan</dt>
              <dd>{normalizedPlanSummary(result)}</dd>
            </div>
          ) : null}
          {result.missing_fields?.length ? (
            <div>
              <dt>Missing</dt>
              <dd>{result.missing_fields.join(", ")}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </div>
  );
}

function qualificationTone(result: MealPlanQualificationResult | null, status: QualificationState["status"]): string {
  if (status === "failed") return "block";
  if (!result) return "info";
  if (result.status === "eligible_for_verification" || result.status === "eligible_with_unresolved_items") return "pass";
  if (result.status === "not_meal_plan") return "block";
  return "warn";
}

function normalizedPlanSummary(result: MealPlanQualificationResult): string {
  const plan = result.normalized_plan;
  if (!plan) return "";
  const dayCount = plan.days.length;
  const mealCount = plan.days.reduce((sum, day) => sum + day.meals.length, 0);
  const itemCount = plan.days.reduce((sum, day) => sum + day.meals.reduce((mealSum, meal) => mealSum + meal.items.length, 0), 0);
  return `${dayCount} day${dayCount === 1 ? "" : "s"}, ${mealCount} meal${mealCount === 1 ? "" : "s"}, ${itemCount} item${itemCount === 1 ? "" : "s"}`;
}
