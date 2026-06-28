#!/usr/bin/env python3
"""Generate MealCheck's WWEIA/NHANES real-recall evaluation layer.

The script reads public NHANES August 2021-August 2023 dietary interview XPT
files and USDA FNDDS 2021-2023 food descriptions, then writes a compact,
deterministic evaluation dataset:

- data/evaluation/wweia-nhanes-real-recalls-v1.json

Raw NHANES and FNDDS files are intentionally not checked in. Regeneration
expects the source files documented in docs/evaluation.md.
"""

from __future__ import annotations

import argparse
import json
import math
import re
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable
from xml.etree import ElementTree
from zipfile import ZipFile


DATASET_ID = "wweia-nhanes-real-recalls-v1"
CATALOG_PATH = Path("data/nutrients/fixture-catalog-v1.json")
DATASET_PATH = Path("data/evaluation/wweia-nhanes-real-recalls-v1.json")
SOURCE_ID = "wweia-nhanes-2021-2023"
FNDDS_SOURCE_ID = "fndds-2021-2023"

OCCASION_LABELS = {
    1: "Breakfast",
    2: "Lunch",
    3: "Dinner",
    4: "Supper",
    5: "Brunch",
    6: "Snack",
    7: "Drink",
    8: "Infant feeding",
    9: "Extended consumption",
    10: "Breakfast",
    11: "Lunch",
    12: "Dinner",
    13: "Snack",
    14: "Dinner",
    15: "Snack",
    16: "Snack",
    17: "Snack",
    18: "Snack",
    19: "Drink",
    91: "Other",
    99: "Unknown",
}


@dataclass(frozen=True)
class XPTVariable:
    name: str
    var_type: int
    length: int
    position: int


@dataclass
class RecallItem:
    seqn: int
    day: int
    line: int
    occasion_code: int
    occasion_label: str
    time_seconds: int
    food_code: str
    food_description: str
    grams: float
    energy_kcal: float
    protein_g: float
    sodium_mg: float
    saturated_fat_g: float
    known_local: bool


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--nhanes-source-dir",
        default="/tmp/mealcheck-nhanes-2021-2023",
        help="Directory containing DR1IFF_L.xpt, DR2IFF_L.xpt, and DEMO_L.xpt.",
    )
    parser.add_argument(
        "--fndds-source-dir",
        default="/tmp/mealcheck-fndds-2021-2023",
        help="Directory containing the FNDDS 2021-2023 At A Glance workbooks.",
    )
    parser.add_argument("--catalog", default=str(CATALOG_PATH))
    parser.add_argument("--out", default=str(DATASET_PATH))
    args = parser.parse_args()

    nhanes_dir = Path(args.nhanes_source_dir)
    fndds_dir = Path(args.fndds_source_dir)
    catalog_path = Path(args.catalog)

    fndds_descriptions = load_fndds_food_descriptions(fndds_dir / "foods-and-beverages.xlsx")
    local_codes = load_local_catalog_codes(catalog_path)
    adult_seqns = load_adult_seqns(nhanes_dir / "DEMO_L.xpt")
    recall_items = load_recall_items(nhanes_dir, fndds_descriptions, local_codes, adult_seqns)

    cases = select_cases(recall_items)
    dataset = {
        "schema_version": "0.1",
        "dataset_id": DATASET_ID,
        "description": (
            "One hundred MealCheck evaluation cases generated from public "
            "WWEIA/NHANES August 2021-August 2023 dietary interview records. "
            "Cases preserve real reported eating occasions and gram weights, "
            "while FNDDS food-code descriptions provide user-facing food text."
        ),
        "catalog_path": str(CATALOG_PATH),
        "source_refs": [
            {
                "source": SOURCE_ID,
                "data_type": "NHANES Dietary Interview - Individual Foods",
                "note": "DR1IFF_L and DR2IFF_L public XPT files, filtered to adult reliable recalls.",
            },
            {
                "source": "nhanes-demo-2021-2023",
                "data_type": "NHANES Demographics",
                "note": "DEMO_L public XPT file used for adult age filtering.",
            },
            {
                "source": FNDDS_SOURCE_ID,
                "data_type": "FNDDS At A Glance",
                "note": "Foods and Beverages workbook used to map food codes to descriptions.",
            },
        ],
        "cases": cases,
        "summary": dataset_summary(cases),
    }
    write_json(Path(args.out), dataset)
    print(f"wrote {args.out} with {len(cases)} cases")
    print(json.dumps(dataset["summary"], indent=2))


