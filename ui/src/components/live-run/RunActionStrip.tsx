import type { QualificationState } from "../../types";

export function RunActionStrip({
  canCreateRun,
  canQualify,
  canDeleteRun,
  createFeedback,
  qualification,
  showQualify,
  onQualify,
  onDelete,
}: {
  canCreateRun: boolean;
  canQualify: boolean;
  canDeleteRun: boolean;
  createFeedback: string;
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
