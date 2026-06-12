import type { ReportArtifacts, ReportTab } from "../../types";
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
  const { resolved, unresolved } = artifacts;
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
            <h3 className="food-title">Unresolved Foods</h3>
            <span className="status-pill status-pill--block">{unresolved.length}</span>
          </header>
          {unresolved.length === 0 ? (
            <p className="empty-state">None.</p>
          ) : (
            <DataTable rows={unresolved as Record<string, unknown>[]} fields={["day", "meal", "food", "quantity_text", "unresolved_reason"]} />
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

function SourcesPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const sources = artifacts.citations?.sources || [];
  return (
    <>
      <h2>Guidelines And Sources</h2>
      {sources.length === 0 ? <p className="empty-state">No citations available.</p> : null}
      <div className="source-list">
        {sources.map((source) => (
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
            {source.claims_used?.length ? (
              <ul className="source-claim-list">
                {source.claims_used.map((claim) => (
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