def load_local_catalog_codes(path: Path) -> set[str]:
    catalog = json.loads(path.read_text(encoding="utf-8"))
    result = set()
    for food in catalog["foods"]:
        for source in food.get("source_refs", []):
            if source.get("source") == FNDDS_SOURCE_ID and source.get("source_id"):
                result.add(str(source["source_id"]))
    return result


def load_adult_seqns(path: Path) -> set[int]:
    result = set()
    for row in read_xpt_rows(path, {"SEQN", "RIDAGEYR"}):
        age = row.get("RIDAGEYR")
        if age is not None and age >= 19:
            result.add(int(row["SEQN"]))
    return result


def load_recall_items(
    nhanes_dir: Path,
    fndds_descriptions: dict[str, str],
    local_codes: set[str],
    adult_seqns: set[int],
) -> list[RecallItem]:
    items: list[RecallItem] = []
    for day, filename in [(1, "DR1IFF_L.xpt"), (2, "DR2IFF_L.xpt")]:
        prefix = f"DR{day}"
        wanted = {
            "SEQN",
            f"{prefix}ILINE",
            f"{prefix}DRSTZ",
            f"{prefix}_020",
            f"{prefix}_030Z",
            f"{prefix}IFDCD",
            f"{prefix}IGRMS",
            f"{prefix}IKCAL",
            f"{prefix}IPROT",
            f"{prefix}ISODI",
            f"{prefix}ISFAT",
        }
        for row in read_xpt_rows(nhanes_dir / filename, wanted):
            seqn = int(row["SEQN"])
            if seqn not in adult_seqns:
                continue
            if row.get(f"{prefix}DRSTZ") != 1:
                continue
            grams = row.get(f"{prefix}IGRMS")
            food_code_value = row.get(f"{prefix}IFDCD")
            if grams is None or grams <= 0 or food_code_value is None:
                continue
            food_code = str(int(food_code_value))
            description = fndds_descriptions.get(food_code)
            if not description:
                continue
            occasion_code = int(row.get(f"{prefix}_030Z") or 99)
            items.append(
                RecallItem(
                    seqn=seqn,
                    day=day,
                    line=int(row.get(f"{prefix}ILINE") or 0),
                    occasion_code=occasion_code,
                    occasion_label=OCCASION_LABELS.get(occasion_code, "Other"),
                    time_seconds=int(row.get(f"{prefix}_020") or 0),
                    food_code=food_code,
                    food_description=description,
                    grams=float(grams),
                    energy_kcal=float(row.get(f"{prefix}IKCAL") or 0),
                    protein_g=float(row.get(f"{prefix}IPROT") or 0),
                    sodium_mg=float(row.get(f"{prefix}ISODI") or 0),
                    saturated_fat_g=float(row.get(f"{prefix}ISFAT") or 0),
                    known_local=food_code in local_codes,
                )
            )
    return items


def select_cases(items: list[RecallItem]) -> list[dict[str, Any]]:
    recall_groups: dict[tuple[int, int], list[RecallItem]] = defaultdict(list)
    occasion_groups: dict[tuple[int, int, int, int], list[RecallItem]] = defaultdict(list)
    for item in items:
        recall_groups[(item.seqn, item.day)].append(item)
        occasion_groups[(item.seqn, item.day, item.occasion_code, item.time_seconds)].append(item)

    selected: list[dict[str, Any]] = []
    used_recall_keys: set[tuple[int, int]] = set()
    used_occasion_keys: set[tuple[int, int, int, int]] = set()

    high_sodium = sorted(
        [
            (key, group)
            for key, group in recall_groups.items()
            if full_recall_candidate(group) and resolved_sodium(group) >= 2300
        ],
        key=lambda pair: (-resolved_sodium(pair[1]), -known_ratio(pair[1]), pair[0]),
    )
    add_full_recall_cases(selected, used_recall_keys, high_sodium, "wweia_high_sodium", 20)

    low_protein = sorted(
        [
            (key, group)
            for key, group in recall_groups.items()
            if full_recall_candidate(group) and known_ratio(group) >= 0.5 and resolved_protein(group) < 50
        ],
        key=lambda pair: (resolved_protein(pair[1]), -known_ratio(pair[1]), pair[0]),
    )
    add_full_recall_cases(selected, used_recall_keys, low_protein, "wweia_low_protein", 10)

    common_recall = sorted(
        [
            (key, group)
            for key, group in recall_groups.items()
            if full_recall_candidate(group) and known_ratio(group) >= 0.7
        ],
        key=lambda pair: (-known_ratio(pair[1]), len(pair[1]), pair[0]),
    )
    add_full_recall_cases(selected, used_recall_keys, common_recall, "wweia_common_recall_day", 30)

    resolved_occasions = sorted(
        [
            (key, group)
            for key, group in occasion_groups.items()
            if 2 <= len(group) <= 12 and all(item.known_local for item in group)
        ],
        key=lambda pair: (pair[0][0], pair[0][1], pair[0][3], pair[0][2]),
    )
    add_occasion_cases(
        selected,
        used_recall_keys,
        used_occasion_keys,
        resolved_occasions,
        "wweia_resolved_eating_occasion",
        40,
    )

    if len(selected) != 100:
        raise ValueError(f"expected 100 selected WWEIA/NHANES cases, got {len(selected)}")
    return renumber_cases(selected)


