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
  qualificationFromApiError,
} from "./lib/api";
import { recoveryFromError } from "./lib/recovery";
import type { RecoveryNotice } from "./lib/recovery";
import { configuredApiBase } from "./lib/runtime_config";
import { LiveWorkspace } from "./components/live-run/LiveWorkspace";
import { ReportSurface } from "./components/report/ReportSurface";
import { BrandMark } from "./components/brand/BrandMark";
import { RecoveryNoticeView } from "./components/common/RecoveryNotice";
import { LiveSummary, SiteFooter } from "./components/shell/Shell";
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
  progress: null,
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
  const [recovery, setRecovery] = useState<RecoveryNotice | null>(null);
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
    setRecovery(null);
    setQualification(INITIAL_QUALIFICATION);
    setLive({
      runID: "",
      status: "queued",
      message: "Creating report.",
      events: [],
      artifactItems: [],
      progress: null,
    });

    let created: Awaited<ReturnType<typeof createRun>>;
    try {
      created = await createRun(cleanBase, inviteToken, payload);
    } catch (error) {
      const qualificationResult = qualificationFromApiError(error);
      if (qualificationResult) {
        const message = qualificationMessage(qualificationResult);
        setQualification({
          status: "completed",
          message,
          result: qualificationResult,
        });
        setLive({
          runID: "",
          status: "failed",
          message: "MealCheck could not start a report because this input is not ready for verification.",
          events: [],
          artifactItems: [],
          progress: null,
        });
        return;
      }
      throw error;
    }
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
    setRecovery(null);
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
      const notice = recoveryFromError(error);
      setQualification({
        status: "failed",
        message: notice.title,
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
    const progress = runDoc.progress || null;
    setLive((current) => ({
      ...current,
      runID,
      status: run.status,
      message: progress?.message || run.error || run.summary || `Report ${run.status}.`,
      events,
      progress,
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
    const notice = recoveryFromError(errorLike);
    setRecovery(notice);
    setLive((current) => ({ ...current, status: "failed", message: notice.title }));
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
        <section className="workspace" aria-live="polite">
          <LiveSummary live={live} />

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

          {recovery ? (
            <section className="panel error-state">
              <RecoveryNoticeView notice={recovery} role="alert" />
            </section>
          ) : null}

          {artifacts ? (
            <ReportSurface
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              artifacts={artifacts}
            />
          ) : null}
        </section>
      </main>

      <SiteFooter />
    </div>
  );
}

function qualificationMessage(result: MealPlanQualificationResult): string {
  if (result.status === "eligible_for_verification" || result.status === "eligible_with_unresolved_items") {
    return result.reason || "Candidate text qualifies for verification.";
  }
  return result.reason || "Candidate text is not ready for verification.";
}
