# MealCheck UI

This directory is a static frontend for the seeded public demo.

It has no build step. Deploy `ui/` as the Cloudflare Pages static root.

Local preview:

```bash
python3 -m http.server 4173 --directory ui
```

Then open:

```text
http://localhost:4173
```

Seeded artifacts live under `demo-runs/`. Refresh the current demo bundle from
the repository root with:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out ui/demo-runs/seeded-3-day-peanut-allergy
```

The refresh command exits `1` because the seeded plan intentionally produces a
`block` decision after writing the artifact bundle.
