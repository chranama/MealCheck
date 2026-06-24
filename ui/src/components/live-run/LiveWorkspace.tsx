import { useEffect, useState } from "react";
import type { Dispatch, FormEvent, SetStateAction } from "react";
import {
  DEFAULT_CANDIDATE_TEXT,
  DEFAULT_GENERATION_PROMPT,
  DEFAULT_PROVIDER,
  DEFAULT_SETTINGS,
  PROVIDER_OPTIONS,
} from "../../constants";
import { cleanApiBase } from "../../lib/api";
import { readableID } from "../../lib/format";
import { buildLocalModelRunPayload, buildQualificationPayload, buildRunPayload } from "../../lib/payload";
import { recoveryFromQualification, recoveryFromRunFailure } from "../../lib/recovery";
import type { RecoveryNotice } from "../../lib/recovery";
import type {
  BackendState,
  GenerationMode,
  LiveState,
  MealPlanQualificationResult,
  ProviderConfig,
  ProviderType,
  QualificationState,
  QualifyMealPlanPayload,
  RunPayload,
  SettingsDraft,
} from "../../types";
import { RecoveryNoticeView } from "../common/RecoveryNotice";
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
  const localModelReady = Boolean(backend.localModel?.enabled && backend.localModel?.ready);
  const localModelUnavailable = isLocalModelHosted && !localModelReady;
  const candidateTextLimit = isLocalModelHosted
    ? backend.localModel?.max_input_chars || backend.maxCandidateTextChars
    : backend.maxCandidateTextChars;
  const candidateTextLength = candidateText.length;
  const candidateTextTooLong = Boolean(candidateTextLimit && candidateTextLength > candidateTextLimit);
  const allowOpenAICompatible = backend.accessMode !== "public_byok" || backend.publicOpenAICompatible;
  const isSubmitting = live.status === "queued" || live.status === "running";
  const isCheckingQualification = qualification.status === "checking";
  const healthBlocksSubmit = backend.kind === "offline" && Boolean(cleanBase);
  const canDeleteRun = Boolean(live.runID && live.status !== "deleted");
  const baseActionsEnabled = Boolean(cleanBase) && (!inviteRequired || Boolean(inviteToken.trim())) && !isSubmitting && !isCheckingQualification && !healthBlocksSubmit;
  const canQualify = !isLocalModelHosted && baseActionsEnabled && Boolean(candidateText.trim());
  const canCreateRun = baseActionsEnabled && (
    isLocalModelHosted
      ? Boolean(candidateText.trim()) && !candidateTextTooLong && !localModelUnavailable
      : true
  );
  const qualificationRecovery = qualification.result ? recoveryFromQualification(qualification.result) : null;
  const inlineMealPlanRecovery = qualificationRecovery && qualification.result?.status !== "eligible_for_verification"
    ? qualificationRecovery
    : null;
  const createFeedback = createRunFeedback({
    apiBase: cleanBase,
    candidateTextTooLong,
    hasInviteToken: Boolean(inviteToken.trim()),
    inviteRequired,
    healthBlocksSubmit,
    isLocalModelHosted,
    localModelUnavailable,
    needsCandidateText: isLocalModelHosted && !candidateText.trim(),
    isBusy: isSubmitting || isCheckingQualification,
  });

  useEffect(() => {
    if (!allowOpenAICompatible && provider.type === "openai_compatible") {
      setProvider((current) => ({ ...current, type: "openai", base_url: "" }));
    }
  }, [allowOpenAICompatible, provider.type]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
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
          live={live}
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
            />
          </fieldset>

          {isLocalModelHosted ? (
            <LocalModelPanel backend={backend} />
          ) : (
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

          <section className="verification-settings-section" aria-label="Verification settings">
            <details className="advanced-section verification-settings">
              <summary>
                <span>Verification Settings</span>
                <small>Targets and checks</small>
              </summary>
              <div className="verification-settings-body">
                <NutritionTargetsForm settings={settings} setSettings={setSettings} />
                <ConstraintsForm settings={settings} setSettings={setSettings} />
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
  showQualify,
  onQualify,
  onDelete,
}: {
  canCreateRun: boolean;
  canQualify: boolean;
  canDeleteRun: boolean;
  createFeedback: string;
  live: LiveState;
  qualification: QualificationState;
  showQualify: boolean;
  onQualify: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="live-action-strip" aria-label="Report actions">
      <div className="action-strip-state">
        <p className="submit-feedback" id="create-run-feedback" role="status">{createFeedback}</p>
      </div>
      <div className="action-strip-actions">
        {showQualify ? (
          <button
            className="action-button action-button--secondary"
            disabled={!canQualify}
            id="qualify-button"
            type="button"
            onClick={onQualify}
          >
            {qualification.status === "checking" ? "Checking" : "Check Eligibility"}
          </button>
        ) : null}
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
  candidateTextTooLong,
  hasInviteToken,
  inviteRequired,
  healthBlocksSubmit,
  isLocalModelHosted,
  localModelUnavailable,
  needsCandidateText,
  isBusy,
}: {
  apiBase: string;
  candidateTextTooLong: boolean;
  hasInviteToken: boolean;
  inviteRequired: boolean;
  healthBlocksSubmit: boolean;
  isLocalModelHosted: boolean;
  localModelUnavailable: boolean;
  needsCandidateText: boolean;
  isBusy: boolean;
}) {
  if (isBusy) return "Request in progress.";
  if (!apiBase) return "Report creation needs a configured MealCheck service.";
  if (inviteRequired && !hasInviteToken) return "Enter your access code to start.";
  if (healthBlocksSubmit) return "Service is unavailable right now.";
  if (localModelUnavailable) return "Local model is unavailable right now.";
  if (needsCandidateText) return "Enter a meal plan to create a report.";
  if (candidateTextTooLong) return "Meal plan text is over the local model limit.";
  if (isLocalModelHosted) return "Create a local-model MealCheck report.";
  return "Check eligibility or create a MealCheck report.";
}

function NutritionTargetsForm({ settings, setSettings }: { settings: SettingsDraft; setSettings: Dispatch<SetStateAction<SettingsDraft>> }) {
  function update<K extends keyof SettingsDraft["nutrition_targets"]>(field: K, value: SettingsDraft["nutrition_targets"][K]) {
    setSettings((current) => ({
      ...current,
      nutrition_targets: {
        ...current.nutrition_targets,
        [field]: value,
      },
    }));
  }

  const targets = settings.nutrition_targets;
  return (
    <fieldset>
      <legend>Nutrition Targets</legend>
      <div className="form-grid">
        <Field label="Calories"><NumberInput value={targets.calorie_target_kcal} min={1} max={8000} step={1} onChange={(value) => update("calorie_target_kcal", value)} /></Field>
        <Field label="Protein g"><NumberInput value={targets.protein_target_g} min={1} max={400} step={1} onChange={(value) => update("protein_target_g", value)} /></Field>
      </div>
    </fieldset>
  );
}

function ConstraintsForm({ settings, setSettings }: { settings: SettingsDraft; setSettings: Dispatch<SetStateAction<SettingsDraft>> }) {
  function update<K extends keyof SettingsDraft["verification_constraints"]>(field: K, value: SettingsDraft["verification_constraints"][K]) {
    setSettings((current) => ({
      ...current,
      verification_constraints: {
        ...current.verification_constraints,
        [field]: value,
      },
    }));
  }

  const constraints = settings.verification_constraints;
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
  limit,
  recovery,
  setCandidateText,
}: {
  candidateText: string;
  limit?: number;
  recovery?: RecoveryNotice | null;
  setCandidateText: (value: string) => void;
}) {
  const overLimit = Boolean(limit && candidateText.length > limit);
  const describedBy = ["candidate-text-guidance", limit ? "candidate-text-counter" : ""].filter(Boolean).join(" ");
  return (
    <section className="mode-section" id="candidate-text-section">
      <Field label="Meal plan text">
        <textarea
          aria-describedby={describedBy}
          maxLength={limit ? limit + 1 : undefined}
          value={candidateText}
          rows={7}
          onChange={(event) => setCandidateText(event.target.value)}
        />
      </Field>
      <p className="field-help" id="candidate-text-guidance">
        For multi-day plans, use clear labels like Day 1 and Day 2 with meals and amounts listed under each day.
      </p>
      {recovery ? <RecoveryNoticeView notice={recovery} /> : null}
      {limit ? (
        <p className={`character-counter${overLimit ? " is-over-limit" : ""}`} id="candidate-text-counter">
          {candidateText.length.toLocaleString()} / {limit.toLocaleString()} characters
        </p>
      ) : null}
    </section>
  );
}

