import { describe, expect, it } from "vitest";
import { ApiError } from "../api";
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

  it("maps unreachable API failures to status-page guidance", () => {
    const notice = recoveryFromError(new TypeError("Failed to fetch"));

    expect(notice.title).toBe("MealCheck API is unreachable");
    expect(notice.tone).toBe("block");
    expect(notice.action?.href).toBe("/status.html");
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
});
