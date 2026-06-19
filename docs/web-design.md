# Web Design

This document defines the first web design system for MealCheck. It is scoped to
the static frontend in `ui/` and the current MVP product surface: public seeded
reports plus access-code-gated live runs through a backend API.

MealCheck is a client-facing verification product, not an internal operations
dashboard. The homepage should help a reviewer answer one question quickly:

`Can I check a meal plan now, and can I trust the resulting decision?`


### 1. Make The Live Check The Product

The homepage should prioritize the live verification workflow. Demos, reports,
and explanatory material support the product, but they are not the default job.
This keeps the first screen focused and limits choice overload.

### 2. Reduce Cognitive Load With Progressive Disclosure

MealCheck has more state than the user should have to parse at once: service
access, optional access code for private deployments, nutrition targets,
verification constraints, meal-plan entry mode, provider settings, progress
events, and report artifacts. Group related controls, reveal mode-specific
controls only when needed, and hide operational detail behind plain-language
disclosures.

### 3. Make System State Impossible To Miss

Because the frontend is static and the backend is optional at page load, service
state must be visible in both the shell and the live-run workspace. Example,
ready, and unavailable states should each have distinct copy, color, and next
action affordances.

### 4. Separate Outcome From Evidence

The final decision, risk level, warnings, and short summary should be visible
before raw evidence. Detailed checks, nutrition totals, source citations, event
logs, and the downloadable report remain inspectable, but they should not
compete with the decision hierarchy.

### 5. Design Around Flows, Not Screens

Use real-world product references for flows such as create, progress, complete,
review, settings, and confirmation. The live check should feel like one
continuous path: enter a plan, create a report, review the result, and delete
the report when needed.

### 6. Use A Small Component System

Favor a compact set of reusable primitives: button variants, status pills,
chips, fields, notices, dialogs, tabs, metrics, and panels. The goal is
consistent behavior and spacing, not a large design framework.

### 7. Keep The Visual Tone Operational And Trustworthy

MealCheck should look credible, calm, and evidence-oriented. Avoid playful game
visuals, marketing hero composition, saturated palettes, and decorative imagery.
Use accent color only to clarify status and primary action.

### 8. Treat Static Deployment As A First-Class Constraint

The page must remain useful when hosted as static files. Runtime API
configuration should be visible. Demos must work offline. Live-run controls must
degrade clearly when no backend is configured or the backend is unavailable. No
provider key or access code may be persisted.

### 9. Give Immediate Feedback

Every meaningful action should acknowledge state quickly: health checks,
validation errors, run submission, queued/running/completed states, failed runs,
artifact availability, and deletion. Avoid silent waits.

### 10. Make Risky Actions Deliberate

Provider keys, access codes, live-check content, and deletion need conservative
handling. Destructive actions should be disabled when impossible and confirmed
when they affect a selected report.

## Visual Identity

MealCheck's identity should read as an evidence-backed audit console for meal
plans. It should not look like a wellness tracker, recipe app, diet coach,
medical product, or generic AI landing page.

### Identity Attributes

MealCheck should feel:

- trustworthy
- calm
- precise
- transparent
- operational

The product should avoid:

- wellness green as the primary brand color
- food photography as a core identity device
- fork, spoon, leaf, apple, or plate-only stock marks
- playful diet-app illustrations
- hospital or clinical styling
- glossy AI gradients and abstract decoration

### Brand Mark Direction

Use a small code-native mark that centers the recognizable visual play between a
blue `M` and a green check mark. The mark should stay streamlined: no seal
field, plate ring, side brackets, or secondary audit symbols in the primary
top-bar mark. Those motifs can appear elsewhere in the interface, but the brand
mark should reduce to the monogram and verification stroke. Prefer a deliberate
inline SVG mark over fragile CSS-only construction when precision matters. It
should work at small sizes and next to the `MealCheck` wordmark in the static top
bar.

The mark should be functional enough to reuse for:

- the top-bar product identity
- live-run navigation
- report artifact rows
- source and evidence references
- future favicon/app-icon exploration

### Color Roles

Color must separate identity from status:

- brand/evidence: deep blue used for the mark, links, active navigation, and
  primary action emphasis
- neutral base: cool grays and white surfaces for the operational workspace
- pass: green only for successful checks and healthy backend/run states
- warn: amber only for incomplete, waiting, or caution states
- block/error: red only for failures, destructive actions, and blocked checks
- data support: muted slate or teal only where secondary graphics need another
  non-status color

Do not use green as the brand color because green already carries pass/success
meaning in verification results.

MealCheck uses the Scientific Ledger palette:

- ink: `#172a35`
- brand navy: `#123a52`
- brand blue: `#1f6f8b`
- evidence blue-gray: `#ddeaf0`
- page background: `#f5f7f8`
- surface: `#ffffff`
- non-status teal accent: `#2d7a73`

Pass, warn, and block colors remain separate from these identity tokens.

### Graphic Language

Graphics should be audit artifacts, not decoration. Favor:

- status chips and report-readiness indicators
- evidence/source marks near citations
- nutrient bars tied to explicit targets
- artifact/document glyphs for generated files
- compact bounded panels with clear labels

Avoid:

- large hero imagery
- stock meal photos
- decorative blobs, bokeh, or ambient gradients
- icons without text labels
- graphics that imply the app gives medical advice

### Voice

Copy should be factual and bounded. Prefer verbs like `check`, `verify`,
`resolve`, `review`, `export`, and `delete`. Avoid promising optimization,
health outcomes, personalized nutrition advice, or clinical certainty.

### Typesetting

MealCheck should use type to distinguish product identity from operational UI:

- wordmark and major page titles: a sober serif system stack for a more
  distinctive and editorial product signature
- body, forms, and controls: an identity-forward sans stack that prefers IBM
  Plex Sans, then static-safe platform fallbacks such as Avenir Next and Aptos
- status pills, chips, metadata, and stage labels: a compact monospace stack
  that prefers IBM Plex Mono, then platform monospace fallbacks, to reinforce
  the audit-console character

Do not load remote fonts for the static MVP. If a custom typeface is introduced
later, self-host it and keep the page useful when the font fails to load.

## Layout And Graphics Hardening

The remaining Milestone 9 design work should improve hierarchy and scanning
without making the static page feel like a marketing site.

### Compact The First Viewport

The live-check page should show service state, the primary create action, and
report status without requiring a desktop user to scroll through the entire
form. Use a compact action/status strip near the top of the workspace. The full
form can continue below it.

### Use Progressive Disclosure For Advanced Inputs

The default public path should expose only the controls most reviewers need to
start a run:

- meal-plan text
- model provider settings
- optional verification settings for nutrition targets, day and meal count,
  allergies, excluded foods, nutrient thresholds, and prep-safety requirement

Access code entry should appear only when the backend reports
`invite_required`.

Do not expose demographic profile fields or unused switches until the verifier
or provider prompt has a concrete use for them.

### Hide Internal Workflow Mechanics

Do not expose internal pipeline stages, event counts, run identifiers, API chips,
or artifact lists in the default live-check viewport. Use product language:

- Access code
- Create Report
- Delete Report
- Results
- Activity details

Use "Access code" only in invite-required deployments.

Activity details can disclose event messages after a report has started, but the
default state should stay focused on the next user action.

### Turn Run Status Into Results

The status panel should behave like a report-results surface:

- readable report state
- short result/progress message
- report readiness
- optional activity details behind disclosure
- delete affordance through the shared destructive confirmation path

### Use Functional Graphics

Graphics should explain the verification process rather than decorate the page.
Use small, code-native visuals for:

- brand identity
- source references
- artifact file types
- nutrient limit bars
- report readiness

Avoid stock images, decorative illustrations, gradient orbs, and large hero
graphics.

### Use Icons Sparingly

Use icons only when they make repeated controls easier to scan. Because the
static frontend has no icon library dependency today, icons should be
code-native UI marks rather than image assets. They must remain paired with text
labels.

For the live-check form and navigation, avoid decorative selector symbols. Plain
text controls are preferred unless an icon materially improves recognition.

### Keep Manual Entry Contained

Manual food entry should never push outside its panel. Desktop rows should use a
responsive grid with zero-min-width controls. At narrower widths, the live form
and results panel should stack before the row layout becomes cramped. Mobile rows
should become labeled controls rather than a horizontal table.

## Static Frontend Constraint

The web UI must keep working as a static site hosted from prebuilt assets.

Design implications:

- No server-rendered routes are required.
- The first page must be useful before any authenticated or personalized state
  exists.
- Backend availability is a runtime condition, not a page-loading dependency.
- Seeded demos must remain usable when the backend is offline.
- Live-run controls can call the configured API, but the UI cannot assume the
  API is healthy.
- Public configuration belongs in `config.json`, build-time public environment
  variables, URL query parameters, or non-secret HTML metadata.
- Provider keys, access codes, and live-run content must not be persisted in
  static assets or browser storage.

## Product Posture

The UI should feel:

- quiet and operational
- evidence-oriented
- stable under repeated use
- restrained about backend state; availability should appear through actions,
  disabled states, and errors rather than standalone service panels
- clear about pass, warn, and block outcomes

Avoid:

- landing-page hero layouts
- decorative gradients, blobs, and illustrations
- copy that implies medical advice or nutrition optimization
- broad food-database positioning
- account-dashboard patterns before accounts exist

