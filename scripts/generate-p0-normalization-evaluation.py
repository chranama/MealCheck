#!/usr/bin/env python3
"""Generate MealCheck P0 meal-plan normalization evaluation cases.

The checked-in seed comes from examples/meal-plan-input-robustness. Optional
external source datasets can be provided locally through environment variables
or flags. Raw third-party source data is not checked in.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import re
from collections import Counter
from dataclasses import dataclass, field
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
EXTERNAL_WRAPPER_STYLES = [
    "one_day_inline",
    "one_day_bullets",
    "numbered_items",
    "two_day_sections",
    "snack_inclusive",
]

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
RANGE_QUANTITY_PATTERN = re.compile(r"(?i)\b\d+(?:\.\d+)?\s*(?:-|to|or)\s*\d+")
VAGUE_QUANTITY_PATTERN = re.compile(
    r"(?i)\b(handful|pinch|dash|sprinkle|small|medium|large|to taste|as needed|some|several)\b"
)
OPTIONAL_PATTERN = re.compile(r"(?i)\b(optional|or|substitute|alternatively|if desired)\b")


@dataclass
class SourceItem:
    source_item_id: int
    day: int
    meal_code: str
    source_text: str
    food: str
    quantity: float
    unit: str
    source_ref: dict[str, Any] | None = None


@dataclass
class CandidateRecord:
    source_dataset: str
    source_ref: dict[str, Any]
    raw_text: str
    quantity_text: str = ""
    quantity: float = 0.0
    unit_text: str = ""
    unit: str = ""
    food: str = ""
    prep_or_quality: str = ""
    status: str = "schema_error"
    reason: str = ""


@dataclass
class ExternalOutput:
    source_dataset: str
    prefix: str
    source_info: dict[str, Any]
    success_cases: list[dict[str, Any]] = field(default_factory=list)
    failure_cases: list[dict[str, Any]] = field(default_factory=list)
    quarantine_rows: list[dict[str, Any]] = field(default_factory=list)
    status_counts: Counter[str] = field(default_factory=Counter)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out-dir", default=str(OUT_DIR))
    parser.add_argument("--robustness-dir", default=str(ROBUSTNESS_DIR))
    parser.add_argument("--nyt-csv", default=os.environ.get("MEALCHECK_NYT_INGREDIENTS_CSV", ""))
    parser.add_argument("--tasteset-csv", default=os.environ.get("MEALCHECK_TASTESET_CSV", ""))
    parser.add_argument("--nyt-limit", type=int, default=0)
    parser.add_argument("--tasteset-limit", type=int, default=0)
    parser.add_argument("--probe-sources", action="store_true")
    args = parser.parse_args()

    if args.probe_sources:
        print(json.dumps(probe_sources(args), indent=2, sort_keys=False))
        return

    out_dir = Path(args.out_dir)
    robust_success, robust_failures = load_robustness_cases(Path(args.robustness_dir))

    external_outputs: list[ExternalOutput] = []
    if args.nyt_csv:
        external_outputs.append(load_nyt_output(Path(args.nyt_csv), args.nyt_limit))
    if args.tasteset_csv:
        external_outputs.append(load_tasteset_output(Path(args.tasteset_csv), args.tasteset_limit))

    write_artifacts(out_dir, robust_success, robust_failures, external_outputs)
    total_success = len(robust_success) + sum(len(output.success_cases) for output in external_outputs)
    total_failures = len(robust_failures) + sum(len(output.failure_cases) for output in external_outputs)
    print(f"wrote {total_success} success and {total_failures} failure cases to {out_dir}")


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


def load_nyt_output(path: Path, limit: int) -> ExternalOutput:
    source_dataset = "nyt_ingredient_phrase_tagger"
    records: list[CandidateRecord] = []
    with path.open(encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        for row_number, row in enumerate(reader, start=2):
            records.append(nyt_candidate(row, row_number))

    return external_output_from_records(
        source_dataset=source_dataset,
        prefix="nyt",
        source_info=source_info(
            source=source_dataset,
            data_type="public ingredient phrase CSV",
            path=path,
            url="https://github.com/nytimes/ingredient-phrase-tagger",
            expected_env="MEALCHECK_NYT_INGREDIENTS_CSV",
        ),
        records=records,
        limit=limit,
    )


def nyt_candidate(row: dict[str, str], row_number: int) -> CandidateRecord:
    quantity_text = clean_text(row.get("qty", ""))
    unit_text = clean_text(row.get("unit", ""))
    food = clean_text(row.get("name", ""))
    comment = clean_text(row.get("comment", ""))
    raw_text = clean_text(row.get("input", "")) or " ".join(
        value for value in [quantity_text, unit_text, food, comment] if value
    )
    quantity = parse_quantity(quantity_text)
    unit = normalize_unit(unit_text)
    status = classify_candidate(quantity_text, quantity, unit_text, unit, food, raw_text, comment)
    return CandidateRecord(
        source_dataset="nyt_ingredient_phrase_tagger",
        source_ref={"row_number": row_number},
        raw_text=raw_text,
        quantity_text=quantity_text,
        quantity=quantity,
        unit_text=unit_text,
        unit=unit,
        food=food,
        prep_or_quality=comment,
        status=status,
        reason=status,
    )


def load_tasteset_output(path: Path, limit: int) -> ExternalOutput:
    source_dataset = "tasteset"
    records: list[CandidateRecord] = []
    with path.open(encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        for row_number, row in enumerate(reader, start=2):
            records.extend(tasteset_candidates(row, row_number))

    return external_output_from_records(
        source_dataset=source_dataset,
        prefix="tasteset",
        source_info=source_info(
            source=source_dataset,
            data_type="public recipe NER CSV",
            path=path,
            url="https://github.com/taisti/TASTEset",
            expected_env="MEALCHECK_TASTESET_CSV",
        ),
        records=records,
        limit=limit,
    )


def tasteset_candidates(row: dict[str, str], row_number: int) -> list[CandidateRecord]:
    ingredients = row.get("ingredients", "")
    entities_text = row.get("ingredients_entities", "")
    if not ingredients or not entities_text:
        raw = clean_text(ingredients or json.dumps(row, sort_keys=True))
        return [
            CandidateRecord(
                source_dataset="tasteset",
                source_ref={"row_number": row_number},
                raw_text=raw,
                status="schema_error",
                reason="missing ingredients or ingredients_entities",
            )
        ]
    try:
        entities = json.loads(entities_text)
    except json.JSONDecodeError:
        return [
            CandidateRecord(
                source_dataset="tasteset",
                source_ref={"row_number": row_number},
                raw_text=clean_text(ingredients),
                status="schema_error",
                reason="ingredients_entities is not JSON",
            )
        ]

    records: list[CandidateRecord] = []
    offset = 0
    for line_index, line in enumerate(ingredients.splitlines(True), start=1):
        end = offset + len(line)
        line_entities = [entity for entity in entities if entity.get("start", -1) >= offset and entity.get("end", -1) <= end]
        records.append(tasteset_candidate_from_line(line, line_entities, row_number, line_index))
        offset = end
    return records


def tasteset_candidate_from_line(line: str, entities: list[dict[str, Any]], row_number: int, line_index: int) -> CandidateRecord:
    typed = [(clean_text(entity.get("type", "")).upper(), clean_text(entity.get("entity", ""))) for entity in entities]
    quantity_text = first_entity(typed, "QUANTITY")
    unit_text = first_entity(typed, "UNIT")
    food_parts = [value for label, value in typed if label == "FOOD"]
    modifiers = [value for label, value in typed if label in {"PROCESS", "PHYSICAL_QUALITY"}]
    food = " ".join([*modifiers, *food_parts]).strip()
    quantity = parse_quantity(quantity_text)
    unit = normalize_unit(unit_text)
    raw_text = clean_text(line)
    status = classify_candidate(quantity_text, quantity, unit_text, unit, food, raw_text, "")
    return CandidateRecord(
        source_dataset="tasteset",
        source_ref={"row_number": row_number, "line_number": line_index},
        raw_text=raw_text,
        quantity_text=quantity_text,
        quantity=quantity,
        unit_text=unit_text,
        unit=unit,
        food=food,
        prep_or_quality=" ".join(modifiers),
        status=status,
        reason=status,
    )


def first_entity(entities: list[tuple[str, str]], label: str) -> str:
    for entity_label, value in entities:
        if entity_label == label and value:
            return value
    return ""


def external_output_from_records(source_dataset: str, prefix: str, source_info: dict[str, Any], records: list[CandidateRecord], limit: int) -> ExternalOutput:
    status_counts = Counter(record.status for record in records)
    success_candidates = [record for record in records if record.status == "success_candidate"]
    failure_candidates = [
        record
        for record in records
        if record.status in {"unsupported_unit", "missing_quantity", "vague_quantity", "range_quantity"}
    ]
    quarantine_candidates = [
        record
        for record in records
        if record.status not in {"success_candidate", "unsupported_unit", "missing_quantity", "vague_quantity", "range_quantity"}
    ]
    success_cases = generated_meal_cases(prefix, source_dataset, success_candidates, limit)
    failure_cases = generated_failure_cases(prefix, source_dataset, failure_candidates, limit)
    quarantine_rows = quarantine_records(prefix, source_dataset, quarantine_candidates)
    source_info = dict(source_info)
    source_info["status_counts"] = dict(sorted(status_counts.items()))
    source_info["generated_success_cases"] = len(success_cases)
    source_info["generated_failure_cases"] = len(failure_cases)
    source_info["quarantine_cases"] = len(quarantine_rows)
    return ExternalOutput(
        source_dataset=source_dataset,
        prefix=prefix,
        source_info=source_info,
        success_cases=success_cases,
        failure_cases=failure_cases,
        quarantine_rows=quarantine_rows,
        status_counts=status_counts,
    )


def classify_candidate(quantity_text: str, quantity: float, unit_text: str, unit: str, food: str, raw_text: str, comment: str) -> str:
    combined = " ".join([quantity_text, unit_text, food, raw_text, comment]).strip()
    if OPTIONAL_PATTERN.search(combined):
        return "optional_or_alternative"
    if RANGE_QUANTITY_PATTERN.search(quantity_text) or RANGE_QUANTITY_PATTERN.search(raw_text):
        return "range_quantity"
    if not quantity_text:
        return "missing_quantity"
    if VAGUE_QUANTITY_PATTERN.search(combined):
        return "vague_quantity"
    if quantity <= 0:
        return "vague_quantity"
    if not unit_text:
        return "unsupported_unit"
    if unit not in set(SUPPORTED_UNITS.values()):
        return "unsupported_unit"
    if not food:
        return "missing_food"
    return "success_candidate"


def generated_meal_cases(prefix: str, source_dataset: str, records: list[CandidateRecord], limit: int) -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []
    index = 0
    case_index = 1
    while index < len(records):
        style = EXTERNAL_WRAPPER_STYLES[(case_index - 1) % len(EXTERNAL_WRAPPER_STYLES)]
        needed = wrapper_item_count(style)
        if index + needed > len(records):
            break
        if limit > 0 and len(cases) >= limit:
            break
        group = records[index : index + needed]
        text, expected = meal_plan_from_records(group, style)
        cases.append(
            {
                "schema_version": "0.1",
                "id": f"{prefix}_generated_{case_index:06d}",
                "source_dataset": source_dataset,
                "source_ref": {"row_start": group[0].source_ref},
                "input_text": text,
                "expected": {
                    "days": sorted({item.day for item in expected}),
                    "source_items": [source_item_json(item) for item in expected],
                },
                "tags": [
                    "success",
                    "generated",
                    source_dataset,
                    style,
                    *quantity_style_tags(group),
                    *unit_tags(group),
                ],
            }
        )
        index += needed
        case_index += 1
    return cases


def generated_failure_cases(prefix: str, source_dataset: str, records: list[CandidateRecord], limit: int) -> list[dict[str, Any]]:
    case_limit = limit if limit > 0 else len(records)
    cases: list[dict[str, Any]] = []
    for index, record in enumerate(records[:case_limit], start=1):
        text = f"Day 1 breakfast:\n- {failure_source_text(record)}"
        cases.append(
            {
                "schema_version": "0.1",
                "id": f"{prefix}_failure_{index:06d}",
                "source_dataset": source_dataset,
                "source_ref": record.source_ref,
                "input_text": text,
                "expected_failure": {
                    "stage": "source_inventory",
                    "reason": record.status,
                },
                "tags": ["failure", "generated", source_dataset, record.status],
            }
        )
    return cases


def quarantine_records(prefix: str, source_dataset: str, records: list[CandidateRecord]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for index, record in enumerate(records, start=1):
        rows.append(
            {
                "schema_version": "0.1",
                "id": f"{prefix}_quarantine_{index:06d}",
                "source_dataset": source_dataset,
                "source_ref": record.source_ref,
                "raw_text": record.raw_text,
                "quarantine_reason": record.status,
                "detail": record.reason,
            }
        )
    return rows


def wrapper_item_count(style: str) -> int:
    if style == "two_day_sections":
        return 18
    if style == "snack_inclusive":
        return 12
    return 9


def meal_plan_from_records(records: list[CandidateRecord], style: str) -> tuple[str, list[SourceItem]]:
    if style == "two_day_sections":
        return two_day_plan_from_records(records)
    if style == "snack_inclusive":
        return snack_plan_from_records(records)
    return one_day_plan_from_records(records, style)


def one_day_plan_from_records(records: list[CandidateRecord], style: str) -> tuple[str, list[SourceItem]]:
    meals = [("breakfast", "b"), ("lunch", "l"), ("dinner", "d")]
    lines: list[str] = []
    expected: list[SourceItem] = []
    next_id = 1
    for meal_index, (meal_name, meal_code) in enumerate(meals):
        group = records[meal_index * 3 : meal_index * 3 + 3]
        source_texts = [source_text_from_record(record) for record in group]
        if style == "one_day_bullets":
            lines.append(f"Day 1 {meal_name}:")
            lines.extend(f"- {source_text}" for source_text in source_texts)
        elif style == "numbered_items":
            lines.append(f"Day 1 {meal_name}:")
            lines.extend(f"{i}. {source_text}" for i, source_text in enumerate(source_texts, start=1))
        else:
            lines.append(f"Day 1 {meal_name}: {source_texts[0]}, {source_texts[1]}, and {source_texts[2]}.")
        for record, source_text in zip(group, source_texts):
            expected.append(source_item_from_record(1, meal_code, next_id, source_text, record))
            next_id += 1
    return "\n".join(lines), expected


def two_day_plan_from_records(records: list[CandidateRecord]) -> tuple[str, list[SourceItem]]:
    lines: list[str] = []
    expected: list[SourceItem] = []
    next_id = 1
    meal_specs = [("breakfast", "b"), ("lunch", "l"), ("dinner", "d")]
    for day in [1, 2]:
        lines.append(f"Day {day}")
        day_records = records[(day - 1) * 9 : day * 9]
        for meal_index, (meal_name, meal_code) in enumerate(meal_specs):
            group = day_records[meal_index * 3 : meal_index * 3 + 3]
            lines.append(f"{meal_name.title()}:")
            for record in group:
                source_text = source_text_from_record(record)
                lines.append(f"- {source_text}")
                expected.append(source_item_from_record(day, meal_code, next_id, source_text, record))
                next_id += 1
    return "\n".join(lines), expected


def snack_plan_from_records(records: list[CandidateRecord]) -> tuple[str, list[SourceItem]]:
    meal_specs = [
        ("breakfast", "b"),
        ("morning snack", "m"),
        ("lunch", "l"),
        ("afternoon snack", "a"),
        ("dinner", "d"),
        ("evening snack", "e"),
    ]
    lines: list[str] = []
    expected: list[SourceItem] = []
    next_id = 1
    for meal_index, (meal_name, meal_code) in enumerate(meal_specs):
        group = records[meal_index * 2 : meal_index * 2 + 2]
        source_texts = [source_text_from_record(record) for record in group]
        lines.append(f"Day 1 {meal_name}: {source_texts[0]} and {source_texts[1]}.")
        for record, source_text in zip(group, source_texts):
            expected.append(source_item_from_record(1, meal_code, next_id, source_text, record))
            next_id += 1
    return "\n".join(lines), expected


def source_item_from_record(day: int, meal_code: str, source_item_id: int, source_text: str, record: CandidateRecord) -> SourceItem:
    return SourceItem(
        source_item_id=source_item_id,
        day=day,
        meal_code=meal_code,
        source_text=source_text,
        food=record.food,
        quantity=record.quantity,
        unit=record.unit,
        source_ref=record.source_ref,
    )


def source_text_from_record(record: CandidateRecord) -> str:
    quantity = clean_text(record.quantity_text) or format_quantity(record.quantity)
    unit = record.unit
    return f"{quantity} {unit} {record.food}".strip()


def failure_source_text(record: CandidateRecord) -> str:
    if record.raw_text:
        return record.raw_text
    return " ".join(value for value in [record.quantity_text, record.unit_text, record.food] if value)


def quantity_style_tags(records: list[CandidateRecord]) -> list[str]:
    tags: set[str] = set()
    for record in records:
        text = record.quantity_text
        if "/" in text:
            tags.add("fraction")
        elif record.quantity != int(record.quantity):
            tags.add("decimal")
        else:
            tags.add("integer_quantity")
    return sorted(tags)


def unit_tags(records: list[CandidateRecord]) -> list[str]:
    return sorted({f"unit_{record.unit}" for record in records if record.unit})


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
    row = {
        "source_item_id": item.source_item_id,
        "day": item.day,
        "meal_code": item.meal_code,
        "source_text": item.source_text,
        "food": item.food,
        "quantity": item.quantity,
        "unit": item.unit,
    }
    if item.source_ref:
        row["source_ref"] = item.source_ref
    return row


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
    if RANGE_QUANTITY_PATTERN.search(text):
        return 0.0
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


def clean_text(value: Any) -> str:
    return " ".join(str(value or "").strip().split())


def write_artifacts(
    out_dir: Path,
    robust_success: list[dict[str, Any]],
    robust_failures: list[dict[str, Any]],
    external_outputs: list[ExternalOutput],
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    write_json(out_dir / "source-manifest.json", source_manifest(external_outputs))
    write_jsonl(out_dir / "cases-v1.jsonl", robust_success)
    write_jsonl(out_dir / "failure-cases-v1.jsonl", robust_failures)

    case_files = [
        {
            "path": "cases-v1.jsonl",
            "source_dataset": "mealcheck_input_robustness",
            "gate": "strict",
        }
    ]
    failure_case_files = [
        {
            "path": "failure-cases-v1.jsonl",
            "source_dataset": "mealcheck_input_robustness",
            "gate": "strict",
        }
    ]
    quarantine_files: list[dict[str, str]] = []

    for output in external_outputs:
        case_path = f"{output.prefix}-cases-v1.jsonl"
        failure_path = f"{output.prefix}-failure-cases-v1.jsonl"
        quarantine_path = f"{output.prefix}-quarantine-v1.jsonl"
        write_jsonl(out_dir / case_path, output.success_cases)
        write_jsonl(out_dir / failure_path, output.failure_cases)
        write_jsonl(out_dir / quarantine_path, output.quarantine_rows)
        case_files.append(
            {
                "path": case_path,
                "source_dataset": output.source_dataset,
                "gate": "exploratory",
            }
        )
        failure_case_files.append(
            {
                "path": failure_path,
                "source_dataset": output.source_dataset,
                "gate": "exploratory",
            }
        )
        quarantine_files.append(
            {
                "path": quarantine_path,
                "source_dataset": output.source_dataset,
                "gate": "exploratory",
            }
        )

    success_cases = len(robust_success) + sum(len(output.success_cases) for output in external_outputs)
    failure_cases = len(robust_failures) + sum(len(output.failure_cases) for output in external_outputs)
    quarantine_cases = sum(len(output.quarantine_rows) for output in external_outputs)
    total_expected_items = sum(len(case["expected"]["source_items"]) for case in robust_success)
    total_expected_items += sum(
        len(case["expected"]["source_items"]) for output in external_outputs for case in output.success_cases
    )
    manifest = {
        "schema_version": "0.1",
        "dataset_id": DATASET_ID,
        "description": "P0 meal-plan normalization evaluation cases for deterministic source inventory, compact-row adapter checks, qualification failures, and optional external exploratory cases.",
        "case_files": case_files,
        "failure_case_files": failure_case_files,
        "supported_units": ["g", "oz", "cup", "tbsp", "tsp", "slice", "serving"],
        "release_gate": False,
        "source_refs": source_manifest(external_outputs)["sources"],
        "summary": {
            "success_cases": success_cases,
            "failure_cases": failure_cases,
            "quarantine_cases": quarantine_cases,
            "total_expected_source_items": total_expected_items,
        },
    }
    if quarantine_files:
        manifest["quarantine_files"] = quarantine_files
    write_json(out_dir / "manifest.json", manifest)


def source_manifest(external_outputs: list[ExternalOutput]) -> dict[str, Any]:
    sources = [
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
            "license_note": "Review upstream license and generated sample size before committing generated cases.",
        },
        {
            "source": "tasteset",
            "data_type": "public recipe NER CSV",
            "url": "https://github.com/taisti/TASTEset",
            "expected_env": "MEALCHECK_TASTESET_CSV",
            "checked_in": False,
            "license_note": "Review upstream license and generated sample size before committing generated cases.",
        },
    ]
    by_source = {source["source"]: source for source in sources}
    for output in external_outputs:
        by_source[output.source_dataset].update(output.source_info)
    return {
        "schema_version": "0.1",
        "source_manifest_id": "p0-normalization-sources-v1",
        "generation_command": "python3 scripts/generate-p0-normalization-evaluation.py",
        "sources": sources,
    }


def probe_sources(args: argparse.Namespace) -> dict[str, Any]:
    probes: list[dict[str, Any]] = []
    if args.nyt_csv:
        probes.append(probe_csv_source(Path(args.nyt_csv), "nyt_ingredient_phrase_tagger", ["qty", "unit", "name"]))
    if args.tasteset_csv:
        probes.append(probe_csv_source(Path(args.tasteset_csv), "tasteset", ["ingredients", "ingredients_entities"]))
    return {"schema_version": "0.1", "sources": probes}


def probe_csv_source(path: Path, source: str, required_columns: list[str]) -> dict[str, Any]:
    result: dict[str, Any] = {
        "source": source,
        "path": str(path),
        "exists": path.exists(),
        "required_columns": required_columns,
    }
    if not path.exists():
        result["status"] = "missing"
        return result
    result["sha256"] = sha256_file(path)
    with path.open(encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        columns = reader.fieldnames or []
        result["columns"] = columns
        result["missing_columns"] = [column for column in required_columns if column not in columns]
        row_count = 0
        for row_count, _ in enumerate(reader, start=1):
            pass
        result["row_count"] = row_count
    result["status"] = "ok" if not result["missing_columns"] else "schema_error"
    return result


def source_info(source: str, data_type: str, path: Path, url: str, expected_env: str) -> dict[str, Any]:
    return {
        "source": source,
        "data_type": data_type,
        "url": url,
        "expected_env": expected_env,
        "local_path": str(path),
        "sha256": sha256_file(path),
        "checked_in": False,
    }


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_json(path: Path, doc: dict[str, Any]) -> None:
    path.write_text(json.dumps(doc, indent=2, sort_keys=False) + "\n", encoding="utf-8")


def write_jsonl(path: Path, rows: Iterable[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, separators=(",", ":"), sort_keys=False))
            handle.write("\n")


if __name__ == "__main__":
    main()
