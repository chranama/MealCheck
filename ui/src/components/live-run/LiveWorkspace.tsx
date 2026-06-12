import { useState } from "react";
import type { Dispatch, FormEvent, ReactNode, SetStateAction } from "react";
import {
  DEFAULT_CONSTRAINTS,
  DEFAULT_GENERATION_PROMPT,
  DEFAULT_PREP_NOTES,
  DEFAULT_PROFILE,
  DEFAULT_PROVIDER,
  INITIAL_MANUAL_ITEMS,
  MEALS,
  MVP_FOODS,
  UNITS,
} from "../../constants";
import { cleanApiBase } from "../../lib/api";
import { readableID } from "../../lib/format";
import { buildRunPayload } from "../../lib/payload";
import type {
  BackendState,
  ConstraintsDraft,
  InputMode,
  LiveState,
  ManualItem,
  Profile,
  ProviderConfig,
  RunPayload,
} from "../../types";
import { Field, NumberInput } from "../common/FormControls";

export function LiveWorkspace({
  apiBase,
  backend,
  live,
  onCreateRun,
  onDeleteRun,
  onError,
}: {
  apiBase: string;
  backend: BackendState;
  live: LiveState;
  onCreateRun: (base: string, inviteToken: string, payload: RunPayload) => Promise<void>;
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
  const [mode, setMode] = useState<InputMode>("manual_structured");
  const [manualItems, setManualItems] = useState<ManualItem[]>(INITIAL_MANUAL_ITEMS);
  const [prepNotes, setPrepNotes] = useState(DEFAULT_PREP_NOTES);
  const [provider, setProvider] = useState<ProviderConfig>(DEFAULT_PROVIDER);
  const [generationPrompt, setGenerationPrompt] = useState(DEFAULT_GENERATION_PROMPT);
  const [repairJSON, setRepairJSON] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const cleanBase = cleanApiBase(apiBase);
  const isSubmitting = live.status === "queued" || live.status === "running";
  const healthBlocksSubmit = backend.kind === "offline" && Boolean(cleanBase);
  const canDeleteRun = Boolean(live.runID && live.status !== "deleted");
  const canCreateRun = Boolean(cleanBase && inviteToken.trim()) && !isSubmitting && !healthBlocksSubmit;
  const createFeedback = createRunFeedback({
    apiBase: cleanBase,
    hasInviteToken: Boolean(inviteToken.trim()),
    healthBlocksSubmit,
    isSubmitting,
  });

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const payload = buildRunPayload({
        mode,
        profile,
        constraints,
        manualItems,
        prepNotes,
        provider,
        generationPrompt,
        repairJSON,
      });
      await onCreateRun(cleanBase, inviteToken, payload);
      if (mode !== "manual_structured") {
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
          canDeleteRun={canDeleteRun}
          createFeedback={createFeedback}
          live={live}
          onDelete={() => setConfirmDelete(true)}
        />

        <form id="live-run-form" onSubmit={handleSubmit}>
          <fieldset>
            <legend>Access</legend>
            <div className="form-grid">
              <Field label="Invite code">
                <input autoComplete="off" value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} type="password" />
              </Field>
            </div>
          </fieldset>

          <ProfileForm profile={profile} setProfile={setProfile} />
          <ConstraintsForm constraints={constraints} setConstraints={setConstraints} />

          <fieldset>
            <legend>Meal Plan Entry</legend>
            <div className="segmented" role="group" aria-label="Input mode">
              <ModeButton mode="manual_structured" activeMode={mode} setMode={setMode} label="Manual" />
              <ModeButton mode="profile_generation" activeMode={mode} setMode={setMode} label="Profile" />
              <ModeButton mode="prompt_generation" activeMode={mode} setMode={setMode} label="Prompt" />
            </div>

            {mode === "manual_structured" ? (
              <ManualPlanForm manualItems={manualItems} setManualItems={setManualItems} prepNotes={prepNotes} setPrepNotes={setPrepNotes} />
            ) : (
              <ProviderForm
                provider={provider}
                setProvider={setProvider}
                repairJSON={repairJSON}
                setRepairJSON={setRepairJSON}
                mode={mode}
                generationPrompt={generationPrompt}
                setGenerationPrompt={setGenerationPrompt}
              />
            )}
          </fieldset>

        </form>
      </section>

      <RunStatusPanel live={live} />

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
  canDeleteRun,
  createFeedback,
  live,
  onDelete,
}: {
  canCreateRun: boolean;
  canDeleteRun: boolean;
  createFeedback: string;
  live: LiveState;
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
  isSubmitting,
}: {
  apiBase: string;
  hasInviteToken: boolean;
  healthBlocksSubmit: boolean;
  isSubmitting: boolean;
}) {
  if (isSubmitting) return "Creating your report.";
  if (!apiBase) return "Report creation needs a configured MealCheck service.";
  if (!hasInviteToken) return "Enter your invite code to start.";
  if (healthBlocksSubmit) return "Service is unavailable right now.";
  return "Ready to create a MealCheck report.";
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
      <legend>Profile</legend>
      <div className="form-grid">
        <Field label="Age"><NumberInput value={profile.age} min={18} max={120} step={1} onChange={(value) => update("age", value)} /></Field>
        <Field label="Sex">
          <select value={profile.sex} onChange={(event) => update("sex", event.target.value)}>
            <option value="male">Male</option>
            <option value="female">Female</option>
          </select>
        </Field>
        <Field label="Height cm"><NumberInput value={profile.height_cm} min={1} max={260} step={1} onChange={(value) => update("height_cm", value)} /></Field>
        <Field label="Weight kg"><NumberInput value={profile.weight_kg} min={1} max={300} step={1} onChange={(value) => update("weight_kg", value)} /></Field>
        <Field label="Activity">
          <select value={profile.activity_level} onChange={(event) => update("activity_level", event.target.value)}>
            <option value="inactive">Inactive</option>
            <option value="low_active">Low Active</option>
            <option value="moderate">Moderate</option>
            <option value="active">Active</option>
            <option value="very_active">Very Active</option>
          </select>
        </Field>
        <Field label="Goal"><input value={profile.goal} onChange={(event) => update("goal", event.target.value)} type="text" /></Field>
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
        <Field label="Diet pattern"><input value={constraints.diet_pattern} onChange={(event) => update("diet_pattern", event.target.value)} type="text" /></Field>
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
          <label><input checked={constraints.requires_shopping_list} onChange={(event) => update("requires_shopping_list", event.target.checked)} type="checkbox" />Shopping list required</label>
          <label><input checked={constraints.requires_prep_safety_notes} onChange={(event) => update("requires_prep_safety_notes", event.target.checked)} type="checkbox" />Prep safety notes required</label>
        </div>
      </details>
    </fieldset>
  );
}

