import { useEffect, useState } from "react";
import type { Dispatch, FormEvent, SetStateAction } from "react";
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
import { modeLabel, readableID } from "../../lib/format";
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
  setApiBase,
  backend,
  live,
  onHealthCheck,
  onCreateRun,
  onDeleteRun,
  onError,
}: {
  apiBase: string;
  setApiBase: (value: string) => void;
  backend: BackendState;
  live: LiveState;
  onHealthCheck: (base: string) => Promise<void>;
  onCreateRun: (base: string, inviteToken: string, payload: RunPayload) => Promise<void>;
  onDeleteRun: () => Promise<void>;
  onError: (error: unknown) => void;
}) {
  const [apiDraft, setApiDraft] = useState(apiBase);
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

  useEffect(() => {
    setApiDraft(apiBase);
  }, [apiBase]);

  async function handleHealthCheck() {
    const cleanBase = cleanApiBase(apiDraft);
    setApiBase(cleanBase);
    await onHealthCheck(cleanBase);
  }

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
      await onCreateRun(cleanApiBase(apiDraft), inviteToken, payload);
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
        <ol className="flow-steps" aria-label="Run workflow">
          <FlowStep active label="Connection" value={backend.label} />
          <FlowStep active label="Profile" value={`${profile.age}, ${readableID(profile.sex)}`} />
          <FlowStep active label="Plan" value={modeLabel(mode)} />
          <FlowStep active={live.status === "completed"} label="Report" value={readableID(live.status)} />
        </ol>

        <form id="live-run-form" onSubmit={handleSubmit}>
          <fieldset>
            <legend>Connection</legend>
            <div className="form-grid">
              <Field label="API Base URL">
                <input value={apiDraft} onChange={(event) => setApiDraft(event.target.value)} placeholder="http://127.0.0.1:8080" type="text" />
              </Field>
              <Field label="Invite Token">
                <input autoComplete="off" value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} type="password" />
              </Field>
            </div>
            <div className="form-actions form-actions--compact">
              <button className="action-button action-button--secondary" type="button" onClick={handleHealthCheck}>
                Check Health
              </button>
            </div>
          </fieldset>

          <ProfileForm profile={profile} setProfile={setProfile} />
          <ConstraintsForm constraints={constraints} setConstraints={setConstraints} />

          <fieldset>
            <legend>Input Mode</legend>
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

          <div className="form-actions">
            <button className="action-button action-button--primary" id="create-run-button" type="submit">Create Run</button>
            <button className="action-button action-button--danger" id="delete-run-button" type="button" onClick={() => onDeleteRun().catch(onError)}>
              Delete Run
            </button>
          </div>
        </form>
      </section>

      <RunStatusPanel live={live} apiBase={apiBase} />
    </section>
  );
}

function FlowStep({ active, label, value }: { active: boolean; label: string; value: string }) {
  return (
    <li className={`flow-step${active ? " is-active" : ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </li>
  );
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
        <Field label="Sodium mg/day"><NumberInput value={constraints.max_sodium_mg_per_day} min={1} max={10000} step={1} onChange={(value) => update("max_sodium_mg_per_day", value)} /></Field>
        <Field label="Added sugar g/meal"><NumberInput value={constraints.max_added_sugar_g_per_meal} min={0} max={200} step={0.1} onChange={(value) => update("max_added_sugar_g_per_meal", value)} /></Field>
        <Field label="Sat fat % kcal"><NumberInput value={constraints.max_saturated_fat_pct_calories} min={0} max={100} step={0.1} onChange={(value) => update("max_saturated_fat_pct_calories", value)} /></Field>
        <Field label="Calorie tolerance %"><NumberInput value={constraints.calorie_tolerance_pct} min={0} max={100} step={0.1} onChange={(value) => update("calorie_tolerance_pct", value)} /></Field>
      </div>
      <div className="check-row">
        <label><input checked={constraints.requires_shopping_list} onChange={(event) => update("requires_shopping_list", event.target.checked)} type="checkbox" />Shopping list required</label>
        <label><input checked={constraints.requires_prep_safety_notes} onChange={(event) => update("requires_prep_safety_notes", event.target.checked)} type="checkbox" />Prep safety notes required</label>
      </div>
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
      <div className="manual-table" id="manual-items">
        {manualItems.map((item) => (
          <div className="manual-row" key={item.id}>
            <NumberInput className="item-day" value={item.day} min={1} max={7} step={1} onChange={(value) => updateItem(item.id, "day", value)} />
            <select className="item-meal" value={item.meal} onChange={(event) => updateItem(item.id, "meal", event.target.value)}>
              {MEALS.map((meal) => <option key={meal} value={meal}>{readableID(meal)}</option>)}
            </select>
            <select className="item-food" value={item.food} onChange={(event) => updateItem(item.id, "food", event.target.value)}>
              {MVP_FOODS.map((food) => <option key={food} value={food}>{food}</option>)}
            </select>
            <NumberInput className="item-quantity" value={item.quantity} min={0.1} max={10000} step={0.1} onChange={(value) => updateItem(item.id, "quantity", value)} />
            <select className="item-unit" value={item.unit} onChange={(event) => updateItem(item.id, "unit", event.target.value)}>
              {UNITS.map((unit) => <option key={unit} value={unit}>{unit}</option>)}
            </select>
            <button className="action-button action-button--ghost" type="button" onClick={() => removeFood(item.id)}>Remove</button>
          </div>
        ))}
      </div>
      <div className="form-actions form-actions--compact">
        <button className="action-button action-button--secondary" type="button" onClick={addFood}>Add Food</button>
      </div>
      <Field label="Prep notes">
        <textarea value={prepNotes} rows={4} onChange={(event) => setPrepNotes(event.target.value)} />
      </Field>
    </section>
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

function RunStatusPanel({ live, apiBase }: { live: LiveState; apiBase: string }) {
  return (
    <section className="panel live-panel" id="live-status-panel">
      <h2>Run Status</h2>
      <div className="status-stack">
        <div className="status-line">
          <span id="live-run-state" className={`status-pill status-pill--${live.status === "completed" ? "pass" : live.status === "failed" || live.status === "deleted" ? "block" : live.status === "queued" || live.status === "running" ? "warn" : "info"}`}>
            {readableID(live.status)}
          </span>
          {live.runID ? <span className="chip">{live.runID}</span> : null}
        </div>
        <p className="summary-text">{live.message || "No live run has been created."}</p>
      </div>
      {live.events.length > 0 ? (
        <div className="event-list">
          {live.events.map((event, index) => (
            <div className="event-row" key={`${event.type}-${index}`}>
              <strong>{readableID(event.type)}</strong>
              <span>{event.message}</span>
            </div>
          ))}
        </div>
      ) : null}
      {live.status !== "deleted" && live.artifactItems.length > 0 ? (
        <div className="artifact-list live-artifact-list">
          {live.artifactItems.map((item) => (
            <div className="artifact-row" key={item.path}>
              <a href={`${apiBase}${item.url}`} target="_blank" rel="noreferrer">{item.path}</a>
              <p className="artifact-meta">{item.type}</p>
            </div>
          ))}
        </div>
      ) : null}
    </section>
  );
}
