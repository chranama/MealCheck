import type { BackendState, QualificationState, RunStatus } from "../types";

export type ReportPreflightReason =
  | "ready"
  | "request_in_progress"
  | "api_unconfigured"
  | "invite_required"
  | "backend_offline"
  | "local_model_unavailable"
  | "candidate_text_required"
  | "candidate_text_too_long";

export type ReportPreflight = {
  canCreate: boolean;
  canQualify: boolean;
  reason: ReportPreflightReason;
  message: string;
};

export function reportCreationPreflight({
  apiBase,
  backend,
  candidateText,
  candidateTextTooLong,
  inviteToken,
  liveStatus,
  qualificationStatus,
  reviewStatus,
}: {
  apiBase: string;
  backend: BackendState;
  candidateText: string;
  candidateTextTooLong: boolean;
  inviteToken: string;
  liveStatus: RunStatus;
  qualificationStatus: QualificationState["status"];
  reviewStatus: string;
}): ReportPreflight {
  const inviteRequired = backend.accessMode === "invite_required";
  const isLocalModelHosted = backend.hostedMode === "local_model";
  const localModelUnavailable = isLocalModelHosted && !Boolean(backend.localModel?.enabled && backend.localModel?.ready);
  const isBusy = liveStatus === "queued" || liveStatus === "running" || liveStatus === "awaiting_review" || reviewStatus === "submitting";
  const isCheckingQualification = qualificationStatus === "checking";
  const candidateTextMissing = isLocalModelHosted && !candidateText.trim();
  const healthBlocksSubmit = backend.kind === "offline" && Boolean(apiBase);

  if (isBusy || isCheckingQualification) {
    return blocked("request_in_progress", "Request in progress.");
  }
  if (!apiBase) {
    return blocked("api_unconfigured", "Report creation needs a configured MealCheck service.");
  }
  if (inviteRequired && !inviteToken.trim()) {
    return blocked("invite_required", "Enter your access code to start.");
  }
  if (healthBlocksSubmit) {
    return blocked("backend_offline", "Service is unavailable right now.");
  }
  if (localModelUnavailable) {
    return {
      canCreate: false,
      canQualify: false,
      reason: "local_model_unavailable",
      message: "Local model is unavailable right now.",
    };
  }
  if (candidateTextMissing) {
    return blocked("candidate_text_required", "Enter a meal plan to create a report.");
  }
  if (isLocalModelHosted && candidateTextTooLong) {
    return blocked("candidate_text_too_long", "Meal plan text is over the local model limit.");
  }

  return {
    canCreate: true,
    canQualify: !isLocalModelHosted && Boolean(candidateText.trim()),
    reason: "ready",
    message: isLocalModelHosted
      ? "Create a one-day local-model MealCheck report."
      : "Check eligibility or create a MealCheck report.",
  };
}

function blocked(reason: ReportPreflightReason, message: string): ReportPreflight {
  return {
    canCreate: false,
    canQualify: false,
    reason,
    message,
  };
}