function ModeButton({ mode, activeMode, setMode, label }: { mode: InputMode; activeMode: InputMode; setMode: (mode: InputMode) => void; label: string }) {
  return (
    <button className={`mode-button${activeMode === mode ? " is-active" : ""}`} data-mode={mode} type="button" onClick={() => setMode(mode)}>
      {label}
    </button>
  );
}

function ManualPlanForm({
  manualItems,
  setManualItems,
  prepNotes,
  setPrepNotes,
}: {
  manualItems: ManualItem[];
  setManualItems: Dispatch<SetStateAction<ManualItem[]>>;
  prepNotes: string;
  setPrepNotes: (notes: string) => void;
}) {
  function updateItem<K extends keyof ManualItem>(id: string, field: K, value: ManualItem[K]) {
    setManualItems((current) => current.map((item) => (item.id === id ? { ...item, [field]: value } : item)));
  }
  function addFood() {
    setManualItems((current) => [...current, { id: `item-${Date.now()}`, day: 1, meal: "snack", food: "apple", quantity: 1, unit: "serving" }]);
  }
  function removeFood(id: string) {
    setManualItems((current) => (current.length > 1 ? current.filter((item) => item.id !== id) : current));
  }

  return (
    <section className="mode-section" id="manual-section">
      <div className="manual-header" aria-hidden="true">
        <span>Day</span>
        <span>Meal</span>
        <span>Food</span>
        <span>Qty</span>
        <span>Unit</span>
        <span>Action</span>
      </div>
      <div className="manual-table" id="manual-items">
        {manualItems.map((item) => (
          <div className="manual-row" key={item.id}>
            <ManualCell label="Day">
              <NumberInput className="item-day" value={item.day} min={1} max={7} step={1} onChange={(value) => updateItem(item.id, "day", value)} />
            </ManualCell>
            <ManualCell label="Meal">
              <select className="item-meal" value={item.meal} onChange={(event) => updateItem(item.id, "meal", event.target.value)}>
                {MEALS.map((meal) => <option key={meal} value={meal}>{readableID(meal)}</option>)}
              </select>
            </ManualCell>
            <ManualCell label="Food">
              <select className="item-food" value={item.food} onChange={(event) => updateItem(item.id, "food", event.target.value)}>
                {MVP_FOODS.map((food) => <option key={food} value={food}>{food}</option>)}
              </select>
            </ManualCell>
            <ManualCell label="Qty">
              <NumberInput className="item-quantity" value={item.quantity} min={0.1} max={10000} step={0.1} onChange={(value) => updateItem(item.id, "quantity", value)} />
            </ManualCell>
            <ManualCell label="Unit">
              <select className="item-unit" value={item.unit} onChange={(event) => updateItem(item.id, "unit", event.target.value)}>
                {UNITS.map((unit) => <option key={unit} value={unit}>{unit}</option>)}
              </select>
            </ManualCell>
            <div className="manual-cell manual-cell--action">
              <span className="manual-cell-label">Action</span>
              <button className="action-button action-button--ghost" disabled={manualItems.length <= 1} type="button" onClick={() => removeFood(item.id)}>
                Remove
              </button>
            </div>
          </div>
        ))}
      </div>
      <div className="form-actions form-actions--compact">
        <button className="action-button action-button--secondary" type="button" onClick={addFood}>
          Add Food
        </button>
      </div>
      <Field label="Prep notes">
        <textarea value={prepNotes} rows={4} onChange={(event) => setPrepNotes(event.target.value)} />
      </Field>
    </section>
  );
}

