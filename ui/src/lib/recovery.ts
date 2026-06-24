import type { MealPlanQualificationResult, RunStatus } from "../types";
import { ApiError, qualificationFromApiError } from "./api";

export type RecoveryTone = "info" | "pass" | "warn" | "block";

export type RecoveryNotice = {
  title: string;
  message: string;
  tone: RecoveryTone;
  steps?: string[];
  action?: {
    label: string;
    href: string;
  };
};

export function recoveryFromError(errorLike: unknown): RecoveryNotice {
  const qualification = qualificationFromApiError(errorLike);
  if (qualification) return recoveryFromQualification(qualification);

  if (errorLike instanceof ApiError) {
    const code = apiErrorCode(errorLike);
    switch (code) {
      case "queue_full":
        return {
          title: "MealCheck is busy",
          message: "The report queue is full right now. Your input was not submitted.",
          tone: "warn",
          steps: ["Wait a minute and try again.", "Check service status if the queue stays full."],
          action: { label: "Open status page", href: "/status.html" },
        };
      case "rate_limited":
      case "daily_run_limit_reached":
      case "invite_limit_reached":
        return {
          title: "Request limit reached",
          message: "MealCheck accepted too many recent requests from this browser or access code.",
          tone: "warn",
          steps: ["Wait before trying again.", "Avoid repeated retries while a report is already queued."],
        };
      case "local_model_unavailable":
        return {
          title: "AI meal normalization is unavailable",
          message: "The hosted local model is not ready, so MealCheck cannot normalize pasted meal-plan text right now.",
          tone: "warn",
          steps: ["Try again later.", "Check whether the public status page reports a model outage."],
          action: { label: "Open status page", href: "/status.html" },
        };
      case "provider_error":
        return {
          title: "Model provider failed",
          message: "The selected model provider did not return usable meal-plan output.",
          tone: "warn",
          steps: ["Check the model name and API key.", "Try again with a shorter or clearer meal plan.", "Use a temporary, budget-limited provider key."],
        };
      case "unauthorized":
        return {
          title: "Access code needed",
          message: "This MealCheck service requires a valid access code before it can create reports.",
          tone: "warn",
          steps: ["Enter the access code exactly as provided.", "Ask for a fresh code if this one has expired."],
        };
      case "invalid_request":
        return {
          title: "Request needs revision",
          message: apiErrorMessage(errorLike) || "MealCheck could not use this request.",
          tone: "warn",
          steps: ["Review the meal-plan text and verification settings.", "Keep days between 1 and 7 and meals per day between 1 and 6."],
        };
      case "store_error":
      case "store_unavailable":
      case "artifact_error":
      case "artifact_delete_failed":
      case "artifacts_unavailable":
        return {
          title: "MealCheck service problem",
          message: "MealCheck could not complete a backend storage or report-file operation.",
          tone: "block",
          steps: ["Try again later.", "Check the public status page before submitting another report."],
          action: { label: "Open status page", href: "/status.html" },
        };
      case "not_found":
        return {
          title: "Report not found",
          message: "MealCheck could not find that report or artifact.",
          tone: "warn",
          steps: ["Create a new report if the old one was deleted or expired."],
        };
      default:
        if (errorLike.status >= 500) {
          return {
            title: "MealCheck service problem",
            message: "MealCheck returned a server error before the report could finish.",
            tone: "block",
            steps: ["Try again later.", "Check the public status page if the problem persists."],
            action: { label: "Open status page", href: "/status.html" },
          };
        }
        return {
          title: "Request could not be completed",
          message: apiErrorMessage(errorLike) || "MealCheck could not complete this request.",
          tone: "warn",
          steps: ["Review the input and try again."],
        };
    }
  }

  const rawMessage = errorLike instanceof Error ? errorLike.message : String(errorLike || "");
  if (/api base url is required|configured mealcheck service/i.test(rawMessage)) {
    return {
      title: "MealCheck service is not configured",
      message: "The frontend does not have a MealCheck API URL for report creation.",
      tone: "warn",
      steps: ["Use the deployed site configuration or add a valid ?api= URL for local testing."],
    };
  }

  return {
    title: "MealCheck API is unreachable",
    message: "The browser could not reach the MealCheck API.",
    tone: "block",
    steps: ["Check your connection and try again.", "Open the status page to see whether the service is available."],
    action: { label: "Open status page", href: "/status.html" },
  };
}

