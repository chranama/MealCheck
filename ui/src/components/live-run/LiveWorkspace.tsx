import { useState } from "react";
import type { Dispatch, FormEvent, SetStateAction } from "react";
import {
  DEFAULT_CANDIDATE_TEXT,
  DEFAULT_CONSTRAINTS,
  DEFAULT_GENERATION_PROMPT,
  DEFAULT_PROFILE,
  DEFAULT_PROVIDER,
  PROVIDER_OPTIONS,
} from "../../constants";
import { cleanApiBase } from "../../lib/api";
import { readableID } from "../../lib/format";
import { buildQualificationPayload, buildRunPayload } from "../../lib/payload";
import type {
  BackendState,
  ConstraintsDraft,
  GenerationMode,
  LiveState,
  MealPlanQualificationResult,
  Profile,
  ProviderConfig,
  ProviderType,
  QualificationState,
  QualifyMealPlanPayload,
  RunPayload,
} from "../../types";
import { Field, NumberInput } from "../common/FormControls";

export function LiveWorkspace({
  apiBase,
  backend,
  live,
  qualification,
  onCreateRun,
  onQualify,
  onDeleteRun,
  onError,
}: {
  apiBase: string;
  backend: BackendState;
  live: LiveState;
  qualification: QualificationState;
  onCreateRun: (base: string, inviteToken: string, payload: RunPayload) => Promise<void>;
  onQualify: (base: string, inviteToken: string, payload: QualifyMealPlanPayload) => Promise<void>;
  onDeleteRun: () => Promise<void>;
  onError: (error: unknown) => void;
}) {
  const [inviteToken, setInviteToken] = useState("");
  const [profile, setProfile] = useState<Profile>(DEFAULT_PROFILE);
  const [constraints, setConstraints] = useState<ConstraintsDraft>({
    ...DEFAULT_CONSTRAINTS,
    allergies: DEFAULT_CONSTRAINTS.allergies.join(", "),
    excluded_foods: DEFAULT_CONSTRAINTS.excluded_foods.join(", "),
  });
  const [candidateText, setCandidateText] = useState(DEFAULT_CANDIDATE_TEXT);
  const [mode, setMode] = useState<GenerationMode>("profile_generation");
  const [provider, setProvider] = useState<ProviderConfig>(DEFAULT_PROVIDER);
  const [generationPrompt, setGenerationPrompt] = useState(DEFAULT_GENERATION_PROMPT);
  const [repairJSON, setRepairJSON] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const cleanBase = cleanApiBase(apiBase);
  const isSubmitting = live.status === "queued" || live.status === "running";
  const isCheckingQualification = qualification.status === "checking";
  const healthBlocksSubmit = backend.kind === "offline" && Boolean(cleanBase);
  const canDeleteRun = Boolean(live.runID && live.status !== "deleted");
  const baseActionsEnabled = Boolean(cleanBase && inviteToken.trim()) && !isSubmitting && !isCheckingQualification && !healthBlocksSubmit;
  const canQualify = baseActionsEnabled && Boolean(candidateText.trim());
  const canCreateRun = baseActionsEnabled;
  const createFeedback = createRunFeedback({
    apiBase: cleanBase,
    hasInviteToken: Boolean(inviteToken.trim()),
    healthBlocksSubmit,
    isBusy: isSubmitting || isCheckingQualification,
  });

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const payload = buildRunPayload({
        mode,
        profile,
        constraints,
        provider,
        generationPrompt,
        repairJSON,
      });
      await onCreateRun(cleanBase, inviteToken, payload);
      setProvider((current) => ({ ...current, api_key: "" }));
    } catch (error) {
      onError(error);
    }
  }

  async function handleQualify() {
    try {
      const payload = buildQualificationPayload({
        text: candidateText,
        profile,
        constraints,
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
          live={live}
          qualification={qualification}
          onQualify={handleQualify}
          onDelete={() => setConfirmDelete(true)}
        />

        <form id="live-run-form" onSubmit={handleSubmit}>
          <fieldset>
            <legend>Access</legend>
            <div className="form-grid">
              <Field label="Access code">
                <input autoComplete="off" value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} type="password" />
              </Field>
            </div>
          </fieldset>

          <fieldset>
            <legend>Meal Plan Text</legend>
            <CandidateTextForm candidateText={candidateText} setCandidateText={setCandidateText} />
          </fieldset>

          <fieldset>
            <legend>Model Provider</legend>
            <ProviderForm
              provider={provider}
              setProvider={setProvider}
              repairJSON={repairJSON}
              setRepairJSON={setRepairJSON}
              mode={mode}
              setMode={setMode}
              generationPrompt={generationPrompt}
              setGenerationPrompt={setGenerationPrompt}
            />
          </fieldset>

          <section className="verification-settings-section" aria-label="Verification settings">
            <details className="advanced-section verification-settings">
              <summary>
                <span>Verification Settings</span>
                <small>Targets and checks</small>
              </summary>
              <div className="verification-settings-body">
                <ProfileForm profile={profile} setProfile={setProfile} />
                <ConstraintsForm constraints={constraints} setConstraints={setConstraints} />
              </div>
            </details>
          </section>
        </form>
      </section>

      <RunStatusPanel live={live} qualification={qualification} />

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

function RunActionStrip({
  canCreateRun,
  canQualify,
  canDeleteRun,
  createFeedback,
  live,
  qualification,
  onQualify,
  onDelete,
}: {
  canCreateRun: boolean;
  canQualify: boolean;
  canDeleteRun: boolean;
  createFeedback: string;
  live: LiveState;
  qualification: QualificationState;
  onQualify: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="live-action-strip" aria-label="Report action status">
      <div className="action-strip-state">
        <span className={`status-pill status-pill--${runPillTone(live.status)}`}>{live.status === "idle" ? "Ready" : readableID(live.status)}</span>
        <p className="submit-feedback" id="create-run-feedback" role="status">{createFeedback}</p>
      </div>
      <div className="action-strip-actions">
        <button
          className="action-button action-button--secondary"
          disabled={!canQualify}
          id="qualify-button"
          type="button"
          onClick={onQualify}
        >
          {qualification.status === "checking" ? "Checking" : "Check Eligibility"}
        </button>
        <button
          aria-describedby="create-run-feedback"
          className="action-button action-button--primary"
          disabled={!canCreateRun}
          form="live-run-form"
          id="create-run-button"
          type="submit"
        >
          Create Report
        </button>
        <button
          className="action-button action-button--danger"
          disabled={!canDeleteRun}
          id="delete-run-button"
          type="button"
          onClick={onDelete}
        >
          Delete Report
        </button>
      </div>
    </section>
  );
}

function createRunFeedback({
  apiBase,
  hasInviteToken,
  healthBlocksSubmit,
  isBusy,
}: {
  apiBase: string;
  hasInviteToken: boolean;
  healthBlocksSubmit: boolean;
  isBusy: boolean;
}) {
  if (isBusy) return "Request in progress.";
  if (!apiBase) return "Report creation needs a configured MealCheck service.";
  if (!hasInviteToken) return "Enter your access code to start.";
  if (healthBlocksSubmit) return "Service is unavailable right now.";
  return "Ready to check eligibility or create a MealCheck report.";
}

function runPillTone(status: LiveState["status"]): string {
  if (status === "completed") return "pass";
  if (status === "failed" || status === "deleted") return "block";
  if (status === "queued" || status === "running") return "warn";
  return "info";
}

function ProfileForm({ profile, setProfile }: { profile: Profile; setProfile: Dispatch<SetStateAction<Profile>> }) {
  function update<K extends keyof Profile>(field: K, value: Profile[K]) {
    setProfile((current) => ({ ...current, [field]: value }));
  }

  return (
    <fieldset>
      <legend>Nutrition Targets</legend>
      <div className="form-grid">
        <Field label="Calories"><NumberInput value={profile.calorie_target_kcal} min={1} max={8000} step={1} onChange={(value) => update("calorie_target_kcal", value)} /></Field>
        <Field label="Protein g"><NumberInput value={profile.protein_target_g} min={1} max={400} step={1} onChange={(value) => update("protein_target_g", value)} /></Field>
      </div>
    </fieldset>
  );
}

function ConstraintsForm({ constraints, setConstraints }: { constraints: ConstraintsDraft; setConstraints: Dispatch<SetStateAction<ConstraintsDraft>> }) {
  function update<K extends keyof ConstraintsDraft>(field: K, value: ConstraintsDraft[K]) {
    setConstraints((current) => ({ ...current, [field]: value }));
  }

  return (
    <fieldset>
      <legend>Constraints</legend>
      <div className="form-grid">
        <Field label="Days"><NumberInput value={constraints.days} min={1} max={7} step={1} onChange={(value) => update("days", value)} /></Field>
        <Field label="Meals/day"><NumberInput value={constraints.meals_per_day} min={1} max={6} step={1} onChange={(value) => update("meals_per_day", value)} /></Field>
        <Field label="Allergies"><input value={constraints.allergies} onChange={(event) => update("allergies", event.target.value)} type="text" /></Field>
        <Field label="Excluded foods"><input value={constraints.excluded_foods} onChange={(event) => update("excluded_foods", event.target.value)} type="text" /></Field>
      </div>
      <details className="advanced-section">
        <summary>
          <span>Advanced constraints</span>
          <small>Thresholds and policy checks</small>
        </summary>
        <div className="form-grid">
          <Field label="Sodium mg/day"><NumberInput value={constraints.max_sodium_mg_per_day} min={1} max={10000} step={1} onChange={(value) => update("max_sodium_mg_per_day", value)} /></Field>
          <Field label="Added sugar g/meal"><NumberInput value={constraints.max_added_sugar_g_per_meal} min={0} max={200} step={0.1} onChange={(value) => update("max_added_sugar_g_per_meal", value)} /></Field>
          <Field label="Sat fat % kcal"><NumberInput value={constraints.max_saturated_fat_pct_calories} min={0} max={100} step={0.1} onChange={(value) => update("max_saturated_fat_pct_calories", value)} /></Field>
          <Field label="Calorie tolerance %"><NumberInput value={constraints.calorie_tolerance_pct} min={0} max={100} step={0.1} onChange={(value) => update("calorie_tolerance_pct", value)} /></Field>
        </div>
        <div className="check-row">
          <label><input checked={constraints.requires_prep_safety_notes} onChange={(event) => update("requires_prep_safety_notes", event.target.checked)} type="checkbox" />Prep safety notes required</label>
        </div>
      </details>
    </fieldset>
  );
}

function CandidateTextForm({
  candidateText,
  setCandidateText,
}: {
  candidateText: string;
  setCandidateText: (value: string) => void;
}) {
  return (
    <section className="mode-section" id="candidate-text-section">
      <Field label="Meal plan text">
        <textarea value={candidateText} rows={7} onChange={(event) => setCandidateText(event.target.value)} />
      </Field>
    </section>
  );
}

function ModeButton({ mode, activeMode, setMode, label }: { mode: GenerationMode; activeMode: GenerationMode; setMode: (mode: GenerationMode) => void; label: string }) {
  return (
    <button className={`mode-button${activeMode === mode ? " is-active" : ""}`} data-mode={mode} type="button" onClick={() => setMode(mode)}>
      {label}
    </button>
  );
}

function ProviderForm({
  provider,
  setProvider,
  repairJSON,
  setRepairJSON,
  mode,
  setMode,
  generationPrompt,
  setGenerationPrompt,
}: {
  provider: ProviderConfig;
  setProvider: Dispatch<SetStateAction<ProviderConfig>>;
  repairJSON: boolean;
  setRepairJSON: (value: boolean) => void;
  mode: GenerationMode;
  setMode: (mode: GenerationMode) => void;
  generationPrompt: string;
  setGenerationPrompt: (value: string) => void;
}) {
  function update<K extends keyof ProviderConfig>(field: K, value: ProviderConfig[K]) {
    setProvider((current) => ({ ...current, [field]: value }));
  }
  function updateProviderType(type: ProviderType) {
    setProvider((current) => ({
      ...current,
      type,
      base_url: type === "openai_compatible" ? current.base_url : "",
    }));
  }

  const selectedProvider = PROVIDER_OPTIONS.find((option) => option.type === provider.type) ?? PROVIDER_OPTIONS[0];

  return (
    <section className="mode-section" id="provider-section">
      <div className="notice">
        <strong>Model provider disclosure</strong>
        <p>MealCheck sends this key to the backend for the requested provider call. Use temporary, scoped, budget-limited keys; custom OpenAI-compatible endpoints receive the key too. For maximum control, run MealCheck locally from the repo.</p>
      </div>
      <div className="form-grid">
        <Field label="Provider">
          <select value={provider.type} onChange={(event) => updateProviderType(event.target.value as ProviderType)}>
            {PROVIDER_OPTIONS.map((option) => (
              <option key={option.type} value={option.type}>{option.label}</option>
            ))}
          </select>
        </Field>
        {provider.type === "openai_compatible" ? (
          <Field label="Base URL"><input placeholder="https://api.openai.com/v1" value={provider.base_url} onChange={(event) => update("base_url", event.target.value)} type="text" /></Field>
        ) : null}
        <Field label="Model"><input placeholder={selectedProvider.modelHint} value={provider.model} onChange={(event) => update("model", event.target.value)} type="text" /></Field>
        <Field label="API key"><input autoComplete="off" value={provider.api_key} onChange={(event) => update("api_key", event.target.value)} type="password" /></Field>
      </div>
      <section className="generation-mode-section">
        <div className="segmented" role="group" aria-label="Generation mode">
          <ModeButton mode="profile_generation" activeMode={mode} setMode={setMode} label="Targets" />
          <ModeButton mode="prompt_generation" activeMode={mode} setMode={setMode} label="Prompt" />
        </div>
      </section>
      {mode === "prompt_generation" ? (
        <section id="prompt-section">
          <Field label="Prompt"><textarea value={generationPrompt} rows={4} onChange={(event) => setGenerationPrompt(event.target.value)} /></Field>
        </section>
      ) : null}
      <div className="check-row">
        <label><input checked={repairJSON} onChange={(event) => setRepairJSON(event.target.checked)} type="checkbox" />Allow one JSON repair attempt</label>
      </div>
    </section>
  );
}

function RunStatusPanel({ live, qualification }: { live: LiveState; qualification: QualificationState }) {
  const hasEvents = live.events.length > 0;
  const hasReport = live.status !== "deleted" && live.artifactItems.length > 0;
  return (
    <section className="panel live-panel results-panel" id="live-status-panel">
      <div className="panel-heading">
        <h2>Results</h2>
        <span className={`status-pill status-pill--${runPillTone(live.status)}`}>{live.status === "idle" ? "Not started" : readableID(live.status)}</span>
      </div>
      <div className="status-stack">
        <p className="summary-text">{live.message || "Your report will appear here after you create a check."}</p>
        <QualificationNotice qualification={qualification} />
        {hasReport ? (
          <div className="notice notice--pass live-report-ready" role="status">
            <strong>Report ready</strong>
            <p>Decision details are available below.</p>
          </div>
        ) : null}
      </div>
      {hasEvents ? (
        <details className="activity-details">
          <summary>Activity details</summary>
          <div className="event-list">
            {live.events.map((event, index) => (
              <div className="event-row" key={`${event.type}-${index}`}>
                <strong>{readableID(event.type)}</strong>
                <span>{event.message}</span>
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </section>
  );
}

function QualificationNotice({ qualification }: { qualification: QualificationState }) {
  if (qualification.status === "idle") return null;
  const result = qualification.result || null;
  return (
    <div className={`notice notice--${qualificationTone(result, qualification.status)}`} role="status">
      <strong>Qualification</strong>
      <p>{qualification.message}</p>
      {result ? (
        <dl className="qualification-facts">
          <div>
            <dt>Status</dt>
            <dd>{readableID(result.status)}</dd>
          </div>
          <div>
            <dt>Provider</dt>
            <dd>{result.provider_used ? "Used" : "Not used"}</dd>
          </div>
          {normalizedPlanSummary(result) ? (
            <div>
              <dt>Plan</dt>
              <dd>{normalizedPlanSummary(result)}</dd>
            </div>
          ) : null}
          {result.missing_fields?.length ? (
            <div>
              <dt>Missing</dt>
              <dd>{result.missing_fields.join(", ")}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </div>
  );
}

function qualificationTone(result: MealPlanQualificationResult | null, status: QualificationState["status"]): string {
  if (status === "failed") return "block";
  if (!result) return "info";
  if (result.status === "eligible_for_verification" || result.status === "eligible_with_unresolved_items") return "pass";
  if (result.status === "not_meal_plan") return "block";
  return "warn";
}

function normalizedPlanSummary(result: MealPlanQualificationResult): string {
  const plan = result.normalized_plan;
  if (!plan) return "";
  const dayCount = plan.days.length;
  const mealCount = plan.days.reduce((sum, day) => sum + day.meals.length, 0);
  const itemCount = plan.days.reduce((sum, day) => sum + day.meals.reduce((mealSum, meal) => mealSum + meal.items.length, 0), 0);
  return `${dayCount} day${dayCount === 1 ? "" : "s"}, ${mealCount} meal${mealCount === 1 ? "" : "s"}, ${itemCount} item${itemCount === 1 ? "" : "s"}`;
}

function ConfirmDeleteDialog({
  runID,
  onCancel,
  onConfirm,
}: {
  runID: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-labelledby="delete-dialog-title" aria-modal="true" className="confirm-dialog" role="dialog">
        <h2 id="delete-dialog-title">Delete report?</h2>
        <p>This removes the report and its files for {runID}.</p>
        <div className="form-actions">
          <button className="action-button action-button--ghost" type="button" onClick={onCancel}>Cancel</button>
          <button className="action-button action-button--danger" type="button" onClick={onConfirm}>Delete Report</button>
        </div>
      </section>
    </div>
  );
}
