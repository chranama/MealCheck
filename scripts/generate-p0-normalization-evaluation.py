#!/usr/bin/env python3
"""Generate MealCheck P0 meal-plan normalization evaluation cases.

The script always regenerates a small reviewed seed from
examples/meal-plan-input-robustness. When optional public source datasets are
available locally, it can append generated cases from NYT Ingredient Phrase
Tagger and TASTEset.
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable


DATASET_ID = "p0-normalization-v1"
OUT_DIR = Path("data/evaluation/p0-normalization")
ROBUSTNESS_DIR = Path("examples/meal-plan-input-robustness")
SUPPORTED_UNITS = {
    "g": "g",
    "gram": "g",
    "grams": "g",
    "oz": "oz",
    "ounce": "oz",
    "ounces": "oz",
    "cup": "cup",
    "cups": "cup",
    "tbsp": "tbsp",
    "tablespoon": "tbsp",
    "tablespoons": "tbsp",
    "tsp": "tsp",
    "teaspoon": "tsp",
    "teaspoons": "tsp",
    "slice": "slice",
    "slices": "slice",
    "serving": "serving",
    "servings": "serving",
}
MEAL_CODES = {
    "breakfast": "b",
    "morning snack": "m",
    "lunch": "l",
    "afternoon snack": "a",
    "dinner": "d",
    "evening snack": "e",
    "snack": "s",
}

QUANTITY_PATTERN = r"(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)"
UNIT_PATTERN = r"(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)"
RESOLVED_ITEM_LINE_PATTERN = re.compile(
    rf"^\s*(?:[-*]|\d+[.)])\s+{QUANTITY_PATTERN}\s*{UNIT_PATTERN}\b",
    re.IGNORECASE,
)
INLINE_ITEM_PATTERN = re.compile(
    rf"^\s*({QUANTITY_PATTERN})\s+(({UNIT_PATTERN})\s+)?(.+?)\s*$",
    re.IGNORECASE,
)
INLINE_BOUNDARY_PATTERN = re.compile(
    rf"\s+\b(?:and|with|plus)\s+(({QUANTITY_PATTERN})\s+)",
    re.IGNORECASE,
)
DAY_PATTERN = re.compile(r"\bday\s*([1-7])\b", re.IGNORECASE)
SOURCE_ITEM_MARKER_PATTERN = re.compile(r"^\s*(?:[-*]|\d+[.)])\s+")
PARSED_SOURCE_PATTERN = re.compile(rf"^\s*({QUANTITY_PATTERN})\s+(\S+)\s+(.+?)\s*$", re.IGNORECASE)


@dataclass
class SourceItem:
    source_item_id: int
    day: int
    meal_code: str
    source_text: str
    food: str
    quantity: float
    unit: str


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out-dir", default=str(OUT_DIR))
    parser.add_argument("--robustness-dir", default=str(ROBUSTNESS_DIR))
    parser.add_argument("--nyt-csv", default=os.environ.get("MEALCHECK_NYT_INGREDIENTS_CSV", ""))
    parser.add_argument("--tasteset-csv", default=os.environ.get("MEALCHECK_TASTESET_CSV", ""))
    parser.add_argument("--nyt-limit", type=int, default=0)
    parser.add_argument("--tasteset-limit", type=int, default=0)
    args = parser.parse_args()

    out_dir = Path(args.out_dir)
    success_cases: list[dict[str, Any]] = []
    failure_cases: list[dict[str, Any]] = []

    robust_success, robust_failures = load_robustness_cases(Path(args.robustness_dir))
    success_cases.extend(robust_success)
    failure_cases.extend(robust_failures)

    if args.nyt_csv:
        success_cases.extend(load_nyt_cases(Path(args.nyt_csv), args.nyt_limit))
    if args.tasteset_csv:
        success_cases.extend(load_tasteset_cases(Path(args.tasteset_csv), args.tasteset_limit))

    write_artifacts(out_dir, success_cases, failure_cases)
    print(f"wrote {len(success_cases)} success and {len(failure_cases)} failure cases to {out_dir}")


def load_robustness_cases(root: Path) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
    success_cases: list[dict[str, Any]] = []
    for case in manifest["cases"]:
        text = (root / case["file"]).read_text(encoding="utf-8").strip()
        source_items = resolved_source_items(text)
        success_cases.append(
            {
                "schema_version": "0.1",
                "id": f"robustness_{case['id']}",
                "source_dataset": "mealcheck_input_robustness",
                "source_ref": {
                    "file": str(Path("examples/meal-plan-input-robustness") / case["file"]),
                    "case_id": case["id"],
                },
                "input_text": text,
                "expected": {
                    "days": sorted({item.day for item in source_items}),
                    "source_items": [source_item_json(item) for item in source_items],
                },
                "tags": ["success", "reviewed_seed", *case.get("coverage_tags", [])],
            }
        )

    failure_manifest = json.loads((root / "failure-manifest.json").read_text(encoding="utf-8"))
    failure_cases: list[dict[str, Any]] = []
    for case in failure_manifest["cases"]:
        text = (root / case["file"]).read_text(encoding="utf-8").strip()
        failure_cases.append(
            {
                "schema_version": "0.1",
                "id": f"robustness_{case['id']}",
                "source_dataset": "mealcheck_input_robustness",
                "source_ref": {
                    "file": str(Path("examples/meal-plan-input-robustness") / case["file"]),
                    "case_id": case["id"],
                },
                "input_text": text,
                "expected_failure": {
                    "stage": "qualification",
                    "status": case["expected_status"],
                },
                "tags": ["failure", "reviewed_seed", case["expected_status"]],
            }
        )
    return success_cases, failure_cases


def load_nyt_cases(path: Path, limit: int) -> list[dict[str, Any]]:
    rows: list[SourceItem] = []
    with path.open(encoding="utf-8", newline="") as handle:
        for row in csv.DictReader(handle):
            unit = normalize_unit(row.get("unit", ""))
            if unit not in set(SUPPORTED_UNITS.values()):
                continue
            name = " ".join((row.get("name") or "").split())
            if not name:
                continue
            quantity = parse_quantity(row.get("qty", ""))
            if quantity <= 0:
                continue
            source_text = f"{format_quantity(quantity)} {unit} {name}"
            item = source_item_from_text(1, "b", 1, source_text)
            if item is None:
                continue
            rows.append(item)
            if limit > 0 and len(rows) >= limit * 9:
                break
    return generated_meal_cases("nyt", "nyt_ingredient_phrase_tagger", rows, limit)


def load_tasteset_cases(path: Path, limit: int) -> list[dict[str, Any]]:
    rows: list[SourceItem] = []
    with path.open(encoding="utf-8", newline="") as handle:
        for recipe_index, row in enumerate(csv.DictReader(handle), start=1):
            block = row["ingredients"]
            entities = json.loads(row["ingredients_entities"])
            pos = 0
            for line in block.splitlines(True):
                end = pos + len(line)
                line_entities = [e for e in entities if e["start"] >= pos and e["end"] <= end]
                item = source_item_from_tasteset_entities(line_entities)
                if item is not None:
                    rows.append(item)
                pos = end
                if limit > 0 and len(rows) >= limit * 9:
                    break
            if limit > 0 and len(rows) >= limit * 9:
                break
    return generated_meal_cases("tasteset", "tasteset", rows, limit)


def source_item_from_tasteset_entities(entities: list[dict[str, Any]]) -> SourceItem | None:
    quantity_entities = [e["entity"] for e in entities if e["type"] == "QUANTITY"]
    unit_entities = [e["entity"] for e in entities if e["type"] == "UNIT"]
    food_entities = [e["entity"] for e in entities if e["type"] == "FOOD"]
    if not quantity_entities or not unit_entities or not food_entities:
        return None
    quantity = parse_quantity(quantity_entities[0])
    unit = normalize_unit(unit_entities[0])
    if quantity <= 0 or unit not in set(SUPPORTED_UNITS.values()):
        return None
    food = " ".join(" ".join(food_entities).split())
    if not food:
        return None
    source_text = f"{format_quantity(quantity)} {unit} {food}"
    return source_item_from_text(1, "b", 1, source_text)


def generated_meal_cases(prefix: str, source_dataset: str, rows: list[SourceItem], limit: int) -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []
    case_limit = limit if limit > 0 else 0
    for case_index, offset in enumerate(range(0, len(rows) - 8, 9), start=1):
        if case_limit and len(cases) >= case_limit:
            break
        group = rows[offset : offset + 9]
        text, expected = meal_plan_from_items(group)
        cases.append(
            {
                "schema_version": "0.1",
                "id": f"{prefix}_generated_{case_index:06d}",
                "source_dataset": source_dataset,
                "source_ref": {"row_start": offset + 1},
                "input_text": text,
                "expected": {
                    "days": [1],
                    "source_items": [source_item_json(item) for item in expected],
                },
                "tags": ["success", "generated", source_dataset, "one_day", "three_meals"],
            }
        )
    return cases


def meal_plan_from_items(items: list[SourceItem]) -> tuple[str, list[SourceItem]]:
    meals = [("breakfast", "b"), ("lunch", "l"), ("dinner", "d")]
    lines: list[str] = []
    expected: list[SourceItem] = []
    next_id = 1
    for meal_index, (meal_name, meal_code) in enumerate(meals):
        group = items[meal_index * 3 : meal_index * 3 + 3]
        source_texts = [item.source_text for item in group]
        line = f"Day 1 {meal_name}: {source_texts[0]}, {source_texts[1]}, and {source_texts[2]}."
        lines.append(line)
        for source_text in source_texts:
            item = source_item_from_text(1, meal_code, next_id, source_text)
            if item is None:
                raise ValueError(f"generated source text did not parse: {source_text}")
            expected.append(item)
            next_id += 1
    return "\n".join(lines), expected


def resolved_source_items(text: str) -> list[SourceItem]:
    items: list[SourceItem] = []
    current_day = 1
    current_meal_code = ""
    for line in text.splitlines():
        trimmed = line.strip()
        if not trimmed:
            continue
        is_item_line = bool(RESOLVED_ITEM_LINE_PATTERN.search(line))
        if not is_item_line:
            day = day_from_heading(trimmed)
            if day > 0:
                current_day = day
            meal_code = meal_code_from_heading(trimmed)
            if meal_code:
                current_meal_code = meal_code
            for source_text in inline_source_texts(trimmed):
                item = source_item_from_text(current_day, current_meal_code, len(items) + 1, source_text)
                if item is not None:
                    items.append(item)
            continue
        source_text = SOURCE_ITEM_MARKER_PATTERN.sub("", line).strip()
        item = source_item_from_text(current_day, current_meal_code, len(items) + 1, source_text)
        if item is not None:
            items.append(item)
    return items


def inline_source_texts(line: str) -> list[str]:
    if ":" not in line:
        return []
    rest = line.split(":", 1)[1].strip()
    if not rest:
        return []
    rest = rest.replace(";", ",")
    phrases: list[str] = []
    for part in rest.split(","):
        phrases.extend(split_inline_and_quantified(part))
    source_texts: list[str] = []
    for phrase in phrases:
        phrase = re.sub(r"^\s*and\s+", "", phrase.strip().strip("."), flags=re.IGNORECASE)
        if not phrase:
            continue
        match = INLINE_ITEM_PATTERN.match(phrase)
        if not match:
            continue
        quantity = " ".join(match.group(1).split())
        unit = (match.group(2) or "").strip()
        food = match.group(4).strip()
        if unit:
            food = re.sub(r"^of\s+", "", food, flags=re.IGNORECASE).strip()
        else:
            unit = "serving"
        unit = normalize_unit(unit)
        source_texts.append(f"{quantity} {unit} {food}".strip())
    return source_texts


def split_inline_and_quantified(part: str) -> list[str]:
    remaining = part.strip()
    if not remaining:
        return []
    phrases: list[str] = []
    while True:
        match = INLINE_BOUNDARY_PATTERN.search(remaining)
        if not match:
            phrases.append(remaining)
            return phrases
        left = remaining[: match.start()].strip()
        if left:
            phrases.append(left)
        remaining = remaining[match.start(1) :].strip()
        if not remaining:
            return phrases


def source_item_from_text(day: int, meal_code: str, source_item_id: int, source_text: str) -> SourceItem | None:
    match = PARSED_SOURCE_PATTERN.match(source_text.strip())
    if not match:
        return None
    quantity = parse_quantity(match.group(1))
    unit = normalize_unit(match.group(2))
    food = match.group(3).strip()
    if quantity <= 0 or not unit or not food:
        return None
    return SourceItem(
        source_item_id=source_item_id,
        day=day,
        meal_code=meal_code,
        source_text=source_text.strip(),
        food=food,
        quantity=quantity,
        unit=unit,
    )


def source_item_json(item: SourceItem) -> dict[str, Any]:
    return {
        "source_item_id": item.source_item_id,
        "day": item.day,
        "meal_code": item.meal_code,
        "source_text": item.source_text,
        "food": item.food,
        "quantity": item.quantity,
        "unit": item.unit,
    }


def day_from_heading(line: str) -> int:
    match = DAY_PATTERN.search(line)
    if not match:
        return 0
    return int(match.group(1))


def meal_code_from_heading(line: str) -> str:
    heading = line.lower().strip().rstrip(":")
    for label, code in MEAL_CODES.items():
        if label in heading:
            return code
    return ""


def normalize_unit(unit: str) -> str:
    return SUPPORTED_UNITS.get(unit.lower().strip(), unit.lower().strip())


def parse_quantity(value: str | float | int | None) -> float:
    if value is None:
        return 0.0
    text = str(value).strip()
    if not text:
        return 0.0
    text = (
        text.replace("\u00bd", "1/2")
        .replace("\u2153", "1/3")
        .replace("\u2154", "2/3")
        .replace("\u00bc", "1/4")
        .replace("\u00be", "3/4")
        .replace("\u2044", "/")
    )
    try:
        return float(text)
    except ValueError:
        pass
    parts = text.split()
    if len(parts) == 2 and "/" in parts[1]:
        return parse_quantity(parts[0]) + parse_quantity(parts[1])
    if "/" in text:
        numerator, denominator = text.split("/", 1)
        try:
            denominator_value = float(denominator)
            if denominator_value == 0:
                return 0.0
            return float(numerator) / denominator_value
        except ValueError:
            return 0.0
    return 0.0


def format_quantity(value: float) -> str:
    if value == int(value):
        return str(int(value))
    return f"{value:.4f}".rstrip("0").rstrip(".")


def write_artifacts(out_dir: Path, success_cases: list[dict[str, Any]], failure_cases: list[dict[str, Any]]) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    write_json(out_dir / "source-manifest.json", source_manifest())
    write_json(
        out_dir / "manifest.json",
        {
            "schema_version": "0.1",
            "dataset_id": DATASET_ID,
            "description": "P0 meal-plan normalization evaluation cases for deterministic source inventory, compact-row adapter checks, and qualification failures.",
            "case_files": ["cases-v1.jsonl"],
            "failure_case_files": ["failure-cases-v1.jsonl"],
            "supported_units": ["g", "oz", "cup", "tbsp", "tsp", "slice", "serving"],
            "release_gate": False,
            "source_refs": source_manifest()["sources"],
            "summary": {
                "success_cases": len(success_cases),
                "failure_cases": len(failure_cases),
                "total_expected_source_items": sum(len(case["expected"]["source_items"]) for case in success_cases),
            },
        },
    )
    write_jsonl(out_dir / "cases-v1.jsonl", success_cases)
    write_jsonl(out_dir / "failure-cases-v1.jsonl", failure_cases)


def source_manifest() -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "source_manifest_id": "p0-normalization-sources-v1",
        "generation_command": "python3 scripts/generate-p0-normalization-evaluation.py",
        "sources": [
            {
                "source": "mealcheck_input_robustness",
                "data_type": "checked-in MealCheck robustness examples",
                "path": "examples/meal-plan-input-robustness",
                "checked_in": True,
            },
            {
                "source": "nyt_ingredient_phrase_tagger",
                "data_type": "public ingredient phrase CSV",
                "url": "https://github.com/nytimes/ingredient-phrase-tagger",
                "expected_env": "MEALCHECK_NYT_INGREDIENTS_CSV",
                "checked_in": False,
            },
            {
                "source": "tasteset",
                "data_type": "public recipe NER CSV",
                "url": "https://github.com/taisti/TASTEset",
                "expected_env": "MEALCHECK_TASTESET_CSV",
                "checked_in": False,
            },
        ],
    }


def write_json(path: Path, doc: dict[str, Any]) -> None:
    path.write_text(json.dumps(doc, indent=2, sort_keys=False) + "\n", encoding="utf-8")


def write_jsonl(path: Path, rows: Iterable[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, separators=(",", ":"), sort_keys=False))
            handle.write("\n")


if __name__ == "__main__":
    main()
