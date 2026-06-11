import type { ReportArtifacts, ReportTab } from "../../types";
import { TABS } from "../../constants";
import { artifactType, readableID, round, sourceChip, valueText } from "../../lib/format";

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
        {activeTab === "artifacts" ? <ArtifactsPanel artifacts={artifacts} /> : null}
      </section>
    </>
  );
}

function ChecksPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const { decision, citations } = artifacts;
  return (
    <>
      <h2>Check Details</h2>
      <div className="check-list">
        {(decision.checks || []).map((check) => {
          const sourceChips = (check.source_refs || []).map((sourceID) => sourceChip(citations, sourceID));
          const affected = [
            ...(check.affected_days || []).map((day) => `Day ${day}`),
            ...(check.affected_meals || []),
          ];
          return (
            <article className="check-card" key={check.check_id}>
              <header>
                <h3 className="check-title">{readableID(check.check_id)}</h3>
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
      <h2>Source References</h2>
      {sources.length === 0 ? <p className="empty-state">No citations available.</p> : null}
      <div className="source-list">
        {sources.map((source) => (
          <article className="source-card" key={source.source_id}>
            <header>
              <div>
                <h3 className="source-title">{source.title}</h3>
                <p className="source-meta">{source.publisher || source.source_id}</p>
              </div>
              <a href={source.url} target="_blank" rel="noreferrer">Open source</a>
            </header>
            <div className="chip-row">
              {(source.claims_used || []).map((claim) => (
                <span className="chip" key={claim.claim_id}>{claim.claim_id}</span>
              ))}
            </div>
          </article>
        ))}
      </div>
    </>
  );
}

function ArtifactsPanel({ artifacts }: { artifacts: ReportArtifacts }) {
  const { manifest, base, artifactItems } = artifacts;
  const items = artifactItems?.length
    ? artifactItems.map((item) => ({ href: `${artifacts.apiBase}${item.url}`, path: item.path, type: item.type }))
    : manifest.artifacts.map((artifactPath) => ({ href: `${base}/${artifactPath}`, path: artifactPath, type: artifactType(artifactPath) }));
  return (
    <>
      <h2>Artifact Bundle</h2>
      <div className="artifact-list">
        {items.map((item) => (
          <div className="artifact-row" key={item.path}>
            <a href={item.href} target="_blank" rel="noreferrer">{item.path}</a>
            <p className="artifact-meta">{item.type}</p>
          </div>
        ))}
      </div>
    </>
  );
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
      <summary>Evidence</summary>
      <pre>{JSON.stringify(evidence, null, 2)}</pre>
    </details>
  );
}

function ChipRow({ values }: { values: string[] }) {
  if (!values.length) return null;
  return (
    <div className="chip-row">
      {values.map((entry) => <span className="chip" key={entry}>{entry}</span>)}
    </div>
  );
}
