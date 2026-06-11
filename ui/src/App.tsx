import { useEffect, useRef, useState } from "react";
import {
  checkHealth,
  cleanApiBase,
  createRun,
  deleteRun,
  fetchEvents,
  fetchRun,
  loadDemoArtifacts,
  loadDemoIndex,
  loadLiveArtifacts as loadLiveArtifactsForRun,
} from "./lib/api";
import { configuredApiBase } from "./lib/runtime_config";
import { LiveWorkspace } from "./components/live-run/LiveWorkspace";
import { ReportSurface } from "./components/report/ReportSurface";
import {
  BackendStatus,
  EmptySummary,
  LiveSummary,
  ReportSummary,
  Sidebar,
} from "./components/shell/Shell";
import type {
  BackendState,
  DemoRun,
  LiveState,
  ReportArtifacts,
  ReportTab,
  RunPayload,
  RuntimeConfig,
  ViewMode,
} from "./types";

const INITIAL_BACKEND: BackendState = {
  online: false,
  label: "Static demo",
  kind: "idle",
};

const INITIAL_LIVE: LiveState = {
  runID: "",
  status: "idle",
  message: "",
  events: [],
  artifactItems: [],
};

export default function App({ runtimeConfig }: { runtimeConfig: RuntimeConfig }) {
  const [activeTab, setActiveTab] = useState<ReportTab>("checks");
  const [view, setView] = useState<ViewMode>("demo");
  const [apiBase, setApiBase] = useState(() => configuredApiBase(runtimeConfig));
  const [backend, setBackend] = useState<BackendState>(INITIAL_BACKEND);
  const [demos, setDemos] = useState<DemoRun[]>([]);
  const [selectedDemoID, setSelectedDemoID] = useState("");
  const [reportTitle, setReportTitle] = useState("");
  const [artifacts, setArtifacts] = useState<ReportArtifacts | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState<LiveState>(INITIAL_LIVE);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function boot() {
      await updateBackendHealth(apiBase);
      const index = await loadDemoIndex();
      if (cancelled) return;

      const demoRuns = index.demo_runs || [];
      setDemos(demoRuns);
      if (demoRuns.length > 0) {
        await loadDemo(demoRuns[0]);
      }
    }

    boot().catch(showError);
    return () => {
      cancelled = true;
      stopLivePolling();
    };
  }, []);

  async function updateBackendHealth(base = apiBase) {
    const cleanBase = cleanApiBase(base);
    if (!cleanBase) {
      setBackend(INITIAL_BACKEND);
      return;
    }

    const online = await checkHealth(cleanBase);
    setBackend({
      online,
      label: online ? "Online" : "Unavailable",
      kind: online ? "online" : "offline",
    });
  }

  async function loadDemo(demo: DemoRun) {
    stopLivePolling();
    setError("");
    setView("demo");
    setSelectedDemoID(demo.id);
    setReportTitle(demo.title);
    const nextArtifacts = await loadDemoArtifacts(demo);
    setActiveTab("checks");
    setArtifacts(nextArtifacts);
  }

  function showLiveWorkspace() {
    setError("");
    setView("live");
    setSelectedDemoID("");
    setReportTitle(live.runID ? `Live Run ${live.runID}` : "Live Run");
  }

  async function createLiveRun(base: string, inviteToken: string, payload: RunPayload) {
    const cleanBase = cleanApiBase(base);
    if (!cleanBase) {
      throw new Error("API base URL is required for live runs.");
    }
    if (!inviteToken.trim()) {
      throw new Error("Invite token is required for live runs.");
    }

    stopLivePolling();
    setApiBase(cleanBase);
    setArtifacts(null);
    setActiveTab("checks");
    setError("");
    setLive({
      runID: "",
      status: "queued",
      message: "Submitting run.",
      events: [],
      artifactItems: [],
    });

    const created = await createRun(cleanBase, inviteToken, payload);
    setLive((current) => ({
      ...current,
      runID: created.run_id,
      status: created.status,
      message: "Run queued.",
    }));
    startLivePolling(cleanBase, created.run_id);
  }

  function startLivePolling(base: string, runID: string) {
    stopLivePolling();
    pollLiveRun(base, runID).catch(showError);
    pollRef.current = window.setInterval(() => {
      pollLiveRun(base, runID).catch(showError);
    }, 1500);
  }

  function stopLivePolling() {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  async function pollLiveRun(base: string, runID: string) {
    const [runDoc, events] = await Promise.all([
      fetchRun(base, runID),
      fetchEvents(base, runID, live.events),
    ]);
    const run = runDoc.run;
    setLive((current) => ({
      ...current,
      runID,
      status: run.status,
      message: run.error || run.summary || `Run ${run.status}.`,
      events,
    }));
    if (run.status === "completed") {
      stopLivePolling();
      await loadLiveArtifacts(base, runID);
    } else if (run.status === "failed" || run.status === "deleted") {
      stopLivePolling();
    }
  }

  async function loadLiveArtifacts(base: string, runID: string) {
    const nextArtifacts = await loadLiveArtifactsForRun(base, runID);
    const artifactItems = nextArtifacts.artifactItems || [];
    setReportTitle(`Live Run ${runID}`);
    setLive((current) => ({ ...current, artifactItems }));
    setArtifacts(nextArtifacts);
  }

  async function deleteLiveRun() {
    if (!live.runID) {
      showError(new Error("No live run is selected."));
      return;
    }

    await deleteRun(apiBase, live.runID);
    stopLivePolling();
    setArtifacts(null);
    setLive((current) => ({
      ...current,
      status: "deleted",
      message: "Run deleted.",
      artifactItems: [],
    }));
  }

  function showError(errorLike: unknown) {
    const message = errorLike instanceof Error ? errorLike.message : String(errorLike);
    setError(message);
    setLive((current) => ({ ...current, status: "failed", message }));
  }

  const selectedDemo = demos.find((demo) => demo.id === selectedDemoID) || null;

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand-block">
          <p className="eyebrow">Seeded public demo</p>
          <h1>MealCheck</h1>
          <p className="brand-subtitle">Evidence-backed meal plan verification</p>
        </div>
        <BackendStatus backend={backend} />
      </header>

      <main className="main-layout">
        <Sidebar
          demos={demos}
          selectedDemoID={selectedDemo?.id || ""}
          view={view}
          onSelectDemo={(demo) => loadDemo(demo).catch(showError)}
          onLive={showLiveWorkspace}
        />

        <section className="workspace" aria-live="polite">
          {view === "live" ? (
            <LiveSummary apiBase={apiBase} backend={backend} live={live} />
          ) : artifacts ? (
            <ReportSummary reportTitle={reportTitle} artifacts={artifacts} />
          ) : (
            <EmptySummary />
          )}

          {view === "live" ? (
            <LiveWorkspace
              apiBase={apiBase}
              setApiBase={setApiBase}
              backend={backend}
              live={live}
              onHealthCheck={updateBackendHealth}
              onCreateRun={createLiveRun}
              onDeleteRun={deleteLiveRun}
              onError={showError}
            />
          ) : null}

          {error ? <section className="panel error-state">{error}</section> : null}

          {artifacts ? (
            <ReportSurface
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              artifacts={artifacts}
            />
          ) : null}
        </section>
      </main>
    </div>
  );
}
