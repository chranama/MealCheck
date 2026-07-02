import { useEffect, useState } from "react";
import type { SubmitEvent } from "react";
import {
  DEFAULT_CANDIDATE_TEXT,
  DEFAULT_GENERATION_PROMPT,
  DEFAULT_PROVIDER,
  DEFAULT_SETTINGS,
} from "../../constants";
import { cleanApiBase } from "../../lib/api";
import { buildLocalModelRunPayload, buildQualificationPayload, buildRunPayload } from "../../lib/payload";
import { reportCreationPreflight } from "../../lib/report_preflight";
import { recoveryFromQualification } from "../../lib/recovery";
import type {
  BackendState,
  GenerationMode,
  LiveState,
  NormalizedPlanCorrectionPayload,
  NormalizedPlanReviewState,
  ProviderConfig,
  QualificationState,
  QualifyMealPlanPayload,
  RunPayload,
  SettingsDraft,
} from "../../types";
import { Field } from "../common/FormControls";
import { CandidateTextForm } from "./CandidateTextForm";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { ProviderForm } from "./ProviderForm";
import { RunActionStrip } from "./RunActionStrip";
import { RunStatusPanel } from "./RunStatusPanel";
import { VerificationSettingsForm } from "./VerificationSettingsForm";
import { NormalizedPlanReviewPanel } from "./NormalizedPlanReviewPanel";

