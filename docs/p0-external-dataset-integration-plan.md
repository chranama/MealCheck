# P0 External Dataset Integration Plan

This plan integrates NYT Ingredient Phrase Tagger and TASTEset into MealCheck's
P0 meal-plan normalization evaluation framework. The target is evaluation
coverage, not model training and not P1 nutrition resolution.

## Context

Current P0 evaluation uses the checked-in `p0-normalization-v1` seed corpus:

- 8 acceptable-input success cases
- 3 qualification-failure cases
- 120 expected source items
- reviewed MealCheck robustness examples only

The generator already has early optional readers for:

- `MEALCHECK_NYT_INGREDIENTS_CSV`
- `MEALCHECK_TASTESET_CSV`

Current implementation status:

- `scripts/generate-p0-normalization-evaluation.py --probe-sources` validates
  local NYT/TASTEset paths, required columns, row counts, and source SHA256.
- The generator writes strict reviewed seed files separately from optional
  exploratory external files.
- NYT and TASTEset adapters emit success, failure, and quarantine artifacts
  when local source CSVs are provided.
- `manifest.json` supports rich file entries with `path`, `source_dataset`,
  and `gate`.
- `mealcheck eval-normalization` supports `-gate` and `-source-dataset`.
- Eval results include `gate_summary`, `source_dataset_summary`, and
  `quarantine_summary`.
- `fixture-check` validates the expanded P0 manifest and quarantine rows.

Remaining work before promoting external data into a release gate:

- run against full local NYT and TASTEset source files
- manually review a small generated sample before committing it
- decide whether any external subset should move from exploratory to strict
- tune TASTEset label handling after observing real source rows

## Source Roles

NYT Ingredient Phrase Tagger should be the first external source. It provides a
large ingredient-phrase CSV with structured fields such as quantity, unit,
name, comment, and original input. It is best for high-volume tests of quantity,
unit, and food-name extraction.

TASTEset should be the second source. It is a recipe NER benchmark with entity
types such as food products, quantities, units, cooking processes, and physical
qualities. It is best for harder span-boundary tests around preparation words,
quality words, and recipe-like language that must be filtered before becoming a
MealCheck success case.

Neither dataset contains full MealCheck meal plans. MealCheck should wrap
selected ingredient phrases in deterministic synthetic day/meal contexts and
evaluate whether normalization preserves the item-level gold plus generated day
and meal structure.

## Integration Principles

- Keep raw third-party datasets out of the repository unless license, size, and
  redistribution have been explicitly approved.
- Check in a small reviewed generated sample only after source metadata and
  generation are reproducible.
- Keep the existing reviewed seed corpus as the strict release gate.
- Treat large generated NYT/TASTEset evals as opt-in exploratory baselines
  until their sampling, expected rows, and failure classes have been reviewed.
- Preserve source refs and source hashes so failures can be traced back to the
  external row without committing raw external data.
- Make unsupported, vague, ranged, optional, and recipe-like rows explicit
  failure or quarantine cases instead of silently dropping them.

## Target Artifact Shape

Keep the reviewed seed files stable:

```text
data/evaluation/p0-normalization/cases-v1.jsonl
data/evaluation/p0-normalization/failure-cases-v1.jsonl
```

Add generated external files:

```text
data/evaluation/p0-normalization/nyt-cases-v1.jsonl
data/evaluation/p0-normalization/nyt-failure-cases-v1.jsonl
data/evaluation/p0-normalization/tasteset-cases-v1.jsonl
data/evaluation/p0-normalization/tasteset-failure-cases-v1.jsonl
data/evaluation/p0-normalization/quarantine-v1.jsonl
```

Update the manifest to record every case file, whether it participates in the
strict gate, and source metadata:

```json
{
  "case_files": [
    {"path": "cases-v1.jsonl", "source_dataset": "mealcheck_input_robustness", "gate": "strict"},
    {"path": "nyt-cases-v1.jsonl", "source_dataset": "nyt_ingredient_phrase_tagger", "gate": "exploratory"},
    {"path": "tasteset-cases-v1.jsonl", "source_dataset": "tasteset", "gate": "exploratory"}
  ],
  "failure_case_files": [
    {"path": "failure-cases-v1.jsonl", "source_dataset": "mealcheck_input_robustness", "gate": "strict"},
    {"path": "nyt-failure-cases-v1.jsonl", "source_dataset": "nyt_ingredient_phrase_tagger", "gate": "exploratory"},
    {"path": "tasteset-failure-cases-v1.jsonl", "source_dataset": "tasteset", "gate": "exploratory"}
  ]
}
```

If keeping the existing manifest schema is cheaper for the first pass, write
aggregate generated files instead:

```text
generated-external-cases-v1.jsonl
generated-external-failure-cases-v1.jsonl
```

The cleaner target is multiple case files plus runner support for manifest
arrays.

## Slice 1: Source Acquisition And Probe

Add source-probe commands to verify local source files before generation.

NYT probe should validate:

- file exists and is readable
- expected columns are present: at minimum `qty`, `unit`, `name`, and preferably
  `input` and `comment`
- row count
- license/source URL recorded
- SHA256 recorded in `source-manifest.json`
- counts by parse status: success candidate, unsupported unit, missing
  quantity, vague quantity, range quantity, missing food, comment-heavy,
  quarantined

TASTEset probe should validate:

- file or directory exists and is readable
- actual source schema is detected instead of assumed
- recipe/ingredient text and entity annotations can be joined
- supported labels are mapped: `QUANTITY`, `UNIT`, `FOOD`, plus useful
  preparation/quality labels when present
- row or recipe count
- license/source URL recorded
- SHA256 recorded in `source-manifest.json`
- counts by parse status and quarantine reason

Deliverables:

- `scripts/generate-p0-normalization-evaluation.py --probe-sources`:
  implemented.
- source-schema validation through probe mode and a generated temporary fixture
  smoke run: implemented.
- updated source manifest with local path, source URL, license note, and SHA
  fields when external source files are provided: implemented.

## Slice 2: Generator Refactor

Refactor the generator into explicit source adapters:

- `MealCheckRobustnessAdapter`
- `NYTIngredientPhraseAdapter`
- `TASTEsetAdapter`

Each adapter should emit a common intermediate record:

```json
{
  "source_dataset": "nyt_ingredient_phrase_tagger",
  "source_ref": {"row_number": 123, "source_hash": "..."},
  "raw_text": "1/2 cup fresh thyme leaves, finely chopped",
  "quantity_text": "1/2",
  "quantity": 0.5,
  "unit_text": "cup",
  "unit": "cup",
  "food": "fresh thyme leaves",
  "prep_or_quality": "finely chopped",
  "status": "success_candidate",
  "reason": ""
}
```

Classification statuses:

- `success_candidate`
- `unsupported_unit`
- `missing_quantity`
- `vague_quantity`
- `range_quantity`
- `optional_or_alternative`
- `recipe_instruction`
- `missing_food`
- `ambiguous_food`
- `schema_error`

Deliverables:

- source adapters for NYT and TASTEset: implemented.
- deterministic status counts in `source-manifest.json`: implemented.
- stable intermediate-record JSONL: deferred; success, failure, and quarantine
  outputs currently provide the reviewable artifacts.

## Slice 3: Success Case Generation

Generate MealCheck success cases only from `success_candidate` rows.

Sampling rules:

- fixed seed
- stable sorted source refs
- stratify by source dataset
- stratify by unit: `g`, `oz`, `cup`, `tbsp`, `tsp`, `slice`, `serving`
- stratify by quantity style: integer, decimal, fraction, mixed number
- stratify by food phrase shape: one-token, multi-token, prep adjective,
  quality adjective, comment-derived modifier
- cap repeated foods and repeated units so one common pattern does not dominate

Wrapper styles:

- one-day canonical bullets
- one-day inline sentences
- numbered list items
- two-day clear `Day N` sections
- one-day snack-inclusive plans
- compact multi-day text
- natural rewrites with `with`, `plus`, commas, and `of`

Start with a reviewed checked-in sample:

- NYT: 100 success cases
- TASTEset: 100 success cases
- each success case should contain 3, 6, 9, 12, or 18 source items depending on
  wrapper style

Large local-only runs can use higher limits, but should not be committed until
we understand quality and runtime.

Deliverables:

- generated external success JSONL files: implemented.
- per-case tags for source dataset, wrapper, unit, and quantity style:
  implemented.
- deterministic source refs for every expected source item: implemented.

## Slice 4: Failure And Quarantine Generation

Generate explicit failure cases instead of dropping all non-success rows.

Failure cases:

- `unsupported_unit`: quantified food with unsupported unit
- `vague_quantity`: handful, pinch, dash, small, medium, large, to taste, as
  needed
- `range_quantity`: `1 to 2`, `1-2`, `1 or 2`
- `quantity_missing`: food phrase without numeric quantity
- `recipe_or_menu_needs_decomposition`: recipe instruction or composed dish
  text that is not ingredient-level

Quarantine cases:

- rows where the expected food phrase is debatable
- rows with alternatives, optional ingredients, or substitutions
- rows where source annotations conflict
- rows whose source license or provenance is unclear

Deliverables:

- external failure JSONL files: implemented.
- per-source quarantine JSONL with `source_dataset`, `source_ref`, `raw_text`,
  and `quarantine_reason`: implemented.