export function recoveryFromQualification(result: MealPlanQualificationResult): RecoveryNotice {
  switch (result.status) {
    case "not_meal_plan":
      return {
        title: "MealCheck could not find a meal plan",
        message: result.reason || "The text does not describe meals with ingredient-level food items.",
        tone: "block",
        steps: [
          "Paste meals instead of instructions, agenda text, or general notes.",
          "Use labels like Day 1 breakfast, lunch, and dinner.",
          "List foods with amounts, for example: Day 1 breakfast: 1 cup oatmeal, 1 banana, 2 eggs.",
        ],
      };
    case "meal_plan_too_vague":
      return {
        title: "Add amounts and units",
        message: result.reason || "MealCheck needs quantities before it can verify a plan.",
        tone: "warn",
        steps: [
          "Add quantities such as 1 cup, 4 oz, 2 eggs, or 1 tbsp.",
          "Name specific foods instead of broad phrases like salad, bowl, or snack.",
          "Keep each meal on a clear day and meal line.",
        ],
      };
    case "recipe_or_menu_needs_decomposition":
      return {
        title: "Convert this into meal-plan rows",
        message: result.reason || "Recipe or menu text needs to be rewritten as meals before MealCheck can verify it.",
        tone: "warn",
        steps: [
          "Rewrite recipes into breakfast, lunch, dinner, or snack rows.",
          "Assign each row to a day.",
          "List the eaten portion of each ingredient, not cooking instructions.",
        ],
      };
    case "eligible_with_unresolved_items":
      return {
        title: "Some items may need clearer wording",
        message: result.reason || "MealCheck can create a report, but unresolved foods or quantities will remain visible.",
        tone: "warn",
        steps: [
          "Use specific food names from ordinary grocery labels.",
          "Replace vague servings with measured amounts where possible.",
          "Review unresolved items in the report before relying on the decision.",
        ],
      };
    case "eligible_for_verification":
      return {
        title: "Meal plan is ready to check",
        message: result.reason || "MealCheck can verify this meal plan.",
        tone: "pass",
      };
    default:
      return {
        title: "MealCheck reviewed the input",
        message: result.reason || "Review the qualification result before creating a report.",
        tone: "info",
      };
  }
}

export function recoveryFromRunFailure(status: RunStatus, message: string): RecoveryNotice | null {
  if (status !== "failed" || !message.trim()) return null;
  if (/could not normalize|day labels|meal labels|quantit/i.test(message)) {
    return {
      title: "MealCheck could not normalize this plan",
      message,
      tone: "warn",
      steps: [
        "Use clear Day 1, Day 2 labels for multi-day plans.",
        "Put each meal on its own line with food names and numeric quantities.",
        "Shorten the plan and retry if it includes long prose or recipes.",
      ],
    };
  }
  if (/provider|model/i.test(message)) {
    return {
      title: "Model step failed",
      message,
      tone: "warn",
      steps: ["Check provider settings if using BYOK.", "Try a shorter, clearer meal plan.", "Retry later if the hosted model is unavailable."],
    };
  }
  return null;
}

export function apiErrorCode(error: ApiError): string {
  const root = objectRecord(error.bodyJson);
  const errorObject = objectRecord(root?.error);
  const code = errorObject?.code ?? root?.code;
  return code == null ? "" : String(code);
}

function apiErrorMessage(error: ApiError): string {
  const root = objectRecord(error.bodyJson);
  const errorObject = objectRecord(root?.error);
  const message = errorObject?.message ?? errorObject?.detail ?? root?.message;
  return typeof message === "string" ? message : "";
}

function objectRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}
