# Documentation

This directory contains the current technical documentation for MealCheck.

The docs are intentionally small. Planning notes, open questions, and
architecture decisions should be folded into the focused files below instead of
spreading across separate ADR, RFC, and planning documents.

## Documents

- [Product](product.md): problem, users, scope, non-goals, and success criteria.
- [User Story](user-story.md): MVP user story, flow, LLM role, checks, and acceptance criteria.
- [Nutritional Guidelines](nutritional-guidelines.md): source selection, guideline-pack preprocessing, and meal-plan JSON normalization.
- [Privacy And Safety](privacy-and-safety.md): data minimization, retention, disclosure, safety boundaries, and live-run controls.
- [Contracts](contracts.md): meal-plan, guideline-pack, API, decision, report, and artifact contracts.
- [CLI](cli.md): local commands, artifact generation, decision exit codes, and access-code administration.
- [API](api.md): hosted API endpoints, request modes, response shapes, lifecycle, and runtime limits.
- [Architecture](architecture.md): checker engine, guideline sources, hosted wrapper, local model serving, BYOK extension points, and server shape.
- [Web Design](web-design.md): static frontend design posture, layout, component rules, and hardening criteria.
- [Backend Server](backend_server.md): MacBook server requirements, required programs, and setup checklist.
- [Implementation Plan](implementation-plan.md): milestones, first proof path, and acceptance criteria.
- [Runbook](runbook.md): development and MacBook-hosted deployment operations.
- [Decision Log](decision-log.md): accepted decisions and tradeoffs.
- [Seeded HTML Report](seeded-report.html): standalone static report for the checked-in proof case.
- [Deployment Package](../deploy/README.md): local MacBook, Cloudflare, Postgres, and process-supervision templates.

## Documentation Rule

A document should exist only if it answers one of these questions:

- What is this project?
- What is the external contract?
- How is it built?
- How do I run or operate it?
- What has been decided?
- What is the next implementation slice?
