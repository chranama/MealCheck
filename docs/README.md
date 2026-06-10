# Documentation

This directory contains the current technical documentation for MealCheck.

The docs are intentionally small. Planning notes, open questions, and
architecture decisions should be folded into the focused files below instead of
spreading across separate ADR, RFC, and planning documents.

## Documents

- [Product](product.md): problem, users, scope, non-goals, and success criteria.
- [Contracts](contracts.md): meal-plan, guideline-pack, API, decision, report, and artifact contracts.
- [Architecture](architecture.md): checker engine, guideline sources, hosted wrapper, BYOK execution, and server shape.
- [Backend Server](backend_server.md): MacBook server requirements, required programs, and setup checklist.
- [Implementation Plan](implementation-plan.md): milestones, first proof path, and acceptance criteria.
- [Runbook](runbook.md): development and MacBook-hosted deployment operations.
- [Decision Log](decision-log.md): accepted decisions and tradeoffs.

## Documentation Rule

A document should exist only if it answers one of these questions:

- What is this project?
- What is the external contract?
- How is it built?
- How do I run or operate it?
- What has been decided?
- What is the next implementation slice?
