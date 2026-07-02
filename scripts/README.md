# Scripts

This directory is for stable shell entry points. Python implementation and
Python command surfaces live under `tools/` so they can be packaged, imported,
and tested without turning `scripts/` into an unstructured tool dump.

## Categories

| Script | Category | Runtime expectation |
| --- | --- | --- |
| `run-local-model-deployment-profile.sh` | Deployment profile runner | Requires host-local Postgres and llama.cpp unless a documented fallback is selected |
| `run-p0-local-model-regimen.sh` | Local model regimen | Requires a running llama.cpp-compatible endpoint |
| `test-deployed-byok-live.sh` | Live deployment smoke | Requires deployed API access and provider keys |
| `test-deployed-local-model-live.sh` | Live deployment smoke | Requires deployed API access and server-owned local model configuration |
| `test-local-llama-structured-json.sh` | Local model smoke | Requires a running llama.cpp-compatible endpoint |
| `test-meal-plan-input-robustness.sh` | Local model robustness harness | Requires `jq` and delegates to `test-local-llama-structured-json.sh` |

## Ownership Rules

- Keep shell orchestration scripts here when they are meant to be run directly.
- Do not add Python files here. Put Python command surfaces under
  `tools/mealcheck_ops` or `tools/mealcheck_data`.
- Put substantial Python logic under `tools/mealcheck_ops` or
  `tools/mealcheck_data`.
- Do not commit generated caches such as `__pycache__/` or `*.pyc`.
- Every new script should state whether it is CI-safe, local-only,
  live-deployment-only, or dependent on llama.cpp/provider keys.

## Python Tool Commands

Run operator tools through `mealcheck_ops`:

```bash
PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops compare-eval-exports --help

PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops summarize-run-artifacts --help

PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops compare-model-runs --help
```

Run data-generation tools through `mealcheck_data`:

```bash
PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-p0-normalization-evaluation --help

PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-fndds-evaluation --help

PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-wweia-nhanes-evaluation --help

PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data import-fndds-reference --help
```
