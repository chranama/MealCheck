import type { Dispatch, SetStateAction } from "react";
import { PROVIDER_OPTIONS } from "../../constants";
import type { GenerationMode, ProviderConfig, ProviderType } from "../../types";
import { Field } from "../common/FormControls";

export function ProviderForm({
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

function ModeButton({ mode, activeMode, setMode, label }: { mode: GenerationMode; activeMode: GenerationMode; setMode: (mode: GenerationMode) => void; label: string }) {
  return (
    <button className={`mode-button${activeMode === mode ? " is-active" : ""}`} data-mode={mode} type="button" onClick={() => setMode(mode)}>
      {label}
    </button>
  );
}
