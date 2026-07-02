"""Module entry point for MealCheck data tooling."""

from __future__ import annotations

import sys
from collections.abc import Callable, Sequence

from mealcheck_data import generate_fndds_evaluation
from mealcheck_data import generate_p0_normalization_evaluation
from mealcheck_data import generate_wweia_nhanes_evaluation
from mealcheck_data import import_fndds_reference


COMMANDS: dict[str, Callable[[], None]] = {
    "generate-fndds-evaluation": generate_fndds_evaluation.main,
    "generate-p0-normalization-evaluation": generate_p0_normalization_evaluation.main,
    "generate-wweia-nhanes-evaluation": generate_wweia_nhanes_evaluation.main,
    "import-fndds-reference": import_fndds_reference.main,
}


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if not args or args[0] in {"-h", "--help"}:
        names = ",".join(COMMANDS)
        print(f"usage: python -m mealcheck_data {{{names}}} [options]", file=sys.stderr)
        return 0 if args else 2

    command = args[0]
    run = COMMANDS.get(command)
    if run is None:
        print(f"mealcheck_data: unknown command {command!r}", file=sys.stderr)
        return 2

    sys.argv = [f"mealcheck_data {command}", *args[1:]]
    run()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
