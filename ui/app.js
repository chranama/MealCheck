"use strict";

const state = {
  activeTab: "checks",
  demos: [],
  selectedDemo: null,
  artifacts: null,
};

document.addEventListener("DOMContentLoaded", () => {
  init().catch((error) => {
    renderError(error);
  });
});

async function init() {
  setupTabs();
  await updateBackendHealth();
  const index = await loadJSON("demo-runs/index.json");
  state.demos = index.demo_runs || [];
  renderDemoList();
  if (state.demos.length > 0) {
    await loadDemo(state.demos[0].id);
  }
}

function setupTabs() {
  document.querySelectorAll(".tab-button").forEach((button) => {
    button.addEventListener("click", () => {
      state.activeTab = button.dataset.tab;
      document.querySelectorAll(".tab-button").forEach((candidate) => {
        const isActive = candidate === button;
        candidate.classList.toggle("is-active", isActive);
        candidate.setAttribute("aria-selected", String(isActive));
      });
      renderActiveTab();
    });
  });
}

async function updateBackendHealth() {
  const metaBase = document.querySelector('meta[name="mealcheck-api-base"]')?.content || "";
  const apiBase = (window.MEALCHECK_API_BASE_URL || metaBase).trim().replace(/\/$/, "");
  if (!apiBase) {
    setBackendStatus("idle", "Static demo");
    return;
  }

  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 2500);
  try {
    const response = await fetch(`${apiBase}/api/health`, {
      signal: controller.signal,
      headers: { accept: "application/json" },
    });
    window.clearTimeout(timeout);
    setBackendStatus(response.ok ? "online" : "offline", response.ok ? "Online" : "Unavailable");
  } catch {
    window.clearTimeout(timeout);
    setBackendStatus("offline", "Unavailable");
  }
}

function setBackendStatus(kind, label) {
  const dot = document.getElementById("backend-dot");
  const text = document.getElementById("backend-label");
  dot.className = `status-dot status-dot--${kind}`;
  text.textContent = label;
}

async function loadDemo(demoID) {
  const demo = state.demos.find((candidate) => candidate.id === demoID);
  if (!demo) {
    return;
  }
  state.selectedDemo = demo;
  const base = demo.base_path;
  const [decision, report, totals, resolved, unresolved, manifest, pack, citations] = await Promise.all([
    loadJSON(`${base}/decision.json`),
    loadJSON(`${base}/report.json`),
    loadJSON(`${base}/daily-totals.json`),
    loadJSON(`${base}/resolved-foods.json`),
    loadJSON(`${base}/unresolved-foods.json`),
    loadJSON(`${base}/manifest.json`),
    loadJSON(`${base}/guideline-pack/pack.json`),
    loadJSON(`${base}/guideline-pack/citations.json`),
  ]);

  state.artifacts = {
    base,
    decision,
    report,
    totals,
    resolved,
    unresolved,
    manifest,
    pack,
    citations,
  };
  renderDemoList();
  renderSummary();
  renderActiveTab();
}

function renderDemoList() {
  const list = document.getElementById("demo-list");
  clear(list);
  state.demos.forEach((demo) => {
    const button = el("button", {
      className: `demo-button${state.selectedDemo?.id === demo.id ? " is-active" : ""}`,
      type: "button",
    }, [
      el("strong", {}, demo.title),
      el("span", {}, demo.summary),
    ]);
    button.addEventListener("click", () => {
      loadDemo(demo.id).catch(renderError);
    });
    list.append(button);
  });
}

function renderSummary() {
  const target = document.getElementById("summary-band");
  clear(target);
  const { decision, report, totals, unresolved, resolved } = state.artifacts;
  const failedChecks = decision.checks.filter((check) => check.status === "block" || check.status === "warn");
  target.append(
    el("section", { className: "summary-main" }, [
      el("div", { className: "decision-line" }, [
        el("span", { className: `decision-pill decision-pill--${decision.decision}` }, decision.decision),
        el("span", { className: "chip" }, `Risk ${decision.risk_level}`),
        el("span", { className: "chip" }, report.guideline_pack_id),
      ]),
      el("h2", { className: "summary-title" }, state.selectedDemo.title),
      el("p", { className: "summary-text" }, decision.summary),
    ]),
    el("aside", { className: "summary-side" }, [
      el("div", { className: "metric-grid" }, [
        metric("Checks", String(decision.checks.length)),
        metric("Needs Review", String(failedChecks.length)),
        metric("Resolved Foods", String(resolved.length)),
        metric("Unresolved", String(unresolved.length)),
        metric("Days", String(totals.length)),
        metric("Mode", state.artifacts.manifest.mode),
      ]),
    ]),
  );
}

