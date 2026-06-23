import type { BackendState, LiveState } from "../../types";
import { liveStatusTone, readableID } from "../../lib/format";

export function Sidebar({
  onLive,
}: {
  onLive: () => void;
}) {
  return (
    <aside className="sidebar" aria-label="MealCheck navigation">
      <div className="sidebar-heading">
        <span>Start</span>
      </div>
      <button
        className="demo-button live-entry-button is-active"
        id="live-entry-button"
        type="button"
        onClick={onLive}
      >
        <strong>New meal check</strong>
        <span>Use your own meal plan</span>
      </button>
    </aside>
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
  if (backend.kind === "online" && backend.hostedMode === "local_model") {
    return backend.localModel?.ready ? "Local model available" : "Local model unavailable";
  }
  if (backend.kind === "online") return "Live checks available";
  if (apiBase) return "Service unavailable";
  return "Service not configured";
}