def full_recall_candidate(group: list[RecallItem]) -> bool:
    occasion_keys = {(item.occasion_code, item.time_seconds) for item in group}
    return 5 <= len(group) <= 30 and len(occasion_keys) >= 3


def add_full_recall_cases(
    selected: list[dict[str, Any]],
    used_recall_keys: set[tuple[int, int]],
    candidates: list[tuple[tuple[int, int], list[RecallItem]]],
    category: str,
    count: int,
) -> None:
    for key, group in candidates:
        if len([case for case in selected if case["category"] == category]) >= count:
            return
        if key in used_recall_keys:
            continue
        selected.append(case_from_group(category, group, full_day=True))
        used_recall_keys.add(key)
    raise ValueError(f"could not select {count} cases for {category}")


def add_occasion_cases(
    selected: list[dict[str, Any]],
    used_recall_keys: set[tuple[int, int]],
    used_occasion_keys: set[tuple[int, int, int, int]],
    candidates: list[tuple[tuple[int, int, int, int], list[RecallItem]]],
    category: str,
    count: int,
) -> None:
    for key, group in candidates:
        if len([case for case in selected if case["category"] == category]) >= count:
            return
        recall_key = (key[0], key[1])
        if key in used_occasion_keys or recall_key in used_recall_keys:
            continue
        selected.append(case_from_group(category, group, full_day=False))
        used_occasion_keys.add(key)
    raise ValueError(f"could not select {count} cases for {category}")


def case_from_group(category: str, group: list[RecallItem], *, full_day: bool) -> dict[str, Any]:
    group = sorted(group, key=lambda item: (item.time_seconds, item.occasion_code, item.line))
    seqn = group[0].seqn
    day = group[0].day
    meals = meals_from_items(group, full_day=full_day)
    unresolved_count = sum(1 for item in group if not item.known_local)
    expected: dict[str, Any] = {"unresolved_count": unresolved_count, "allow_extra_warnings": True}
    warn_checks: list[str] = []
    if category == "wweia_high_sodium" and resolved_sodium(group) >= 2300:
        warn_checks.append("sodium_under_limit")
    if category == "wweia_low_protein" and resolved_protein(group) < 50:
        warn_checks.append("protein_minimum_met")
    if warn_checks:
        expected["warn_checks"] = warn_checks
    if unresolved_count > 0:
        expected["decision"] = "block"
        expected["block_checks"] = ["quantities_resolvable"]
    elif warn_checks:
        expected["decision"] = "warn"

    case = {
        "case_id": "pending",
        "category": category,
        "description": description_for_case(category, group, full_day=full_day),
        "source_text": source_text_from_meals(meals),
        "source_refs": [
            {
                "source": SOURCE_ID,
                "source_id": f"SEQN {seqn} day {day}",
                "data_type": "NHANES Dietary Interview - Individual Foods",
                "note": f"Lines {line_range(group)}; adult reliable recall; {len(group)} reported food/beverage rows.",
            }
        ],
        "settings": settings_for_case(len(meals), full_day=full_day),
        "plan": {
            "schema_version": "0.1",
            "plan_id": "pending-plan",
            "description": f"{category.replace('_', ' ')} generated from WWEIA/NHANES rows",
            "days": [{"day": 1, "meals": meals}],
            "shopping_list": [],
            "prep_notes": ["Generated from public deidentified NHANES dietary recall rows."],
        },
        "expected": expected,
        "tags": [
            "wweia",
            "nhanes",
            "dietary-recall",
            "real-intake-patterns",
            category,
        ],
        "source_metrics": source_metrics(group),
    }
    return case