function ManualCell({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="manual-cell">
      <span className="manual-cell-label">{label}</span>
      {children}
    </label>
  );
}

function ProviderForm({
  provider,
  setProvider,
  repairJSON,
  setRepairJSON,
  mode,
  generationPrompt,
  setGenerationPrompt,
}: {
  provider: ProviderConfig;
  setProvider: Dispatch<SetStateAction<ProviderConfig>>;
  repairJSON: boolean;
  setRepairJSON: (value: boolean) => void;
  mode: InputMode;
  generationPrompt: string;
  setGenerationPrompt: (value: string) => void;
}) {
  function update<K extends keyof ProviderConfig>(field: K, value: ProviderConfig[K]) {
    setProvider((current) => ({ ...current, [field]: value }));
  }

  return (
    <section className="mode-section" id="provider-section">
      <div className="notice">
        <strong>BYOK provider disclosure</strong>
        <p>Profile, constraints, prompt text, and generated meal-plan content are sent to the selected provider. MealCheck does not persist the provider key.</p>
      </div>
      <div className="form-grid">
        <Field label="Base URL"><input value={provider.base_url} onChange={(event) => update("base_url", event.target.value)} type="text" /></Field>
        <Field label="Model"><input value={provider.model} onChange={(event) => update("model", event.target.value)} type="text" /></Field>
        <Field label="API key"><input autoComplete="off" value={provider.api_key} onChange={(event) => update("api_key", event.target.value)} type="password" /></Field>
      </div>
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

function RunStatusPanel({ live }: { live: LiveState }) {
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
