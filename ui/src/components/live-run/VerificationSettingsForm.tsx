import type { Dispatch, SetStateAction } from "react";
import type { SettingsDraft } from "../../types";
import { Field, NumberInput } from "../common/FormControls";

export function VerificationSettingsForm({
  settings,
  setSettings,
}: {
  settings: SettingsDraft;
  setSettings: Dispatch<SetStateAction<SettingsDraft>>;
}) {
  return (
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
  );
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