def renumber_cases(cases: list[dict[str, Any]]) -> list[dict[str, Any]]:
    result = []
    counts: Counter[str] = Counter()
    for index, case in enumerate(cases, start=1):
        counts[case["category"]] += 1
        case_id = f"{case['category']}-{counts[case['category']]:03d}"
        case["case_id"] = case_id
        case["plan"]["plan_id"] = f"{case_id}-plan"
        result.append(case)
    return result


def meals_from_items(group: list[RecallItem], *, full_day: bool) -> list[dict[str, Any]]:
    meal_groups: dict[tuple[int, int], list[RecallItem]] = defaultdict(list)
    for item in group:
        key = (item.occasion_code, item.time_seconds) if full_day else (item.occasion_code, item.time_seconds)
        meal_groups[key].append(item)

    meals = []
    for (occasion_code, time_seconds), items in sorted(meal_groups.items(), key=lambda pair: (pair[0][1], pair[0][0])):
        label = OCCASION_LABELS.get(occasion_code, "Other")
        meal_name = f"{label} {format_time(time_seconds)}"
        meals.append({"name": meal_name, "items": [food_item(item) for item in sorted(items, key=lambda item: item.line)]})
    return meals


def food_item(item: RecallItem) -> dict[str, Any]:
    result: dict[str, Any] = {
        "food": item.food_description,
        "source_food_code": item.food_code,
        "quantity": round2(item.grams),
        "unit": "g",
    }
    if not item.known_local:
        result["resolution_status"] = "unresolved"
        result["unresolved_reason"] = "unknown_food"
    return result


def source_text_from_meals(meals: list[dict[str, Any]]) -> str:
    chunks = []
    for meal in meals:
        foods = "; ".join(f"{item['quantity']:g} g {item['food']}" for item in meal["items"])
        chunks.append(f"{meal['name']}: {foods}")
    return " | ".join(chunks)


def settings_for_case(meal_count: int, *, full_day: bool) -> dict[str, Any]:
    if not full_day:
        return {
            "nutrition_targets": {"calorie_target_kcal": 0, "protein_target_g": 0},
            "verification_constraints": {
                "days": 1,
                "meals_per_day": meal_count,
                "allergies": [],
                "excluded_foods": [],
                "max_sodium_mg_per_day": 0,
                "max_added_sugar_g_per_meal": 0,
                "max_saturated_fat_pct_calories": 0,
                "calorie_tolerance_pct": 0,
                "requires_prep_safety_notes": False,
            },
        }
    return {
        "nutrition_targets": {"calorie_target_kcal": 2000, "protein_target_g": 50},
        "verification_constraints": {
            "days": 1,
            "meals_per_day": meal_count,
            "allergies": [],
            "excluded_foods": [],
            "max_sodium_mg_per_day": 2300,
            "max_added_sugar_g_per_meal": 10,
            "max_saturated_fat_pct_calories": 10,
            "calorie_tolerance_pct": 40,
            "requires_prep_safety_notes": False,
        },
    }


def description_for_case(category: str, group: list[RecallItem], *, full_day: bool) -> str:
    scope = "full adult recall day" if full_day else "single adult eating occasion"
    metrics = source_metrics(group)
    return (
        f"{scope} from WWEIA/NHANES 2021-2023; category={category}; "
        f"items={metrics['food_items']}; local_catalog_known={metrics['known_local_items']}."
    )


