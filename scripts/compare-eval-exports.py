#!/usr/bin/env python3
"""Compatibility wrapper for the MealCheck eval export comparison CLI."""

from __future__ import annotations

import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
PACKAGE_SRC = REPO_ROOT / "tools" / "mealcheck_ops" / "src"
sys.path.insert(0, str(PACKAGE_SRC))

from mealcheck_ops.cli import compare_eval_exports_main  # noqa: E402


if __name__ == "__main__":
    raise SystemExit(compare_eval_exports_main())