function renderActiveTab() {
  if (!state.artifacts) {
    return;
  }
  const panel = document.getElementById("tab-panel");
  clear(panel);
  if (state.activeTab === "checks") {
    renderChecks(panel);
  } else if (state.activeTab === "nutrition") {
    renderNutrition(panel);
  } else if (state.activeTab === "foods") {
    renderFoods(panel);
  } else if (state.activeTab === "sources") {
    renderSources(panel);
  } else if (state.activeTab === "artifacts") {
    renderArtifacts(panel);
  }
}

function renderChecks(panel) {
  const { decision } = state.artifacts;
  panel.append(el("h2", {}, "Check Details"));
  const list = el("div", { className: "check-list" });
  decision.checks.forEach((check) => {
    const sourceChips = (check.source_refs || []).map((sourceID) => sourceChip(sourceID));
    const affected = [
      ...(check.affected_days || []).map((day) => `Day ${day}`),
      ...(check.affected_meals || []).map((meal) => meal),
    ];
    list.append(el("article", { className: "check-card" }, [
      el("header", {}, [
        el("h3", { className: "check-title" }, readableID(check.check_id)),
        el("span", { className: `status-pill status-pill--${check.status}` }, check.status),
      ]),
      el("p", { className: "check-message" }, check.message),
      chipRow([...affected, ...sourceChips]),
      evidenceBlock(check.evidence),
    ]));
  });
  panel.append(list);
}

function renderNutrition(panel) {
  const { totals, report } = state.artifacts;
  const sodiumLimit = report.constraint_summary.max_sodium_mg_per_day || 2300;
  const calorieTarget = report.profile_summary.calorie_target_kcal || 2000;
  panel.append(el("h2", {}, "Daily Nutrition Totals"));
  const grid = el("div", { className: "nutrition-grid" });
  totals.forEach((total) => {
    const nutrients = total.nutrients;
    grid.append(el("article", { className: "day-card" }, [
      el("div", { className: "day-number" }, `Day ${total.day}`),
      el("div", { className: "nutrient-stack" }, [
        nutrientBar("Calories", nutrients.energy_kcal, calorieTarget, "kcal", nutrients.energy_kcal > calorieTarget * 1.15 || nutrients.energy_kcal < calorieTarget * 0.85),
        nutrientBar("Protein", nutrients.protein_g, report.profile_summary.protein_target_g || 98.4, "g", false),
        nutrientBar("Sodium", nutrients.sodium_mg, sodiumLimit, "mg", nutrients.sodium_mg >= sodiumLimit),
        nutrientBar("Added Sugar", nutrients.added_sugar_g, 30, "g", false),
        nutrientBar("Sat Fat", total.saturated_fat_pct_calories, report.constraint_summary.max_saturated_fat_pct_calories || 10, "% kcal", total.saturated_fat_pct_calories > (report.constraint_summary.max_saturated_fat_pct_calories || 10)),
      ]),
    ]));
  });
  panel.append(grid);
}

function renderFoods(panel) {
  const { resolved, unresolved } = state.artifacts;
  panel.append(el("h2", {}, "Food Resolution"));
  const sections = el("div", { className: "food-sections" });
  sections.append(el("article", { className: "food-card" }, [
    el("header", {}, [
      el("h3", { className: "food-title" }, "Unresolved Foods"),
      el("span", { className: "status-pill status-pill--block" }, String(unresolved.length)),
    ]),
    unresolved.length === 0 ? el("p", { className: "empty-state" }, "None.") : foodTable(unresolved, ["day", "meal", "food", "quantity_text", "unresolved_reason"]),
  ]));
  sections.append(el("article", { className: "food-card" }, [
    el("header", {}, [
      el("h3", { className: "food-title" }, "Resolved Foods"),
      el("span", { className: "status-pill status-pill--pass" }, String(resolved.length)),
    ]),
    foodTable(resolved.map((item) => ({
      day: item.day,
      meal: item.meal,
      food: item.food,
      grams: round(item.grams),
      calories: round(item.nutrients.energy_kcal),
      sodium_mg: round(item.nutrients.sodium_mg),
    })), ["day", "meal", "food", "grams", "calories", "sodium_mg"]),
  ]));
  panel.append(sections);
}