- eval runner does not treat quarantine rows as pass/fail cases: implemented.

## Slice 5: Eval Runner Support

Extend `mealcheck eval-normalization` so it can run:

- strict seed only
- one external source only
- all checked-in P0 cases
- large local generated files

Suggested CLI:

```bash
go run ./cmd/mealcheck eval-normalization \
  -manifest data/evaluation/p0-normalization/manifest.json \
  -gate strict

go run ./cmd/mealcheck eval-normalization \
  -manifest data/evaluation/p0-normalization/manifest.json \
  -gate exploratory \
  -source-dataset nyt_ingredient_phrase_tagger

go run ./cmd/mealcheck eval-normalization \
  -manifest data/evaluation/p0-normalization/manifest.json \
  -gate all
```

Runner changes:

- read manifest `case_files` and `failure_case_files`
- keep existing `-dataset` and `-failures` flags as override shortcuts
- add per-dataset summaries
- add per-tag summaries that remain source-specific
- preserve strict gate result separately from exploratory result
- emit counts for generated, reviewed, failure, and quarantine artifacts

Deliverables:

- runner tests with a multi-file manifest fixture: implemented.
- fixture-check coverage for the expanded manifest shape: implemented.
- result JSON includes `gate_summary`, `source_dataset_summary`, and
  `quarantine_summary`: implemented.

## Slice 6: Validation And Review Workflow

Add a repeatable validation workflow:

```bash
python3 scripts/generate-p0-normalization-evaluation.py \
  --nyt-csv "$MEALCHECK_NYT_INGREDIENTS_CSV" \
  --tasteset-csv "$MEALCHECK_TASTESET_CSV" \
  --nyt-limit 100 \
  --tasteset-limit 100

go run ./cmd/mealcheck fixture-check

go run ./cmd/mealcheck eval-normalization \
  -manifest data/evaluation/p0-normalization/manifest.json \
  -gate strict

go run ./cmd/mealcheck eval-normalization \
  -manifest data/evaluation/p0-normalization/manifest.json \
  -gate exploratory
```

For live local-model runs, keep the strict seed as the release gate first.
External generated cases should initially be reported as exploratory:

```bash
MEALCHECK_P0_REPEATS=1 \
MEALCHECK_P0_ALLOW_MISMATCH=1 \
scripts/run-p0-local-model-regimen.sh
```

Only promote an external subset to release-gate status after manual review of:

- expected source item correctness
- failure-category correctness
- model mismatch classes
- deterministic repair counts
- latency and artifact size

## Slice 7: Gate Promotion

Promotion order:

1. Keep `mealcheck_input_robustness` as the strict P0 release gate.
2. Add NYT reviewed sample as a non-blocking dashboard metric.
3. Promote a small NYT subset to strict after two clean implementation cycles.
4. Add TASTEset reviewed sample as non-blocking.
5. Promote only the least ambiguous TASTEset subset to strict; keep harder
   recipe-like span-boundary cases exploratory.

Do not require the full external generated corpus to pass before release.
Large external datasets are for finding weaknesses and prioritizing work, not
for blocking every small fix.

## Implementation Order

1. Add source probe and source hash metadata.
2. Refactor generator into adapters with tiny source fixtures.
3. Add NYT success, failure, and quarantine generation.
4. Add TASTEset success, failure, and quarantine generation.
5. Extend manifest and eval runner for multiple files, gates, and per-source
   summaries.
6. Generate a small reviewed external sample and inspect diffs manually.
7. Run deterministic eval for strict and exploratory gates.
8. Run one-repeat live local-model exploratory eval and rank failure classes.
9. Decide which external subset, if any, should become strict.

## Initial Acceptance Criteria

- Raw external datasets are not committed.
- Source manifest records URL, license, local path/env var, row count, and
  SHA256 for each source file.
- Generation is deterministic across runs for the same source files and limits.
- `go run ./cmd/mealcheck fixture-check` validates generated artifacts.
- Strict seed P0 gate remains unchanged and passing.
- External eval can be run separately by source dataset.
- Result JSON reports per-source success rate, per-field accuracy, repair
  counts, and failure categories.
- Quarantine rows are visible but do not affect pass/fail counts.

## Open Decisions

- Whether to commit a reviewed generated external sample, and if so how large.
- Whether the manifest should use object entries for case files immediately or
  preserve string entries and add a parallel metadata block.
- Whether unsupported-unit external failures should stop at source inventory,
  qualification, or unresolved-quantity preservation.
- Whether source comments from NYT should become part of the expected food
  phrase, become `preparation`, or be excluded from success cases.
- Which TASTEset quality/process labels are safe to preserve in P0 food phrases
  without turning recipe decomposition into a P0 success requirement.
