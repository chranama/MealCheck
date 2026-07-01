import { useState } from "react";
import { Metric } from "../common/FormControls";
import { readableID, reasonLabel, valueText } from "../../lib/format";
import type { NormalizedPlanCorrectionPayload, NormalizedPlanReviewRow, NormalizedPlanReviewState } from "../../types";

export function NormalizedPlanReviewPanel({
  review,
  onConfirm,
  onCorrectRow,
  onReject,
  onRequestRewrite,
}: {
  review: NormalizedPlanReviewState;
  onConfirm: () => void;
  onCorrectRow: (payload: NormalizedPlanCorrectionPayload) => Promise<void>;
  onReject: () => void;
  onRequestRewrite: () => void;
}) {
  if (review.status === "idle") return null;
  const artifact = review.artifact;
  const busy = review.status === "loading" || review.status === "submitting";
  return (
    <section className="panel live-panel review-panel" aria-busy={busy} id="normalized-plan-review">
      <div className="panel-heading">
        <h2>Normalized Plan Review</h2>
      </div>
      <div className="status-stack">
        <p className="summary-text">{review.message || "Normalized plan review is loading."}</p>
        {artifact ? (
          <>
            <div className="metric-grid review-metrics" aria-label="Review signals">
              <Metric label="Source items" value={String(artifact.trust_signals.source_item_count)} />
              <Metric label="Normalized rows" value={String(artifact.trust_signals.normalized_row_count)} />
              <Metric label="Unresolved" value={String(artifact.trust_signals.unresolved_item_count)} />
              <Metric label="Repairs" value={String(artifact.trust_signals.repair_count)} />
            </div>
            {artifact.requires_confirmation ? (
              <div className="notice notice--warn" role="status">
                <strong>Review needed</strong>
                <p>Unresolved or repaired rows are present.</p>
              </div>
            ) : (
              <div className="notice notice--pass" role="status">
                <strong>Ready to check</strong>
                <p>Source rows normalized without unresolved items or repairs.</p>
              </div>
            )}
            <ReviewRows busy={busy} rows={artifact.rows} onCorrectRow={onCorrectRow} />
            <div className="form-actions form-actions--compact">
              <button className="action-button action-button--primary" disabled={busy} type="button" onClick={onConfirm}>
                Check now
              </button>
              <button className="action-button action-button--secondary" disabled={busy} type="button" onClick={onRequestRewrite}>
                Rewrite input
              </button>
              <button className="action-button action-button--danger" disabled={busy} type="button" onClick={onReject}>
                Reject
              </button>
            </div>
          </>
        ) : null}
      </div>
    </section>
  );
}

type CorrectionDraft = {
  food: string;
  quantity: string;
  unit: string;
  quantityText: string;
  unresolvedReason: string;
};

