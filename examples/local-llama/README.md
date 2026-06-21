# Local llama.cpp Trial Fixture

These files support the local llama.cpp model trial matrix.

- `synthetic-meal-plan.txt` is a small synthetic meal-plan datapoint. It is
  designed to test whether a local model can normalize concrete meal-plan text
  into MealCheck JSON.
- `meal-plan-response.schema.json` is an inline compact verifier schema for
  llama.cpp schema-constrained JSON decoding. It intentionally avoids `$ref`
  and omits optional `description`, `shopping_list`, and redundant resolved-item
  fields to keep local generation latency bounded.

This fixture is a smoke datapoint, not a full evaluation set. A model must pass
this before it is worth testing larger synthetic or manually reviewed examples.

Manual server shape:

```bash
/Users/chranama-server/llama.cpp/build/bin/llama-server \
  -m /Users/chranama-server/MealCheck-data/models/<candidate>.gguf \
  --host 127.0.0.1 \
  --port 11435 \
  --threads 3 \
  --ctx-size 4096 \
  --batch-size 256 \
  --ubatch-size 64 \
  --gpu-layers 0
```

In another shell from the MealCheck repository root:

```bash
LLAMA_MODEL='<candidate-label>' \
MEALCHECK_LLAMA_REPEATS=3 \
scripts/test-local-llama-structured-json.sh
```

Candidate classes to test first:

- 1B or smaller: proves latency floor, likely weak normalization quality.
- 1.5B to 1.7B: likely first viable target for this MacBook.
- 3B to 4B: quality check only if memory and responsiveness remain acceptable.
