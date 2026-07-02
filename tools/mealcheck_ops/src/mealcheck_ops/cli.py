"""Command-line entry points for MealCheck operator tooling."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Sequence

from mealcheck_ops.eval_exports import CompareError, compare_exports, render_markdown
from mealcheck_ops.run_artifacts import (
    ArtifactSummaryError,
    render_artifact_markdown,
    summarize_run_artifacts,
)


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if not args or args[0] in {"-h", "--help"}:
        print(
            "usage: python -m mealcheck_ops "
            "{compare-eval-exports,summarize-run-artifacts} [options]",
            file=sys.stderr,
        )
        return 0 if args else 2

    command, command_args = args[0], args[1:]
    if command == "compare-eval-exports":
        return compare_eval_exports_main(command_args)
    if command == "summarize-run-artifacts":
        return summarize_run_artifacts_main(command_args)

    print(f"mealcheck_ops: unknown command {command!r}", file=sys.stderr)
    return 2


def compare_eval_exports_main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Compare MealCheck eval export JSONL rows.")
    parser.add_argument("--baseline", required=True, help="baseline JSONL export path")
    parser.add_argument("--current", required=True, help="current JSONL export path")
    parser.add_argument("--out", help="optional machine-readable JSON output path")
    parser.add_argument("--markdown", help="optional Markdown report output path")
    args = parser.parse_args(argv)

    try:
        result = compare_exports(Path(args.baseline), Path(args.current))
        encoded = json.dumps(result, indent=2, sort_keys=False) + "\n"
        if args.out:
            Path(args.out).write_text(encoded, encoding="utf-8")
        else:
            sys.stdout.write(encoded)
        if args.markdown:
            Path(args.markdown).write_text(render_markdown(result), encoding="utf-8")
    except CompareError as err:
        print(f"compare-eval-exports failed: {err}", file=sys.stderr)
        return 2
    return 0


def summarize_run_artifacts_main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Summarize MealCheck run artifacts into a review queue.")
    parser.add_argument(
        "--artifact-root",
        default=".mealcheck-data/artifacts",
        help="artifact root, single run artifact directory, or single artifact evidence file",
    )
    parser.add_argument("--out", help="optional machine-readable JSON output path")
    parser.add_argument("--markdown", help="optional Markdown report output path")
    args = parser.parse_args(argv)

    try:
        result = summarize_run_artifacts(Path(args.artifact_root))
        encoded = json.dumps(result, indent=2, sort_keys=False) + "\n"
        if args.out:
            Path(args.out).write_text(encoded, encoding="utf-8")
        else:
            sys.stdout.write(encoded)
        if args.markdown:
            Path(args.markdown).write_text(render_artifact_markdown(result), encoding="utf-8")
    except ArtifactSummaryError as err:
        print(f"summarize-run-artifacts failed: {err}", file=sys.stderr)
        return 2
    return 0