def source_metrics(group: list[RecallItem]) -> dict[str, Any]:
    total = len(group)
    known = sum(1 for item in group if item.known_local)
    return {
        "seqn": group[0].seqn,
        "recall_day": group[0].day,
        "food_items": total,
        "known_local_items": known,
        "unresolved_expected_items": total - known,
        "known_local_rate": round4(known / total if total else 0),
        "source_energy_kcal": round1(sum(item.energy_kcal for item in group)),
        "source_protein_g": round1(sum(item.protein_g for item in group)),
        "source_sodium_mg": round1(sum(item.sodium_mg for item in group)),
        "resolved_source_energy_kcal": round1(sum(item.energy_kcal for item in group if item.known_local)),
        "resolved_source_protein_g": round1(sum(item.protein_g for item in group if item.known_local)),
        "resolved_source_sodium_mg": round1(sum(item.sodium_mg for item in group if item.known_local)),
        "source_food_codes": sorted({item.food_code for item in group}),
        "unresolved_source_food_codes": sorted({item.food_code for item in group if not item.known_local}),
        "unresolved_source_foods": sorted(
            [
                {"source_food_code": code, "description": description}
                for code, description in {
                    item.food_code: item.food_description for item in group if not item.known_local
                }.items()
            ],
            key=lambda item: item["source_food_code"],
        ),
    }


def dataset_summary(cases: list[dict[str, Any]]) -> dict[str, Any]:
    counts = Counter(case["category"] for case in cases)
    total_items = sum(case["source_metrics"]["food_items"] for case in cases)
    known = sum(case["source_metrics"]["known_local_items"] for case in cases)
    unresolved_codes: Counter[tuple[str, str]] = Counter()
    for case in cases:
        for food in case["source_metrics"]["unresolved_source_foods"]:
            unresolved_codes[(food["source_food_code"], food["description"])] += 1
    return {
        "case_count": len(cases),
        "category_counts": dict(sorted(counts.items())),
        "food_items": total_items,
        "known_local_items": known,
        "expected_unresolved_items": total_items - known,
        "known_local_rate": round4(known / total_items if total_items else 0),
        "top_unresolved_source_food_codes": [
            {"source_food_code": code, "description": description, "case_count": count}
            for (code, description), count in unresolved_codes.most_common(20)
        ],
    }


def known_ratio(group: list[RecallItem]) -> float:
    return sum(1 for item in group if item.known_local) / len(group)


def resolved_sodium(group: list[RecallItem]) -> float:
    return sum(item.sodium_mg for item in group if item.known_local)


def resolved_protein(group: list[RecallItem]) -> float:
    return sum(item.protein_g for item in group if item.known_local)


def line_range(group: list[RecallItem]) -> str:
    lines = sorted(item.line for item in group)
    if not lines:
        return "unknown"
    return f"{lines[0]}-{lines[-1]}" if len(lines) > 1 else str(lines[0])


def format_time(seconds: int) -> str:
    seconds = max(0, min(seconds, 24 * 60 * 60 - 1))
    hour = seconds // 3600
    minute = (seconds % 3600) // 60
    return f"{hour:02d}:{minute:02d}"


def load_fndds_food_descriptions(path: Path) -> dict[str, str]:
    descriptions = {}
    for row in records_from_xlsx(path):
        code = normalize_food_code(row["Food code"])
        description = normalize_label(row["Main food description"])
        if code and description:
            descriptions[code] = description
    return descriptions


def records_from_xlsx(path: Path) -> list[dict[str, str]]:
    rows = list(read_first_sheet(path))
    if len(rows) < 3:
        raise ValueError(f"{path} has no data rows")
    headers = [normalize_header(value) for value in rows[1]]
    records = []
    for row in rows[2:]:
        if not any(row):
            continue
        record = {}
        for idx, header in enumerate(headers):
            if header:
                record[header] = row[idx] if idx < len(row) else ""
        records.append(record)
    return records


def read_first_sheet(path: Path) -> list[list[str]]:
    if not path.exists():
        raise FileNotFoundError(path)
    with ZipFile(path) as zf:
        shared = shared_strings(zf)
        workbook = ElementTree.fromstring(zf.read("xl/workbook.xml"))
        rels = ElementTree.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
        relmap = {rel.attrib["Id"]: rel.attrib["Target"] for rel in rels}
        ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
        sheet = workbook.find(f"{ns}sheets").find(f"{ns}sheet")
        if sheet is None:
            raise ValueError(f"{path} has no first worksheet")
        rel_id = sheet.attrib["{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"]
        target = relmap[rel_id]
        if not target.startswith("worksheets/"):
            target = "worksheets/" + target.split("/")[-1]
        root = ElementTree.fromstring(zf.read("xl/" + target))
        rows = []
        for row in root.findall(f".//{ns}sheetData/{ns}row"):
            values = []
            for cell in row.findall(f"{ns}c"):
                idx = column_index(cell.attrib.get("r", "A"))
                while len(values) <= idx:
                    values.append("")
                values[idx] = cell_value(cell, shared)
            rows.append(values)
        return rows


