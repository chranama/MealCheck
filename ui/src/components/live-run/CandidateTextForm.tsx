import { RecoveryNoticeView } from "../common/RecoveryNotice";
import { Field } from "../common/FormControls";
import type { RecoveryNotice } from "../../lib/recovery";

export function CandidateTextForm({
  candidateText,
  limit,
  recovery,
  setCandidateText,
  sourceItemLimit,
}: {
  candidateText: string;
  limit?: number;
  recovery?: RecoveryNotice | null;
  setCandidateText: (value: string) => void;
  sourceItemLimit?: number;
}) {
  const overLimit = Boolean(limit && candidateText.length > limit);
  const describedBy = ["candidate-text-guidance", limit ? "candidate-text-counter" : ""].filter(Boolean).join(" ");
  const sourceItemGuidance = sourceItemLimit
    ? ` Up to ${sourceItemLimit.toLocaleString()} source food items.`
    : "";
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
        Paste one day of meal-labeled ingredient text. Lines or paragraphs are OK when each meal names foods with amounts. Avoid weekly plans, recipes, grocery lists, and long inventories.{sourceItemGuidance}
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
