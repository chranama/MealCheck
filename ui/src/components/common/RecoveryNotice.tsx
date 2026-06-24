import type { RecoveryNotice } from "../../lib/recovery";

export function RecoveryNoticeView({
  notice,
  role = "status",
}: {
  notice: RecoveryNotice;
  role?: "status" | "alert";
}) {
  return (
    <div className={`notice notice--${notice.tone} recovery-notice`} role={role}>
      <strong>{notice.title}</strong>
      <p>{notice.message}</p>
      {notice.steps?.length ? (
        <ul className="recovery-steps">
          {notice.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ul>
      ) : null}
      {notice.action ? (
        <p className="recovery-action">
          <a href={notice.action.href}>{notice.action.label}</a>
        </p>
      ) : null}
    </div>
  );
}
