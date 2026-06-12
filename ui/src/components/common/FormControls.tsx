import type { FieldProps } from "../../types";

export function Field({ label, children }: FieldProps) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  );
}

export function NumberInput({
  ariaLabel,
  className = "",
  value,
  min,
  max,
  step,
  onChange,
}: {
  ariaLabel?: string;
  className?: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
}) {
  return (
    <input
      aria-label={ariaLabel}
      className={className}
      max={max}
      min={min}
      onChange={(event) => onChange(Number(event.target.value))}
      step={step}
      type="number"
      value={value}
    />
  );
}

export function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
      <span className="metric-label">{label}</span>
      <strong className="metric-value">{value}</strong>
    </div>
  );
}