export function LiveWorkspace({
  apiBase,
  backend,
  live,
  qualification,
  review,
  onCreateRun,
  onQualify,
  onDeleteRun,
  onConfirmReview,
  onCorrectReviewRow,
  onRejectReview,
  onRequestRewrite,
  onError,
}: {
  apiBase: string;
  backend: BackendState;
  live: LiveState;
  qualification: QualificationState;
  review: NormalizedPlanReviewState;
  onCreateRun: (base: string, inviteToken: string, payload: RunPayload) => Promise<void>;
  onQualify: (base: string, inviteToken: string, payload: QualifyMealPlanPayload) => Promise<void>;
  onDeleteRun: () => Promise<void>;
  onConfirmReview: () => Promise<void>;
  onCorrectReviewRow: (payload: NormalizedPlanCorrectionPayload) => Promise<void>;
  onRejectReview: () => Promise<void>;
  onRequestRewrite: () => Promise<void>;
  onError: (error: unknown) => void;
}) {
  const [inviteToken, setInviteToken] = useState("");
  const [settings, setSettings] = useState<SettingsDraft>({
    nutrition_targets: DEFAULT_SETTINGS.nutrition_targets,
    verification_constraints: {
      ...DEFAULT_SETTINGS.verification_constraints,
      allergies: DEFAULT_SETTINGS.verification_constraints.allergies.join(", "),
      excluded_foods: DEFAULT_SETTINGS.verification_constraints.excluded_foods.join(", "),
    },
  });
  const [candidateText, setCandidateText] = useState(DEFAULT_CANDIDATE_TEXT);
  const [mode, setMode] = useState<GenerationMode>("profile_generation");
  const [provider, setProvider] = useState<ProviderConfig>(DEFAULT_PROVIDER);
  const [generationPrompt, setGenerationPrompt] = useState(DEFAULT_GENERATION_PROMPT);
  const [repairJSON, setRepairJSON] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const cleanBase = cleanApiBase(apiBase);
  const inviteRequired = backend.accessMode === "invite_required";
  const isLocalModelHosted = backend.hostedMode === "local_model";
  const candidateTextLimit = isLocalModelHosted
    ? backend.localModel?.max_input_chars || backend.maxCandidateTextChars
    : backend.maxCandidateTextChars;
  const sourceItemLimit = isLocalModelHosted ? backend.localModel?.max_source_items : undefined;
  const candidateTextLength = candidateText.length;
  const candidateTextTooLong = Boolean(candidateTextLimit && candidateTextLength > candidateTextLimit);
  const allowOpenAICompatible = backend.accessMode !== "public_byok" || backend.publicOpenAICompatible;
  const canDeleteRun = Boolean(live.runID && live.status !== "deleted");
  const preflight = reportCreationPreflight({
    apiBase: cleanBase,
    backend,
    candidateText,
    candidateTextTooLong,
    inviteToken,
    liveStatus: live.status,
    qualificationStatus: qualification.status,
    reviewStatus: review.status,
  });
  const canQualify = preflight.canQualify;
  const canCreateRun = preflight.canCreate;
  const qualificationRecovery = qualification.result ? recoveryFromQualification(qualification.result) : null;
  const inlineMealPlanRecovery = qualificationRecovery && qualification.result?.status !== "eligible_for_verification"
    ? qualificationRecovery
    : null;
  const createFeedback = preflight.message;

  useEffect(() => {
    if (!allowOpenAICompatible && provider.type === "openai_compatible") {
      setProvider((current) => ({ ...current, type: "openai", base_url: "" }));
    }
  }, [allowOpenAICompatible, provider.type]);

  async function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const payload = isLocalModelHosted
        ? buildLocalModelRunPayload({
          text: candidateText,
          settings,
        })
        : buildRunPayload({
          mode,
          settings,
          provider,
          generationPrompt,
          repairJSON,
        });
      await onCreateRun(cleanBase, inviteToken, payload);
      if (!isLocalModelHosted) {
        setProvider((current) => ({ ...current, api_key: "" }));
      }
    } catch (error) {
      onError(error);
    }
  }

  async function handleQualify() {
    try {
      const payload = buildQualificationPayload({
        text: candidateText,
        settings,
        provider,
      });
      await onQualify(cleanBase, inviteToken, payload);
      if (payload.provider) {
        setProvider((current) => ({ ...current, api_key: "" }));
      }
    } catch (error) {
      onError(error);
    }
  }

  return (
    <section className="live-workspace" id="live-workspace">
      <section className="panel live-panel">
        <RunActionStrip
          canCreateRun={canCreateRun}
          canQualify={canQualify}
          canDeleteRun={canDeleteRun}
          createFeedback={createFeedback}
          qualification={qualification}
          showQualify={!isLocalModelHosted}
          onQualify={handleQualify}
          onDelete={() => setConfirmDelete(true)}
        />

        <form id="live-run-form" onSubmit={handleSubmit}>
          {inviteRequired ? (
            <fieldset>
              <legend>Access</legend>
              <div className="form-grid">
                <Field label="Access code">
                  <input autoComplete="off" value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} type="password" />
                </Field>
              </div>
            </fieldset>
          ) : null}

          <fieldset>
            <legend>Meal Plan Text</legend>
            <CandidateTextForm
              candidateText={candidateText}
              limit={candidateTextLimit}
              recovery={inlineMealPlanRecovery}
              setCandidateText={setCandidateText}
              sourceItemLimit={sourceItemLimit}
            />
          </fieldset>

          {isLocalModelHosted ? null : (
            <fieldset>
              <legend>Model Provider</legend>
              <ProviderForm
                provider={provider}
                setProvider={setProvider}
                repairJSON={repairJSON}
                setRepairJSON={setRepairJSON}
                mode={mode}
                setMode={setMode}
                allowOpenAICompatible={allowOpenAICompatible}
                generationPrompt={generationPrompt}
                setGenerationPrompt={setGenerationPrompt}
              />
            </fieldset>
          )}

          <VerificationSettingsForm settings={settings} setSettings={setSettings} />
        </form>
      </section>

      <RunStatusPanel live={live} qualification={qualification} />

      <NormalizedPlanReviewPanel
        review={review}
        onConfirm={() => onConfirmReview().catch(onError)}
        onCorrectRow={(payload) => onCorrectReviewRow(payload).catch(onError)}
        onReject={() => onRejectReview().catch(onError)}
        onRequestRewrite={() => onRequestRewrite().catch(onError)}
      />

      {confirmDelete ? (
        <ConfirmDeleteDialog
          runID={live.runID}
          onCancel={() => setConfirmDelete(false)}
          onConfirm={() => {
            setConfirmDelete(false);
            onDeleteRun().catch(onError);
          }}
        />
      ) : null}
    </section>
  );
}
