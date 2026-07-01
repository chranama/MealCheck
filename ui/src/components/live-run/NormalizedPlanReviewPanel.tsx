import { Metric } from "../common/FormControls";
import { readableID, reasonLabel, valueText } from "../../lib/format";
import type { NormalizedPlanReviewRow, NormalizedPlanReviewState } from "../../types";

export function NormalizedPlanReviewPanel({
  review,
  onConfirm,
  onReject,
  onRequestRewrite,
}: {
  review: NormalizedPlanReviewState;
  onConfirm: () => void;
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
            <ReviewRows rows={artifact.rows} />
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

function ReviewRows({ rows }: { rows: NormalizedPlanReviewRow[] }) {
  if (rows.length === 0) {
    return <p className="empty-state">No normalized rows are available.</p>;
  }
  return (
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
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function quantityText(row: NormalizedPlanReviewRow): string {
  if (row.quantity_text) return row.quantity_text;
  if (row.quantity !== undefined && row.quantity !== null) {
    return [valueText(row.quantity), row.unit].filter(Boolean).join(" ");
  }
  return "-";
}
