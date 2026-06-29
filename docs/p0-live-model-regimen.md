# P0 Live Local-Model Evaluation Regimen

This regimen is for evaluating MealCheck's P0 normalization task against a live
llama.cpp-compatible model endpoint on a prototyping laptop. It is intended for
fast iteration. It is not a substitute for final acceptance on the serving
MacBook.

## Goal

Measure whether the live local model can turn acceptable pasted meal-plan text
into compact MealCheck rows without losing source items, corrupting day or meal
assignment, changing quantities, changing units, or producing invalid compact
JSON.

The current checked-in P0 corpus is still a seed corpus:

- 8 success cases
- 3 qualification failure cases
- 120 expected source items
- supported units only: `g`, `oz`, `cup`, `tbsp`, `tsp`, `slice`, `serving`

The live-model regimen measures model extraction quality on those success
cases. Failure cases are still checked through deterministic qualification.

## Preconditions

Start one local model behind an OpenAI-compatible `/v1` endpoint. The default
endpoint is:

```text
http://127.0.0.1:11435/v1
```

The script expects:

- `curl`
- `git`
- `go`
- `jq`
- a running llama.cpp-compatible server
- a clean enough MealCheck worktree to make the run interpretable

The script does not start llama.cpp and does not download models.

## Fast Iteration Run

Use one repeat while changing prompts, schemas, parser logic, or model server
settings:

```bash
MEALCHECK_P0_REPEATS=1 \
MEALCHECK_P0_ALLOW_MISMATCH=1 \
scripts/run-p0-local-model-regimen.sh
```

This mode is for finding obvious failure classes. It should not be treated as a
pass/fail gate.

## Baseline Run

Use at least three repeats for a laptop baseline:

```bash
MEALCHECK_P0_REPEATS=3 \
MEALCHECK_P0_LOCAL_MODEL_BASE_URL=http://127.0.0.1:11435/v1 \
MEALCHECK_P0_LOCAL_MODEL_NAME="$MODEL_NAME" \
scripts/run-p0-local-model-regimen.sh
```

If `MEALCHECK_P0_LOCAL_MODEL_NAME` is omitted, the script uses the first model
id from `/v1/models`.

The baseline gate requires:

- deterministic P0 eval passes first
- every live repeat exits successfully
- zero provider failures
- zero compact-output decode failures
- zero case mismatches
- `local_model_row_match_rate` is at least `MEALCHECK_P0_MIN_ROW_MATCH_RATE`
  which defaults to `1`

Useful optional knobs:

- `MEALCHECK_P0_OUTPUT_DIR`: result directory
- `MEALCHECK_P0_MAX_OUTPUT_TOKENS`: local-model output cap, default `1536`
- `MEALCHECK_P0_LOCAL_MODEL_TIMEOUT`: model request timeout, default `240s`
- `MEALCHECK_P0_CURL_MAX_TIME_SECONDS`: endpoint health-check timeout, default
  `20`
- `MEALCHECK_P0_REQUIRE_CLEAN_WORKTREE=1`: require no uncommitted changes

For release-candidate confidence on the prototyping laptop, run five repeats:

```bash
MEALCHECK_P0_REPEATS=5 \
scripts/run-p0-local-model-regimen.sh
```

## Captured Artifacts

Each run writes a timestamped directory under `/tmp` unless
`MEALCHECK_P0_OUTPUT_DIR` is set. The directory contains:

- `metadata.json`: git commit, branch, dirty status, model endpoint, model id,
  optional model SHA/build labels, Go version, OS, CPU, memory, repeat count,
  and gate threshold
- `models-response.json`: raw `/v1/models` response
- `git-status.txt`: short worktree status
- `deterministic-result.json`: offline P0 result
- `live-run-N.json`: local-model result for each repeat
- `live-summary.jsonl`: one compact result row per repeat
- `summary.json`: aggregate gate result
- stdout/stderr files for deterministic and live runs

Set these optional labels when available:

```bash
MEALCHECK_P0_MODEL_SHA=<gguf-sha256> \
MEALCHECK_P0_LLAMA_BUILD=<llama.cpp-version-or-commit> \
scripts/run-p0-local-model-regimen.sh
```

Those labels make laptop-to-server comparison much easier.

## Reading Results

Start with:

```bash
jq . /tmp/mealcheck-p0-local-model-*/summary.json
```

The highest-signal fields are:

- `gate.passed`
- `repeats_with_mismatches`
- `min_local_model_row_match_rate`
- `total_provider_failures`
- `total_decode_failures`
- `mismatch_case_ids`
- `max_duration_seconds`

Then inspect mismatched run files:

```bash
jq '.mismatches' /tmp/mealcheck-p0-local-model-*/live-run-*.json
```

Interpret failures by class:

- provider failures: model endpoint, timeout, server crash, or request
  incompatibility
- decode failures: model did not return valid compact MealCheck JSON
- row mismatches: model returned parseable rows but changed item count, day,
  meal, food phrase, quantity, or unit
- deterministic failures: source inventory or adapter logic changed before the
  model was tested

## Regression Risk Discipline

The prototyping laptop is useful for fast iteration because it has better
hardware, but it can hide production failures. Treat laptop results as a
development baseline only.

Before promoting a prompt, schema, model, llama.cpp build, or extraction logic
change, repeat the same P0 regimen on the serving MacBook with:

- same MealCheck commit
- same GGUF file and SHA
- same llama.cpp build
- same endpoint configuration
- same context size, max output tokens, timeout, thread/GPU settings, and prompt
  cache settings where applicable

The serving MacBook remains authoritative for latency, capacity, timeout, and
queue-risk decisions.
