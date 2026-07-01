package app

import (
	"regexp"
	"strings"

	"github.com/chranama/MealCheck/internal/core"
)

var (
	publicPathPattern = regexp.MustCompile(`(?i)(/Users|/private|/var|/tmp|[A-Z]:\\)[^\s,;:)]+`)
	publicKeyPattern  = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]+|[A-Za-z0-9_-]*api[_-]?key[A-Za-z0-9_-]*)\b`)
)

func runDocument(run Run, events []RunEvent) core.RunDocument {
	publicRun := run
	publicRun.Error = publicRunMessage(publicRun.Error)
	return core.RunDocument{
		Run:      publicRun,
		Links:    linksForRun(run.ID),
		Progress: runProgress(run, events),
	}
}

func runProgress(run Run, events []RunEvent) *core.RunProgress {
	progress := &core.RunProgress{
		State:     "queued",
		Label:     "Queued",
		Message:   "MealCheck has queued this report.",
		UpdatedAt: run.UpdatedAt,
	}
	if len(events) > 0 {
		progress.LastEvent = events[len(events)-1].Type
	}
	switch run.Status {
	case StatusQueued:
		return progress
	case StatusRunning:
		progress.State = "normalizing"
		progress.Label = "Normalizing"
		progress.Message = "MealCheck is normalizing the meal-plan text."
		switch latestEventType(events, EventPlanNormalized, EventArtifactWritten) {
		case EventPlanNormalized:
			progress.State = "checking"
			progress.Label = "Checking"
			progress.Message = "MealCheck is running deterministic food, nutrient, allergen, and guideline checks."
		case EventArtifactWritten:
			progress.State = "writing_report"
			progress.Label = "Writing report"
			progress.Message = "MealCheck is writing report artifacts."
		}
	case StatusAwaitingReview:
		progress.State = "reviewing"
		progress.Label = "Review normalized plan"
		progress.Message = publicRunMessage(firstNonEmpty(run.Summary, "MealCheck normalized this plan for source-linked review."))
	case StatusCompleted:
		progress.State = "ready"
		progress.Label = "Report ready"
		progress.Message = publicRunMessage(firstNonEmpty(run.Summary, "MealCheck finished this report."))
		progress.FinishedAt = run.CompletedAt
	case StatusFailed:
		progress.State = "failed"
		progress.Label = "Run failed"
		progress.Message = publicRunMessage(firstNonEmpty(run.Error, "MealCheck could not finish this report."))
		progress.Recovery = runFailureRecovery(progress.Message)
		progress.FinishedAt = run.CompletedAt
	case StatusDeleted:
		progress.State = "deleted"
		progress.Label = "Report deleted"
		progress.Message = "MealCheck deleted this report and its artifacts."
	default:
		progress.State = run.Status
		progress.Label = readableState(run.Status)
		progress.Message = publicRunMessage(firstNonEmpty(run.Error, run.Summary, "MealCheck is processing this report."))
	}
	return progress
}

func publicRunEvent(event RunEvent) RunEvent {
	event.Message = publicRunMessage(event.Message)
	return event
}

func publicRunMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	message = publicPathPattern.ReplaceAllString(message, "[redacted path]")
	message = publicKeyPattern.ReplaceAllString(message, "[redacted secret]")
	return message
}

func runFailureRecovery(message string) *core.RecoveryNotice {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "source text rewrite requested before checking"):
		return &core.RecoveryNotice{
			Title:   "Rewrite the meal-plan text",
			Message: "MealCheck stopped before checking so the source text can be revised.",
			Tone:    "info",
			Steps: []string{
				"Edit the meal-plan text that produced the reviewed rows.",
				"Use clear meal labels, food names, numeric quantities, and supported units.",
				"Create a new report when the source text reflects the intended plan.",
			},
			Action: &core.RecoveryAction{Label: "Edit meal-plan text", Href: "#candidate-text-section"},
		}
	case strings.Contains(lower, "normalized plan rejected before checking"):
		return &core.RecoveryNotice{
			Title:   "Review rejected before checking",
			Message: "MealCheck stopped before verification because the normalized rows were not accepted.",
			Tone:    "warn",
			Steps: []string{
				"Edit the meal-plan text if the source was ambiguous.",
				"Create a new report after the source text names the intended foods, amounts, and meals.",
				"Use row correction during review when only one normalized row needs a bounded fix.",
			},
			Action: &core.RecoveryAction{Label: "Edit meal-plan text", Href: "#candidate-text-section"},
		}
	case strings.Contains(lower, "checking failed after review confirmation"):
		return &core.RecoveryNotice{
			Title:   "Confirmed plan could not be checked",
			Message: message,
			Tone:    "block",
			Steps: []string{
				"Review corrected rows for unsupported units, missing quantities, or empty meals.",
				"Use row correction for bounded source-linked fixes before checking.",
				"Rewrite the meal-plan text and create a new report if the normalized structure is wrong.",
			},
		}
	case strings.Contains(lower, "timed out") || strings.Contains(lower, "deadline exceeded"):
		return &core.RecoveryNotice{
			Title:   "Report timed out",
			Message: "MealCheck spent too long normalizing or checking this report.",
			Tone:    "warn",
			Steps: []string{
				"Retry with one day of meal-labeled ingredient text.",
				"Shorten long meal descriptions before resubmitting.",
				"Check service status if timeouts repeat.",
			},
			Action: &core.RecoveryAction{Label: "Open status page", Href: "/status.html"},
		}
	case strings.Contains(lower, "could not normalize") ||
		strings.Contains(lower, "day labels") ||
		strings.Contains(lower, "meal labels") ||
		strings.Contains(lower, "source food item") ||
		strings.Contains(lower, "quantit"):
		return &core.RecoveryNotice{
			Title:   "MealCheck could not normalize this plan",
			Message: message,
			Tone:    "warn",
			Steps: []string{
				"Use one day only, with Day 1 or meal labels.",
				"Use lines or paragraphs where each meal names foods with numeric quantities.",
				"Shorten the plan and retry if it includes recipes or unrelated prose.",
			},
		}
	case strings.Contains(lower, "provider") ||
		strings.Contains(lower, "model") ||
		strings.Contains(lower, "llama"):
		return &core.RecoveryNotice{
			Title:   "Model step failed",
			Message: message,
			Tone:    "warn",
			Steps: []string{
				"Retry later if the hosted local model is unavailable.",
				"Use a shorter, clearer meal plan.",
				"Check provider settings when using a BYOK provider.",
			},
			Action: &core.RecoveryAction{Label: "Open status page", Href: "/status.html"},
		}
	case strings.Contains(lower, "artifact") || strings.Contains(lower, "report"):
		return &core.RecoveryNotice{
			Title:   "Report files could not be loaded",
			Message: message,
			Tone:    "block",
			Steps: []string{
				"Refresh the report once.",
				"Create a new report if the files were deleted or expired.",
				"Check service status if report files stay unavailable.",
			},
			Action: &core.RecoveryAction{Label: "Open status page", Href: "/status.html"},
		}
	default:
		return &core.RecoveryNotice{
			Title:   "Report failed",
			Message: message,
			Tone:    "warn",
			Steps: []string{
				"Review the meal-plan text and try again.",
				"Check service status if the same failure repeats.",
			},
			Action: &core.RecoveryAction{Label: "Open status page", Href: "/status.html"},
		}
	}
}

func latestEventType(events []RunEvent, targets ...string) string {
	targetSet := map[string]bool{}
	for _, target := range targets {
		targetSet[target] = true
	}
	for i := len(events) - 1; i >= 0; i-- {
		if targetSet[events[i].Type] {
			return events[i].Type
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func readableState(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return "Processing"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
