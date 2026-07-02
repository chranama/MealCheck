import { describe, expect, it } from "vitest";
import { ApiError } from "../api";
import { ContractParseError } from "../api_contracts";
import {
  recoveryFromError,
  recoveryFromQualification,
  recoveryFromRunFailure,
} from "../recovery";

describe("recovery", () => {
  it("maps queue capacity errors to retry guidance", () => {
    const notice = recoveryFromError(new ApiError(429, "queue full", {
      error: { code: "queue_full", message: "run queue is full" },
    }));

    expect(notice).toMatchObject({
      title: "MealCheck is busy",
      tone: "warn",
      action: { href: "/status.html" },
    });
    expect(notice.steps?.join(" ")).toContain("try again");
  });

  it("maps rate limits to retry-window guidance", () => {
    const notice = recoveryFromError(new ApiError(429, "rate limited", {
      error: { code: "rate_limited", message: "too many requests" },
    }));

    expect(notice.title).toBe("Request limit reached");
    expect(notice.steps?.join(" ")).toContain("retry window");
    expect(notice.action?.href).toBe("/status.html");
  });

  it("maps report artifact loading errors to report-file guidance", () => {
    const notice = recoveryFromError(new ApiError(404, "missing artifacts", {
      error: { code: "artifacts_unavailable", message: "artifact manifest is not available" },
    }));

    expect(notice.title).toBe("Report files are unavailable");
    expect(notice.steps?.join(" ")).toContain("Refresh the report");
  });

  it("maps unreachable API failures to status-page guidance", () => {
    const notice = recoveryFromError(new TypeError("Failed to fetch"));

    expect(notice.title).toBe("MealCheck API is unreachable");
    expect(notice.tone).toBe("block");
    expect(notice.action?.href).toBe("/status.html");
  });

  it("maps API contract parse failures to stale-frontend guidance", () => {
    const notice = recoveryFromError(new ContractParseError("create run response.run_id", "expected string"));

    expect(notice).toMatchObject({
      title: "MealCheck response changed",
      tone: "block",
      action: { href: "/status.html" },
    });
    expect(notice.steps?.join(" ")).toContain("stale static frontend");
  });

  it("maps vague meal-plan qualification to edit guidance", () => {
    const notice = recoveryFromQualification({
      schema_version: "0.1",
      status: "meal_plan_too_vague",
      reason: "Breakfast, lunch, and dinner are named but quantities are missing.",
      provider_used: false,
    });

    expect(notice.title).toBe("Add amounts and units");
    expect(notice.steps?.join(" ")).toContain("1 cup");
    expect(notice.steps?.join(" ")).toContain("4 oz");
  });

  it("maps hosted contract qualification to one-day guidance", () => {
    const notice = recoveryFromQualification({
      schema_version: "0.1",
      status: "meal_plan_outside_hosted_contract",
      reason: "MealCheck checks one day at a time in the hosted local-model path.",
      provider_used: false,
    });

    expect(notice.title).toBe("Use one day of meal text");
    expect(notice.steps?.join(" ")).toContain("multi-day");
    expect(notice.steps?.join(" ")).toContain("source-item cap");
  });

  it("maps unsupported portion-unit qualification to unit guidance", () => {
    const notice = recoveryFromQualification({
      schema_version: "0.1",
      status: "meal_plan_unsupported_units",
      reason: "MealCheck found unsupported portion units: bowl.",
      provider_used: false,
    });

    expect(notice.title).toBe("Use supported portion units");
    expect(notice.steps?.join(" ")).toContain("grams");
    expect(notice.steps?.join(" ")).toContain("bowl");
  });

  it("maps local-model normalization failures to rewrite guidance", () => {
    const notice = recoveryFromRunFailure(
      "failed",
      "MealCheck could not normalize this text into a verifiable meal plan. Use clear day labels, meal labels, food names, numeric quantities, and supported units.",
    );

    expect(notice).toMatchObject({
      title: "MealCheck could not normalize this plan",
      tone: "warn",
    });
    expect(notice?.steps?.join(" ")).toContain("Day 1");
  });

  it("maps run timeouts to timeout recovery guidance", () => {
    const notice = recoveryFromRunFailure("failed", "run timed out");

    expect(notice).toMatchObject({
      title: "Report timed out",
      tone: "warn",
      action: { href: "/status.html" },
    });
    expect(notice?.steps?.join(" ")).toContain("Shorten");
  });
});