def shared_strings(zf: ZipFile) -> list[str]:
    try:
        root = ElementTree.fromstring(zf.read("xl/sharedStrings.xml"))
    except KeyError:
        return []
    ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    values = []
    for item in root.findall(f"{ns}si"):
        values.append("".join(text.text or "" for text in item.iter(f"{ns}t")))
    return values


def column_index(cell_ref: str) -> int:
    letters = "".join(ch for ch in cell_ref if ch.isalpha())
    index = 0
    for ch in letters:
        index = index * 26 + ord(ch.upper()) - 64
    return index - 1


def cell_value(cell: ElementTree.Element, shared: list[str]) -> str:
    ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    value = cell.find(f"{ns}v")
    if value is None:
        return ""
    if cell.attrib.get("t") == "s":
        return shared[int(value.text or "0")]
    return value.text or ""


def read_xpt_rows(path: Path, wanted_columns: set[str]) -> Iterable[dict[str, float | None]]:
    data, variables, row_start, row_length = read_xpt_metadata(path)
    selected = [variable for variable in variables if variable.name in wanted_columns]
    if len(selected) != len(wanted_columns):
        found = {variable.name for variable in selected}
        missing = sorted(wanted_columns - found)
        raise ValueError(f"{path} is missing expected XPT columns: {missing}")
    for offset in range(row_start, len(data) - row_length + 1, row_length):
        row = {}
        for variable in selected:
            raw = data[offset + variable.position : offset + variable.position + variable.length]
            if variable.var_type == 1:
                row[variable.name] = xpt_numeric(raw)
            else:
                row[variable.name] = raw.decode("latin1").strip()
        yield row


def read_xpt_metadata(path: Path) -> tuple[bytes, list[XPTVariable], int, int]:
    if not path.exists():
        raise FileNotFoundError(path)
    data = path.read_bytes()
    namestr_header = data[7 * 80 : 8 * 80].decode("latin1")
    if "NAMESTR HEADER RECORD" not in namestr_header:
        raise ValueError(f"{path} is not a supported SAS transport v5 file")
    variable_count = int(namestr_header[48:58])
    namestr_start = 8 * 80
    variables = []
    for index in range(variable_count):
        raw = data[namestr_start + index * 140 : namestr_start + (index + 1) * 140]
        variables.append(
            XPTVariable(
                name=raw[8:16].decode("latin1").strip(),
                var_type=int.from_bytes(raw[0:2], "big"),
                length=int.from_bytes(raw[4:6], "big"),
                position=int.from_bytes(raw[80:88], "big"),
            )
        )
    obs_header_start = namestr_start + variable_count * 140
    while obs_header_start % 80:
        obs_header_start += 1
    obs_header = data[obs_header_start : obs_header_start + 80].decode("latin1")
    if "OBS     HEADER RECORD" not in obs_header:
        raise ValueError(f"{path} has an unexpected XPT observation header")
    row_start = obs_header_start + 80
    row_length = sum(variable.length for variable in variables)
    return data, variables, row_start, row_length


def xpt_numeric(raw: bytes) -> float | None:
    if not any(raw):
        return 0.0
    if raw[0] == 0x2E:
        return None
    first = raw[0]
    sign = -1 if first & 0x80 else 1
    exponent = (first & 0x7F) - 64
    fraction = int.from_bytes(raw[1:], "big") / (16**14)
    value = sign * fraction * (16**exponent)
    if math.isclose(value, round(value)):
        return float(round(value))
    return value


def normalize_food_code(value: str) -> str:
    return str(value).strip().split(".")[0]


def normalize_header(value: str) -> str:
    return re.sub(r"\s+", " ", value.replace("\n", " ")).strip()


def normalize_label(value: str) -> str:
    return re.sub(r"\s+", " ", value.replace("\n", " ")).strip().strip("; ")


def round1(value: float) -> float:
    if math.isclose(value, round(value)):
        return int(round(value))
    return round(value, 1)


def round2(value: float) -> float:
    if math.isclose(value, round(value)):
        return int(round(value))
    return round(value, 2)


def round4(value: float) -> float:
    return round(value, 4)


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
