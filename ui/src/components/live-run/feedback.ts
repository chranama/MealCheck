export function createRunFeedback({
  apiBase,
  candidateTextTooLong,
  hasInviteToken,
  inviteRequired,
  healthBlocksSubmit,
  isLocalModelHosted,
  localModelUnavailable,
  needsCandidateText,
  isBusy,
}: {
  apiBase: string;
  candidateTextTooLong: boolean;
  hasInviteToken: boolean;
  inviteRequired: boolean;
  healthBlocksSubmit: boolean;
  isLocalModelHosted: boolean;
  localModelUnavailable: boolean;
  needsCandidateText: boolean;
  isBusy: boolean;
}) {
  if (isBusy) return "Request in progress.";
  if (!apiBase) return "Report creation needs a configured MealCheck service.";
  if (inviteRequired && !hasInviteToken) return "Enter your access code to start.";
  if (healthBlocksSubmit) return "Service is unavailable right now.";
  if (localModelUnavailable) return "Local model is unavailable right now.";
  if (needsCandidateText) return "Enter a meal plan to create a report.";
  if (candidateTextTooLong) return "Meal plan text is over the local model limit.";
  if (isLocalModelHosted) return "Create a one-day local-model MealCheck report.";
  return "Check eligibility or create a MealCheck report.";
}