## Homepage Priority

The homepage is the live-check workflow.

First-viewport priorities:

1. Brand identity.
2. New meal check navigation as the active primary surface.
3. A concise summary showing readiness and service availability.
4. A compact action/status strip with the primary report action visible.
5. The live-check form, starting with access, meal-plan text, and provider
   settings.
6. A companion results panel.
7. Seeded demo navigation as a secondary path.

Seeded reports are examples and public proof artifacts. They remain available
from navigation, but they should not be the default homepage state.

## Page Anatomy

Use this base layout:

```text
Top bar
MealCheck

Left navigation                  Main workspace
New meal check                   Live summary
Examples                         Report form
                                 Results
                                 Report surface when available
```

Desktop:

- Fixed-width left navigation.
- Main workspace uses a two-column live layout when space allows: form first,
  results second.
- The form and results panel stack before manual entry becomes cramped.
- Report content appears below the live workspace after a completed report.

Tablet and mobile:

- Navigation stacks above the workspace.
- Form, status, metrics, and report panels collapse to one column.
- Manual food rows become readable vertical groups.

## Visual System

The design system should use a small set of tokens:

- background: neutral gray, not cream or saturated color
- surface: white
- text: near-black
- muted text: neutral gray
- borders: cool gray
- action: blue
- pass: green
- warn: amber
- block/error: red

Typography:

- Use one sans-serif system stack.
- Keep `MealCheck` prominent but not hero-sized.
- Use compact section headings inside panels.
- Use 14-16px form text.
- Do not scale ordinary text with viewport width.

Spacing and shape:

- Use an 8px spacing rhythm.
- Keep cards and panels at 8px radius or less.
- Prefer borders and subtle elevation over heavy shadows.
- Do not nest decorative cards inside cards.

## Component Rules

Navigation:

- New meal check appears first.
- The active navigation item gets a clear accent and border.
- Examples remain visible but secondary.

Top bar:

- Show product name and short posture line.
- Do not expose a separate service-status card in the top bar.

Summary band:

- Live state uses a status pill and short service availability chip.
- Report state uses the decision pill, risk level, guideline pack, and a short
  decision summary.

Live form:

- Group fields in this order: Access, Meal Plan Text, Model Provider, optional
  Verification Settings.
- Keep labels short and visible.
- Keep the primary create action in a compact action/status strip near the top
  of the live-run workspace.
- Put threshold and policy details inside an advanced constraints disclosure.
- The destructive delete action should be disabled when no report exists.

Results:

- Keep status separate from the form.
- Show the current report state and a short message.
- Put event messages behind Activity details.
- Keep raw artifact links out of the client-facing report tabs.
- Do not hide errors inside report tabs.

Reports:

- Preserve the tabbed report surface.
- Decision status should be more visually important than downloads.
- The Report tab should expose one shareable report PDF download, not a raw
  artifact browser.
- Evidence and source links should remain inspectable without decoration-heavy
  UI.

## Accessibility And Robustness

- Every input must have a label.
- Focus states must be visible.
- Status colors must include text labels; color alone is not sufficient.
- Text must not overlap or overflow its control on mobile or desktop.
- Buttons must have clear disabled states.
- Tables and wide artifact areas must support horizontal scrolling.
- Static demo content must render without a backend API base.

## Milestone 9 Acceptance

Web design hardening is accepted when:

- the homepage opens on the live-run workflow
- `web-design.md` codifies MealCheck's visual identity, color-role boundaries,
  mark direction, graphic language, and voice
- the top bar includes a compact MealCheck brand mark that works without image
  assets
- brand/evidence color is visually distinct from pass, warn, and block status
  colors
- source, evidence, and artifact graphics reinforce the audit-console identity
  without decorative imagery
- the static page remains useful without backend access
- seeded demos remain reachable through navigation
- service health is not presented as a standalone client-facing box
- example and unavailable service states are communicated through disabled
  actions or error feedback without hiding seeded demos
- report creation gives immediate disabled or error feedback before an invalid
  request is submitted
- destructive report deletion requires an explicit confirmation
- the primary create action remains visible near the top of the live-check
  workspace
- advanced constraints are progressively disclosed
- default live-check status avoids pipeline graphics, run-event counts, raw
  artifact links, visible Service URL fields, and decorative selector symbols
- the live form and results panel are visually distinct on desktop
- the layout collapses cleanly on mobile
- manual entry stays contained on desktop and is readable as labeled rows or
  cards on mobile
- report tabs and the PDF report download still work after a live report
  completes
- typecheck, unit tests, browser tests, and production build pass
- local browser verification shows no console errors
