import type { BackendState, DemoRun, LiveState, ReportArtifacts, ViewMode } from "../../types";
import { guidelineLabel, isMealPlanCheckID, liveStatusTone, readableID } from "../../lib/format";
import { Metric } from "../common/FormControls";

export function Sidebar({
  demos,
  selectedDemoID,
  view,
  onSelectDemo,
  onLive,
}: {
  demos: DemoRun[];
  selectedDemoID: string;
  view: ViewMode;
  onSelectDemo: (demo: DemoRun) => void;
  onLive: () => void;
}) {
  return (
    <aside className="sidebar" aria-label="MealCheck navigation">
      <div className="sidebar-heading">
        <span>Start</span>
      </div>
      <button
        className={`demo-button live-entry-button${view === "live" ? " is-active" : ""}`}
        id="live-entry-button"
        type="button"
        onClick={onLive}
      >
        <strong>New meal check</strong>
        <span>Use your own meal plan</span>
      </button>

      <div className="sidebar-divider" />
      <div className="sidebar-heading">
        <span>Examples</span>
      </div>
      <div className="demo-list">
        {demos.map((demo) => (
          <button
            className={`demo-button${view === "demo" && selectedDemoID === demo.id ? " is-active" : ""}`}
            key={demo.id}
            type="button"
            onClick={() => onSelectDemo(demo)}
          >
            <strong>{demo.title}</strong>
            <span>{demo.summary}</span>
          </button>
        ))}
      </div>
    </aside>
  );
}

export function EmptySummary() {
  return (
    <section className="summary-band">
      <section className="summary-main">
        <div className="decision-line">
          <span className="status-pill status-pill--info">Loading</span>
        </div>
        <h2 className="summary-title">MealCheck</h2>
        <p className="summary-text">Loading report artifacts.</p>
      </section>
    </section>
  );
}

export function ReportSummary({ reportTitle, artifacts }: { reportTitle: string; artifacts: ReportArtifacts }) {
  const { decision, report, totals, unresolved, resolved } = artifacts;
  const checks = (decision.checks || []).filter((check) => isMealPlanCheckID(check.check_id));
  const failedChecks = checks.filter((check) => check.status === "block" || check.status === "warn");
  return (
    <section className="summary-band">
      <section className="summary-main">
        <div className="decision-line">
          <span className={`decision-pill decision-pill--${decision.decision}`}>{decision.decision}</span>
          <span className="chip">Risk {decision.risk_level}</span>
          <span className="chip">{guidelineLabel(report.guideline_pack_id)}</span>
        </div>
        <AuditRail steps={["Decision", "Checks", "Sources", "Report"]} />
        <h2 className="summary-title">{reportTitle}</h2>
        <p className="summary-text">{decision.summary}</p>
      </section>
      <aside className="summary-side">
        <div className="metric-grid">
          <Metric label="Checks" value={String(checks.length)} />
          <Metric label="Needs Review" value={String(failedChecks.length)} />
          <Metric label="Resolved Foods" value={String(resolved.length)} />
          <Metric label="Unresolved" value={String(unresolved.length)} />
          <Metric label="Days" value={String(totals.length)} />
          <Metric label="Mode" value={artifacts.manifest.mode} />
        </div>
      </aside>
    </section>
  );
}

export function LiveSummary({
  apiBase,
  backend,
  live,
}: {
  apiBase: string;
  backend: BackendState;
  live: LiveState;
}) {
  const statusLabel = live.status === "idle" ? "Ready" : readableID(live.status);
  return (
    <section className="summary-band">
      <section className="summary-main">
        <div className="decision-line">
          <span className={`status-pill status-pill--${liveStatusTone(live.status)}`}>{statusLabel}</span>
          <span className="chip">{serviceSummaryLabel(backend, apiBase)}</span>
          {live.runID ? <span className="chip">Reference {live.runID}</span> : null}
        </div>
        <h2 className="summary-title">Check a meal plan</h2>
        <p className="summary-text">{live.message || "Enter a meal plan and MealCheck will return a clear decision with supporting details."}</p>
      </section>
    </section>
  );
}

function serviceSummaryLabel(backend: BackendState, apiBase: string) {
  if (backend.kind === "online") return "Live checks available";
  if (apiBase) return "Service unavailable";
  return "Examples available";
}

function AuditRail({ steps }: { steps: string[] }) {
  return (
    <div className="audit-rail" aria-label="Verification stages">
      {steps.map((step, index) => (
        <span className="audit-rail-step" key={step}>
          <span className="audit-rail-node" aria-hidden="true">{index + 1}</span>
          {step}
        </span>
      ))}
    </div>
  );
}