function renderSources(panel) {
  const { citations } = state.artifacts;
  panel.append(el("h2", {}, "Source References"));
  const list = el("div", { className: "source-list" });
  citations.sources.forEach((source) => {
    list.append(el("article", { className: "source-card" }, [
      el("header", {}, [
        el("div", {}, [
          el("h3", { className: "source-title" }, source.title),
          el("p", { className: "source-meta" }, source.publisher || source.source_id),
        ]),
        el("a", { href: source.url, target: "_blank", rel: "noreferrer" }, "Open source"),
      ]),
      el("div", { className: "chip-row" }, (source.claims_used || []).map((claim) => el("span", { className: "chip" }, claim.claim_id))),
    ]));
  });
  panel.append(list);
}

function renderArtifacts(panel) {
  const { manifest, base } = state.artifacts;
  panel.append(el("h2", {}, "Artifact Bundle"));
  const list = el("div", { className: "artifact-list" });
  manifest.artifacts.forEach((artifactPath) => {
    list.append(el("div", { className: "artifact-row" }, [
      el("a", { href: `${base}/${artifactPath}` }, artifactPath),
      el("p", { className: "artifact-meta" }, artifactType(artifactPath)),
    ]));
  });
  panel.append(list);
}

function metric(label, value) {
  return el("div", { className: "metric" }, [
    el("span", { className: "metric-label" }, label),
    el("strong", { className: "metric-value" }, value),
  ]);
}

function nutrientBar(label, value, target, unit, flagged) {
  const width = target > 0 ? Math.max(2, Math.min(100, (value / target) * 100)) : 2;
  return el("div", { className: "nutrient-row" }, [
    el("span", { className: "metric-label" }, label),
    el("div", { className: "bar-track", role: "img", "aria-label": `${label} ${round(value)} ${unit}` }, [
      el("div", { className: `bar-fill${flagged ? " bar-fill--warn" : ""}`, style: `width: ${width}%` }),
    ]),
    el("strong", {}, `${round(value)} ${unit}`),
  ]);
}

function foodTable(rows, fields) {
  const table = el("table");
  table.append(el("thead", {}, [
    el("tr", {}, fields.map((field) => el("th", {}, readableID(field)))),
  ]));
  const body = el("tbody");
  rows.forEach((row) => {
    body.append(el("tr", {}, fields.map((field) => el("td", {}, valueText(row[field])))));
  });
  table.append(body);
  return el("div", { className: "table-wrap" }, [table]);
}

function evidenceBlock(evidence) {
  if (!evidence || evidence.length === 0) {
    return document.createDocumentFragment();
  }
  return el("details", { className: "evidence" }, [
    el("summary", {}, "Evidence"),
    el("pre", {}, JSON.stringify(evidence, null, 2)),
  ]);
}

function sourceChip(sourceID) {
  const source = findSource(sourceID);
  return source ? source.title : sourceID;
}

function findSource(sourceID) {
  return state.artifacts.citations.sources.find((source) => source.source_id === sourceID);
}

function chipRow(values) {
  if (values.length === 0) {
    return document.createDocumentFragment();
  }
  return el("div", { className: "chip-row" }, values.map((value) => el("span", { className: "chip" }, value)));
}

function renderError(error) {
  const panel = document.getElementById("tab-panel");
  clear(panel);
  panel.append(el("div", { className: "error-state" }, error.message || String(error)));
}

async function loadJSON(path) {
  const response = await fetch(path, { headers: { accept: "application/json" } });
  if (!response.ok) {
    throw new Error(`Could not load ${path}`);
  }
  return response.json();
}

function artifactType(path) {
  if (path.endsWith(".jsonl")) {
    return "JSONL";
  }
  if (path.endsWith(".json")) {
    return "JSON";
  }
  if (path.endsWith(".html")) {
    return "HTML";
  }
  if (path.endsWith(".md")) {
    return "Markdown";
  }
  return "File";
}

function readableID(value) {
  return String(value || "")
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function valueText(value) {
  if (value === undefined || value === null || value === "") {
    return "-";
  }
  if (typeof value === "number") {
    return String(round(value));
  }
  return String(value);
}

function round(value) {
  return Math.round(Number(value) * 10) / 10;
}

function clear(node) {
  while (node.firstChild) {
    node.removeChild(node.firstChild);
  }
}

function el(tag, options = {}, children = []) {
  const node = document.createElement(tag);
  Object.entries(options).forEach(([key, value]) => {
    if (key === "className") {
      node.className = value;
    } else if (key === "style") {
      node.setAttribute("style", value);
    } else if (key in node) {
      node[key] = value;
    } else {
      node.setAttribute(key, value);
    }
  });

  const childList = Array.isArray(children) ? children : [children];
  childList.forEach((child) => {
    if (child === null || child === undefined) {
      return;
    }
    if (child instanceof Node || child instanceof DocumentFragment) {
      node.append(child);
    } else {
      node.append(document.createTextNode(String(child)));
    }
  });
  return node;
}