function ReviewRows({
  busy,
  rows,
  onCorrectRow,
}: {
  busy: boolean;
  rows: NormalizedPlanReviewRow[];
  onCorrectRow: (payload: NormalizedPlanCorrectionPayload) => Promise<void>;
}) {
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [draft, setDraft] = useState<CorrectionDraft | null>(null);
  if (rows.length === 0) {
    return <p className="empty-state">No normalized rows are available.</p>;
  }
  const editingRow = editingIndex == null ? null : rows[editingIndex] || null;

  function startEditing(index: number) {
    setEditingIndex(index);
    setDraft(correctionDraftFromRow(rows[index]));
  }

  function cancelEditing() {
    setEditingIndex(null);
    setDraft(null);
  }

  async function saveCorrection() {
    if (editingIndex == null || !editingRow || !draft) return;
    const quantity = draft.quantity.trim() === "" ? undefined : Number(draft.quantity);
    const payload: NormalizedPlanCorrectionPayload = {
      row_index: editingIndex,
      source_item_id: editingRow.source_item_id,
      food: draft.food.trim(),
      reason: "Normalized row corrected before checking.",
    };
    if (quantity !== undefined && Number.isFinite(quantity)) {
      payload.quantity = quantity;
      payload.unit = draft.unit.trim();
    } else {
      payload.quantity_text = draft.quantityText.trim();
      payload.resolution_status = "unresolved";
      payload.unresolved_reason = draft.unresolvedReason.trim();
      if (draft.unit.trim()) {
        payload.unit = draft.unit.trim();
      }
    }
    await onCorrectRow(payload);
    cancelEditing();
  }

  return (
    <>
      <div className="table-wrap review-table-wrap">
        <table className="review-table">
          <thead>
            <tr>
              <th>Day</th>
              <th>Meal</th>
              <th>Source text</th>
              <th>Normalized food</th>
              <th>Quantity</th>
              <th>Status</th>
              <th>Correction</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={`${row.day}-${row.meal_code}-${row.source_item_id}-${row.normalized_food || "row"}-${index}`}>
                <td>{valueText(row.day)}</td>
                <td>{readableID(row.meal_label || row.meal_code)}</td>
                <td className="review-source-cell">{row.source_text || "-"}</td>
                <td>{row.normalized_food || "-"}</td>
                <td>{quantityText(row)}</td>
                <td>{row.resolved ? "Resolved" : reasonLabel(row.unresolved_reason || "unresolved")}</td>
                <td>
                  <button className="action-button action-button--ghost review-edit-button" disabled={busy} type="button" onClick={() => startEditing(index)}>
                    Correct
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {editingRow && draft ? (
        <form
          className="review-correction-form"
          aria-label="Correct normalized row"
          onSubmit={(event) => {
            event.preventDefault();
            void saveCorrection();
          }}
        >
          <div>
            <p className="eyebrow">Source item {editingRow.source_item_id}</p>
            <p className="summary-text">{editingRow.source_text}</p>
          </div>
          <div className="form-grid">
            <label className="field">
              <span>Food</span>
              <input value={draft.food} onChange={(event) => setDraft({ ...draft, food: event.target.value })} />
            </label>
            <label className="field">
              <span>Quantity</span>
              <input inputMode="decimal" value={draft.quantity} onChange={(event) => setDraft({ ...draft, quantity: event.target.value })} />
            </label>
            <label className="field">
              <span>Unit</span>
              <select value={draft.unit} onChange={(event) => setDraft({ ...draft, unit: event.target.value })}>
                <option value="">Unresolved</option>
                <option value="g">g</option>
                <option value="oz">oz</option>
                <option value="cup">cup</option>
                <option value="tbsp">tbsp</option>
                <option value="tsp">tsp</option>
                <option value="slice">slice</option>
                <option value="serving">serving</option>
              </select>
            </label>
            <label className="field">
              <span>Quantity text</span>
              <input value={draft.quantityText} onChange={(event) => setDraft({ ...draft, quantityText: event.target.value })} />
            </label>
            <label className="field">
              <span>Unresolved reason</span>
              <input value={draft.unresolvedReason} onChange={(event) => setDraft({ ...draft, unresolvedReason: event.target.value })} />
            </label>
          </div>
          <div className="form-actions form-actions--compact">
            <button className="action-button action-button--primary" disabled={busy || !correctionDraftValid(draft)} type="submit">
              Save correction
            </button>
            <button className="action-button action-button--ghost" disabled={busy} type="button" onClick={cancelEditing}>
              Cancel
            </button>
          </div>
        </form>
      ) : null}
    </>
  );
}

function correctionDraftFromRow(row: NormalizedPlanReviewRow): CorrectionDraft {
  return {
    food: row.normalized_food || "",
    quantity: row.quantity === undefined || row.quantity === null ? "" : String(row.quantity),
    unit: row.unit || "",
    quantityText: row.quantity_text || "",
    unresolvedReason: row.unresolved_reason || "",
  };
}

function correctionDraftValid(draft: CorrectionDraft): boolean {
  if (!draft.food.trim()) return false;
  if (draft.quantity.trim()) {
    return Number(draft.quantity) > 0 && Boolean(draft.unit.trim());
  }
  return Boolean(draft.quantityText.trim() && draft.unresolvedReason.trim());
}

function quantityText(row: NormalizedPlanReviewRow): string {
  if (row.quantity_text) return row.quantity_text;
  if (row.quantity !== undefined && row.quantity !== null) {
    return [valueText(row.quantity), row.unit].filter(Boolean).join(" ");
  }
  return "-";
}
