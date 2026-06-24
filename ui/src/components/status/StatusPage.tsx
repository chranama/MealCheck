import { useEffect, useMemo, useState } from "react";
import { BrandMark } from "../brand/BrandMark";
import { SiteFooter } from "../shell/Shell";
import { cleanApiBase, fetchPublicStatus, joinUrl } from "../../lib/api";
import { configuredApiBase } from "../../lib/runtime_config";
import type { PublicStatusResponse, PublicStatusState, RuntimeConfig, StatusComponent, StatusIncident, StatusSummary } from "../../types";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; status: PublicStatusResponse }
  | { kind: "unreachable"; checkedAt: string };

const API_COMPONENTS: Array<Pick<StatusComponent, "id" | "name">> = [
  { id: "meal_check_submission", name: "Meal Check Submission" },
  { id: "ai_meal_normalization", name: "AI Meal Normalization" },
  { id: "nutrition_allergen_checking", name: "Nutrition & Allergen Checking" },
  { id: "report_generation", name: "Report Generation" },
  { id: "sample_report", name: "Sample Report" },
];

const API_UNREACHABLE_MESSAGE = "The MealCheck API could not be reached.";

export function StatusPage({ runtimeConfig }: { runtimeConfig: RuntimeConfig }) {
  const apiBase = useMemo(() => configuredApiBase(runtimeConfig), [runtimeConfig]);
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });

  async function refreshStatus() {
    if (!cleanApiBase(apiBase)) {
      setLoadState({ kind: "unreachable", checkedAt: new Date().toISOString() });
      return;
    }
    setLoadState((current) => current.kind === "ready" ? current : { kind: "loading" });
    try {
      const status = await fetchPublicStatus(apiBase);
      setLoadState({ kind: "ready", status });
    } catch {
      setLoadState({ kind: "unreachable", checkedAt: new Date().toISOString() });
    }
  }

  useEffect(() => {
    refreshStatus();
  }, [apiBase]);

  const summary = statusSummary(loadState);
  const components = statusComponents(loadState);
  const incidents = loadState.kind === "ready" ? loadState.status.recent_incidents : [];
  const generatedAt = loadState.kind === "ready" ? loadState.status.generated_at : loadState.kind === "unreachable" ? loadState.checkedAt : "";
  const sampleReport = loadState.kind === "ready" ? loadState.status.links?.sample_report || "" : "";

  return (
    <div className="app-shell status-page">
      <header className="topbar status-topbar">
        <div className="brand-cluster">
          <BrandMark />
          <div className="brand-block">
            <p className="eyebrow">Operational status</p>
            <h1>MealCheck Status</h1>
          </div>
        </div>
        <nav className="status-nav" aria-label="Status page navigation">
          <a href="/">MealCheck</a>
          <a href="/about.html">About</a>
        </nav>
      </header>

      <main className="status-layout">
        <section className={`status-hero status-hero--${stateTone(summary.state)}`} aria-labelledby="status-summary-title">
          <div>
            <p className="eyebrow">Current status</p>
            <h2 id="status-summary-title">{summary.message}</h2>
            <p className="status-updated">{generatedAt ? `Last checked ${formatDateTime(generatedAt)}` : "Checking current status."}</p>
          </div>
          <button className="action-button action-button--secondary" type="button" onClick={refreshStatus}>
            Refresh
          </button>
        </section>

        <section className="panel status-panel" aria-labelledby="status-components-title">
          <div className="panel-heading">
            <h2 id="status-components-title">Components</h2>
          </div>
          <div className="status-component-list">
            {components.map((component) => (
              <article className="status-component-row" key={component.id}>
                <div>
                  <h3>{component.name}</h3>
                  {component.message ? <p>{component.message}</p> : null}
                </div>
                <span className={`status-pill status-pill--${stateTone(component.state)}`}>{formatState(component.state)}</span>
              </article>
            ))}
          </div>
          {sampleReport ? (
            <p className="status-supporting-link">
              <a href={joinUrl(apiBase, sampleReport)}>Open sample report</a>
            </p>
          ) : null}
        </section>

        <section className="panel status-panel" aria-labelledby="status-incidents-title">
          <div className="panel-heading">
            <h2 id="status-incidents-title">Recent Incidents</h2>
          </div>
          <IncidentList incidents={incidents} apiReachable={loadState.kind === "ready"} />
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}

function statusSummary(loadState: LoadState): StatusSummary {
  if (loadState.kind === "ready") return loadState.status.overall;
  if (loadState.kind === "unreachable") {
    return {
      state: "major_outage",
      message: "MealCheck API is currently unreachable",
    };
  }
  return {
    state: "unknown",
    message: "Checking MealCheck status",
  };
}

function statusComponents(loadState: LoadState): StatusComponent[] {
  const website: StatusComponent = { id: "website", name: "Website", state: "operational" };
  if (loadState.kind === "ready") {
    return [website, ...loadState.status.components];
  }
  if (loadState.kind === "loading") {
    return [
      website,
      ...API_COMPONENTS.map((component) => ({
        ...component,
        state: "unknown",
        message: "Checking current status.",
      })),
    ];
  }
  return [
    website,
    ...API_COMPONENTS.map((component) => ({
      ...component,
      state: "major_outage",
      message: API_UNREACHABLE_MESSAGE,
    })),
  ];
}

function IncidentList({ incidents, apiReachable }: { incidents: StatusIncident[]; apiReachable: boolean }) {
  if (!apiReachable) {
    return <p className="status-empty">Incident history is unavailable while the API is unreachable.</p>;
  }
  if (incidents.length === 0) {
    return <p className="status-empty">No incidents reported in the past 7 days.</p>;
  }
  return (
    <div className="status-incident-list">
      {incidents.map((incident) => (
        <article className="status-incident-row" key={incident.id}>
          <div>
            <h3>{incident.title}</h3>
            <p>{formatDateTime(incident.started_at)}</p>
          </div>
          <span className={`status-pill status-pill--${stateTone(incident.state)}`}>{formatState(incident.state)}</span>
        </article>
      ))}
    </div>
  );
}

function stateTone(state: PublicStatusState): string {
  switch (state) {
    case "operational":
      return "pass";
    case "degraded_performance":
    case "maintenance":
      return "warn";
    case "partial_outage":
    case "major_outage":
      return "block";
    default:
      return "info";
  }
}

function formatState(state: PublicStatusState): string {
  return state
    .split("_")
    .filter(Boolean)
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(" ");
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}
