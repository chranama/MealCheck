import type {
  Citations,
  Decision,
  DemoIndex,
  Manifest,
  Report,
  ReportArtifacts,
} from "../../types";

export function demoIndex(): DemoIndex {
  return {
    schema_version: "0.1",
    demo_runs: [
      {
        id: "seeded-pass",
        title: "Seeded Pass",
        summary: "A small passing demo.",
        base_path: "/demo-runs/seeded-pass",
      },
    ],
  };
}

export function decision(overrides: Partial<Decision> = {}): Decision {
  return {
    decision: "pass",
    risk_level: "low",
    summary: "Plan fits the configured checks.",
    checks: [
      {
        check_id: "sodium_limit",
        status: "pass",
        message: "Sodium is within the configured daily limit.",
        affected_days: [1],
        source_refs: ["dga_sodium"],
      },
    ],
    ...overrides,
  };
}

export function report(overrides: Partial<Report> = {}): Report {
  return {
    guideline_pack_id: "dga-2025-2030-us-adult-general-v1",
    constraint_summary: {
      max_sodium_mg_per_day: 2300,
      max_saturated_fat_pct_calories: 10,
    },
    profile_summary: {
      calorie_target_kcal: 2000,
      protein_target_g: 98,
    },
    ...overrides,
  };
}

export function citations(): Citations {
  return {
    sources: [
      {
        source_id: "dga_sodium",
        title: "Dietary Guidelines for Americans",
        publisher: "USDA and HHS",
        url: "https://www.dietaryguidelines.gov/",
      },
    ],
  };
}

export function manifest(): Manifest {
  return {
    mode: "manual_structured",
    artifacts: [
      "decision.json",
      "report.json",
      "report.pdf",
      "daily-totals.json",
      "resolved-foods.json",
      "unresolved-foods.json",
      "manifest.json",
      "guideline-pack/citations.json",
    ],
  };
}

export function reportArtifacts(overrides: Partial<ReportArtifacts> = {}): ReportArtifacts {
  return {
    base: "/demo-runs/seeded-pass",
    decision: decision(),
    report: report(),
    totals: [
      {
        day: 1,
        saturated_fat_pct_calories: 8,
        nutrients: {
          energy_kcal: 1980,
          protein_g: 102,
          sodium_mg: 1750,
          added_sugar_g: 8,
        },
      },
    ],
    resolved: [
      {
        day: 1,
        meal: "breakfast",
        food: "cooked oatmeal",
        grams: 234,
        nutrients: {
          energy_kcal: 160,
          protein_g: 6,
          sodium_mg: 5,
          added_sugar_g: 0,
        },
      },
    ],
    unresolved: [],
    manifest: manifest(),
    pack: null,
    citations: citations(),
    artifactItems: [
      {
        path: "report.pdf",
        type: "application/pdf",
        url: "/api/runs/run-1/artifacts/report.pdf",
      },
    ],
    ...overrides,
  };
}