function LocalModelPanel({
  backend,
}: {
  backend: BackendState;
}) {
  const ready = Boolean(backend.localModel?.enabled && backend.localModel?.ready);
  const tone = ready ? "pass" : "warn";
  return (
    <section className={`notice notice--${tone} local-model-panel`} aria-label="Local model status">
      <strong>{ready ? "Local model available" : "Local model unavailable"}</strong>
      <p><a href="https://github.com/chranama/MealCheck">CLI/API and custom endpoint usage</a></p>
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
  allowOpenAICompatible,
  generationPrompt,
  setGenerationPrompt,
}: {
  provider: ProviderConfig;
  setProvider: Dispatch<SetStateAction<ProviderConfig>>;
  repairJSON: boolean;
  setRepairJSON: (value: boolean) => void;
  mode: GenerationMode;
  setMode: (mode: GenerationMode) => void;
  allowOpenAICompatible: boolean;
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

  const providerOptions = PROVIDER_OPTIONS.filter((option) => allowOpenAICompatible || option.type !== "openai_compatible");
  const selectedProvider = providerOptions.find((option) => option.type === provider.type) ?? providerOptions[0];

  return (
    <section className="mode-section" id="provider-section">
      <div className="notice">
        <strong>Model provider disclosure</strong>
        <p>MealCheck sends this key to the backend for the requested provider call. Use temporary, scoped, budget-limited keys. Hosted public mode disables custom OpenAI-compatible endpoints unless explicitly allowed; run MealCheck locally from the repo for maximum endpoint control.</p>
      </div>
      <div className="form-grid">
        <Field label="Provider">
          <select value={provider.type} onChange={(event) => updateProviderType(event.target.value as ProviderType)}>
            {providerOptions.map((option) => (
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
  const runRecovery = recoveryFromRunFailure(live.status, live.message);
  return (
    <section className="panel live-panel results-panel" id="live-status-panel">
      <div className="panel-heading">
        <h2>Results</h2>
      </div>
      <div className="status-stack">
        <p className="summary-text">{live.message || "Your report will appear here after you create a check."}</p>
        <QualificationNotice qualification={qualification} />
        {runRecovery ? <RecoveryNoticeView notice={runRecovery} role="alert" /> : null}
        {hasReport ? (
          <div className="notice notice--pass live-report-ready" role="status">
            <strong>Report available</strong>
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
            <dt>Result</dt>
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
