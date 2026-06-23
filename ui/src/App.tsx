import { useEffect, useRef, useState } from "react";
import {
  cleanApiBase,
  createRun,
  deleteRun,
  fetchHealth,
  fetchEvents,
  fetchRun,
  loadLiveArtifacts as loadLiveArtifactsForRun,
  qualifyMealPlan,
} from "./lib/api";
import { configuredApiBase } from "./lib/runtime_config";
import { LiveWorkspace } from "./components/live-run/LiveWorkspace";
import { ReportSurface } from "./components/report/ReportSurface";
import { BrandMark } from "./components/brand/BrandMark";
import {
  LiveSummary,
  Sidebar,
} from "./components/shell/Shell";
import type {
  BackendState,
  LiveState,
  MealPlanQualificationResult,
  QualificationState,
  QualifyMealPlanPayload,
  ReportArtifacts,
  ReportTab,
  RunPayload,
  RuntimeConfig,
} from "./types";

const INITIAL_BACKEND: BackendState = {
  online: false,
  label: "Not configured",
  kind: "idle",
  accessMode: "public_byok",
  hostedMode: "byok",
  publicOpenAICompatible: false,
};

const INITIAL_LIVE: LiveState = {
  runID: "",
  status: "idle",
  message: "",
  events: [],
  artifactItems: [],
};

const INITIAL_QUALIFICATION: QualificationState = {
  status: "idle",
  message: "",
  result: null,
};

export default function App({ runtimeConfig }: { runtimeConfig: RuntimeConfig }) {
  const [activeTab, setActiveTab] = useState<ReportTab>("checks");
  const [apiBase, setApiBase] = useState(() => configuredApiBase(runtimeConfig));
  const [backend, setBackend] = useState<BackendState>(INITIAL_BACKEND);
  const [artifacts, setArtifacts] = useState<ReportArtifacts | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState<LiveState>(INITIAL_LIVE);
  const [qualification, setQualification] = useState<QualificationState>(INITIAL_QUALIFICATION);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    updateBackendHealth(apiBase).catch(showError);
    return () => {
      stopLivePolling();
    };
  }, []);

  async function updateBackendHealth(base = apiBase) {
    const cleanBase = cleanApiBase(base);
    if (!cleanBase) {
      setBackend(INITIAL_BACKEND);
      return;
    }

    try {
      const health = await fetchHealth(cleanBase);
      setBackend({
        online: true,
        label: "Online",
        kind: "online",
        accessMode: health.access_mode || "public_byok",
        hostedMode: health.hosted_mode || "byok",
        publicOpenAICompatible: Boolean(health.public_openai_compatible),
        maxCandidateTextChars: health.max_candidate_text_chars,
        maxGenerationPromptChars: health.max_generation_prompt_chars,
        localModel: health.local_model,
      });
    } catch {
      setBackend({
        online: false,
        label: "Unavailable",
        kind: "offline",
        accessMode: "public_byok",
        hostedMode: "byok",
        publicOpenAICompatible: false,
      });
    }
  }

  function showLiveWorkspace() {
    setError("");
    setArtifacts(null);
  }

  async function createLiveRun(base: string, inviteToken: string, payload: RunPayload) {
    const cleanBase = cleanApiBase(base);
    if (!cleanBase) {
      throw new Error("A configured MealCheck service is required to create a report.");
    }
    if (backend.accessMode === "invite_required" && !inviteToken.trim()) {
      throw new Error("Access code is required to create a report.");
    }

    stopLivePolling();
    setApiBase(cleanBase);
    setArtifacts(null);
    setActiveTab("checks");
    setError("");
    setLive({
      runID: "",
      status: "queued",
      message: "Creating report.",
      events: [],
      artifactItems: [],
    });

    const created = await createRun(cleanBase, inviteToken, payload);
    setLive((current) => ({
      ...current,
      runID: created.run_id,
      status: created.status,
      message: "Report queued.",
    }));
    startLivePolling(cleanBase, created.run_id);
  }

  async function qualifyCandidate(base: string, inviteToken: string, payload: QualifyMealPlanPayload) {
    const cleanBase = cleanApiBase(base);
    if (!cleanBase) {
      throw new Error("A configured MealCheck service is required to check eligibility.");
    }
    if (backend.accessMode === "invite_required" && !inviteToken.trim()) {
      throw new Error("Access code is required to check eligibility.");
    }

    setApiBase(cleanBase);
    setError("");
    setQualification({
      status: "checking",
      message: "Checking meal plan eligibility.",
      result: null,
    });
    try {
      const response = await qualifyMealPlan(cleanBase, inviteToken, payload);
      setQualification({
        status: "completed",
        message: qualificationMessage(response.qualification),
        result: response.qualification,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setQualification({
        status: "failed",
        message,
        result: null,
      });
      throw error;
    }
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
      message: run.error || run.summary || `Report ${run.status}.`,
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
    setLive((current) => ({ ...current, artifactItems }));
    setArtifacts(nextArtifacts);
  }

  async function deleteLiveRun() {
    if (!live.runID) {
      showError(new Error("No report is selected."));
      return;
    }

    await deleteRun(apiBase, live.runID);
    stopLivePolling();
    setArtifacts(null);
    setLive((current) => ({
      ...current,
      status: "deleted",
      message: "Report deleted.",
      artifactItems: [],
    }));
  }

  function showError(errorLike: unknown) {
    const message = errorLike instanceof Error ? errorLike.message : String(errorLike);
    setError(message);
    setLive((current) => ({ ...current, status: "failed", message }));
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand-cluster">
          <BrandMark />
          <div className="brand-block">
            <p className="eyebrow">Live verification</p>
            <h1>MealCheck</h1>
            <p className="brand-subtitle">Evidence-backed meal plan verification</p>
          </div>
        </div>
      </header>

      <main className="main-layout">
        <Sidebar onLive={showLiveWorkspace} />

        <section className="workspace" aria-live="polite">
          <LiveSummary apiBase={apiBase} backend={backend} live={live} />

          <LiveWorkspace
            apiBase={apiBase}
            backend={backend}
            live={live}
            qualification={qualification}
            onCreateRun={createLiveRun}
            onQualify={qualifyCandidate}
            onDeleteRun={deleteLiveRun}
            onError={showError}
          />

          {error ? <section className="panel error-state" role="alert">{error}</section> : null}

          {artifacts ? (
            <ReportSurface
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              artifacts={artifacts}
            />
          ) : null}
        </section>
      </main>

      <footer className="site-footer">
        <a href="https://github.com/chranama/MealCheck" target="_blank" rel="noreferrer">
          View MealCheck on GitHub
        </a>
      </footer>
    </div>
  );
}

function qualificationMessage(result: MealPlanQualificationResult): string {
  if (result.status === "eligible_for_verification" || result.status === "eligible_with_unresolved_items") {
    return result.reason || "Candidate text qualifies for verification.";
  }
  return result.reason || "Candidate text is not ready for verification.";
}
