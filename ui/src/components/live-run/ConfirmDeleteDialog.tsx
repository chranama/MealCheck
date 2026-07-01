import { useEffect, useRef } from "react";

export function ConfirmDeleteDialog({
  runID,
  onCancel,
  onConfirm,
}: {
  runID: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const cancelButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cancelButtonRef.current?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onCancel();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onCancel]);

  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-describedby="delete-dialog-description" aria-labelledby="delete-dialog-title" aria-modal="true" className="confirm-dialog" role="dialog">
        <h2 id="delete-dialog-title">Delete report?</h2>
        <p id="delete-dialog-description">This removes the report and its files for {runID}.</p>
        <div className="form-actions">
          <button ref={cancelButtonRef} className="action-button action-button--ghost" type="button" onClick={onCancel}>Cancel</button>
          <button className="action-button action-button--danger" type="button" onClick={onConfirm}>Delete Report</button>
        </div>
      </section>
    </div>
  );
}
