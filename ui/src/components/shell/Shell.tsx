import type { BackendState, DemoRun, LiveState, ReportArtifacts, ViewMode } from "../../types";
import { liveStatusTone, readableID } from "../../lib/format";
import { Metric } from "../common/FormControls";

export function BackendStatus({ backend }: { backend: BackendState }) {
  return (
    <div className="backend-status" aria-live="polite">
      <span className={`status-dot status-dot--${backend.kind}`} />
      <div>
        <span className="status-label">Backend</span>
        <strong>{backend.label}</strong>
      </div>
    </div>
  );
}

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
    <aside className="sidebar" aria-label="Demo runs">
      <div className="sidebar-heading">
        <span>Demo Runs</span>
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
      <div className="sidebar-divider" />
      <div className="sidebar-heading">
        <span>Live Run</span>
      </div>
      <button
        className={`demo-button live-entry-button${view === "live" ? " is-active" : ""}`}
        id="live-entry-button"
        type="button"
        onClick={onLive}
      >
        <strong>New MealCheck Run</strong>
        <span>Manual or BYOK</span>
      </button>
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
  const checks = decision.checks || [];
  const failedChecks = checks.filter((check) => check.status === "block" || check.status === "warn");
  return (
    <section className="summary-band">
      <section className="summary-main">
        <div className="decision-line">
          <span className={`decision-pill decision-pill--${decision.decision}`}>{decision.decision}</span>
          <span className="chip">Risk {decision.risk_level}</span>
          <span className="chip">{report.guideline_pack_id}</span>
        </div>
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
          <span className="chip">{apiBase || "No API base"}</span>
          {live.runID ? <span className="chip">{live.runID}</span> : null}
        </div>
        <h2 className="summary-title">Live Run</h2>
        <p className="summary-text">{live.message || "Manual or BYOK verification run."}</p>
      </section>
      <aside className="summary-side">
        <div className="metric-grid">
          <Metric label="Backend" value={backend.online ? "Online" : apiBase ? "Offline" : "Static"} />
          <Metric label="Queue" value="3" />
          <Metric label="Active" value="1" />
          <Metric label="Retention" value="7 days" />
        </div>
      </aside>
    </section>
  );
}
