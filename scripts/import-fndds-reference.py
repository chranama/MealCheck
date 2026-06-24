#!/usr/bin/env python3
"""Import the full FNDDS 2021-2023 reference layer.

This script preserves every FNDDS food row as local reference data, then derives
a conservative resolver-candidate layer. The candidate layer is intentionally
not the same as MealCheck's reviewed resolver catalog: ambiguous and composed
foods are quarantined instead of silently becoming auto-resolvable aliases.
"""

from __future__ import annotations

import argparse
import json
import math
import re
import sqlite3
from collections import Counter
from pathlib import Path
from typing import Any
from xml.etree import ElementTree
from zipfile import ZipFile


RELEASE = "2021-2023"
SOURCE_ID = "fndds-2021-2023"
DEFAULT_SOURCE_DIR = Path("/tmp/mealcheck-fndds-2021-2023")
DEFAULT_OUT_DIR = Path("data/reference/fndds-2021-2023")
ROOT_MANIFEST_PATH = Path("data/reference/fndds/source-manifest.json")

STATUS_ELIGIBLE_SPECIFIC = "eligible_specific"
STATUS_ELIGIBLE_GENERIC = "eligible_generic"
STATUS_REVIEW_REQUIRED = "review_required"
STATUS_QUARANTINED_AMBIGUOUS = "quarantined_ambiguous"
STATUS_QUARANTINED_MIXED_DISH = "quarantined_mixed_dish"
STATUS_QUARANTINED_RESTAURANT_OR_BRAND = "quarantined_restaurant_or_brand"
STATUS_QUARANTINED_PREPARATION_UNCLEAR = "quarantined_preparation_unclear"

VALID_STATUSES = {
    STATUS_ELIGIBLE_SPECIFIC,
    STATUS_ELIGIBLE_GENERIC,
    STATUS_REVIEW_REQUIRED,
    STATUS_QUARANTINED_AMBIGUOUS,
    STATUS_QUARANTINED_MIXED_DISH,
    STATUS_QUARANTINED_RESTAURANT_OR_BRAND,
    STATUS_QUARANTINED_PREPARATION_UNCLEAR,
}

VALID_FLAGS = {
    "nfs",
    "not_further_specified",
    "not_specified_as_to",
    "generic_other",
    "generic_name",
    "mixed_dish",
    "sandwich",
    "pizza",
    "burrito",
    "taco",
    "casserole",
    "soup_or_stew",
    "restaurant_or_fast_food",
    "home_recipe",
    "brand_or_product_style",
    "preparation_unclear",
    "added_fat_unspecified",
    "multi_component_allergen_risk",
    "missing_required_nutrients",
    "missing_portion_data",
}

ALLOWLIST_CODES = {
    "56205001": "Rice, white, cooked, no added fat",
    "56205011": "Rice, brown, cooked, no added fat",
    "61210000": "Orange juice, 100%",
    "64104010": "Apple juice, 100%",
    "92101000": "Coffee, brewed",
    "92302000": "Tea, black, brewed",
    "94000100": "Water, tap",
    "94100100": "Water, bottled",
}

AMBIGUOUS_PATTERNS = [
    (r"\bnfs\b", "nfs"),
    (r"not further specified", "not_further_specified"),
    (r"\bns as to\b", "not_specified_as_to"),
    (r"not specified", "not_specified_as_to"),
    (r"\bunknown\b", "generic_other"),
    (r"^other\b|\bother$", "generic_other"),
    (r"variety not specified", "not_specified_as_to"),
    (r"brand not specified", "not_specified_as_to"),
]

MIXED_DISH_PATTERNS = [
    (r"\bsandwich", "sandwich"),
    (r"\bpizza\b", "pizza"),
    (r"\bburrito\b", "burrito"),
    (r"\btaco\b", "taco"),
    (r"\bcasserole\b", "casserole"),
    (r"\blasagna\b", "mixed_dish"),
    (r"macaroni (and|&) cheese", "mixed_dish"),
    (r"pasta .* sauce", "mixed_dish"),
    (r"\bspaghetti .* sauce", "mixed_dish"),
    (r"\bsoup\b|\bstew\b", "soup_or_stew"),
    (r"\bmeal\b", "mixed_dish"),
]

RESTAURANT_OR_BRAND_PATTERNS = [
    (r"\brestaurant\b", "restaurant_or_fast_food"),
    (r"\bfast food\b", "restaurant_or_fast_food"),
    (r"\bcafeteria\b", "restaurant_or_fast_food"),
    (r"\bschool lunch\b", "restaurant_or_fast_food"),
    (r"\bcommercial\b", "brand_or_product_style"),
    (r"\bfrom a mix\b", "brand_or_product_style"),
    (r"\bready-to-serve\b", "brand_or_product_style"),
]

PREPARATION_PATTERNS = [
    (r"\bfat added\b", "added_fat_unspecified"),
    (r"\bwith added fat\b", "added_fat_unspecified"),
    (r"\badded fat\b", "added_fat_unspecified"),
    (r"\bfried\b.*\bunknown\b", "preparation_unclear"),
    (r"\bas ingredient\b", "preparation_unclear"),
]

GENERIC_ONE_WORDS = {
    "cheese",
    "fish",
    "meat",
    "sauce",
    "soup",
    "sandwich",
    "cereal",
    "juice",
    "bread",
    "rice",
    "pasta",
}

REQUIRED_NUTRIENT_KEYS = [
    "energy_kcal",
    "protein_g",
    "carbohydrate_g",
    "fat_g",
    "saturated_fat_g",
    "sodium_mg",
    "total_sugar_g",
    "fiber_g",
]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-dir", default=str(DEFAULT_SOURCE_DIR))
    parser.add_argument("--out-dir", default=str(DEFAULT_OUT_DIR))
    args = parser.parse_args()

    source_dir = Path(args.source_dir)
    out_dir = Path(args.out_dir)

    foods = records_from_xlsx(source_dir / "foods-and-beverages.xlsx")
    nutrients = records_from_xlsx(source_dir / "nutrient-values.xlsx")
    portions = records_from_xlsx(source_dir / "portions-and-weights.xlsx")

    nutrient_by_code = {normalize_food_code(row["Food code"]): row for row in nutrients}
    portions_by_code: dict[str, list[dict[str, str]]] = {}
    for row in portions:
        portions_by_code.setdefault(normalize_food_code(row["Food code"]), []).append(row)

    food_rows = []
    nutrient_rows = []
    portion_rows = []
    for source_food in foods:
        food_code = normalize_food_code(source_food["Food code"])
        nutrient_source = nutrient_by_code.get(food_code)
        nutrients_doc = nutrients_for(nutrient_source)
        portion_options = portions_for(portions_by_code.get(food_code, []))
        classification = classify_food(source_food, nutrients_doc, portion_options)

        food_row = {
            "release": RELEASE,
            "food_code": food_code,
            "main_description": normalize_label(source_food["Main food description"]),
            "additional_description": normalize_label(source_food.get("Additional food description", "")),
            "wweia_category_number": normalize_label(source_food["WWEIA Category number"]),
            "wweia_category_description": normalize_label(source_food["WWEIA Category description"]),
            "nutrients_per_100g": nutrients_doc,
            "portion_options": portion_options,
            "allergens": allergens_for(source_food),
            "food_groups": food_groups_for(source_food),
            "candidate_status": classification["candidate_status"],
            "ambiguity_flags": classification["ambiguity_flags"],
            "candidate_reason": classification["candidate_reason"],
            "source_refs": source_refs(food_code, source_food),
        }
        food_rows.append(food_row)

        nutrient_rows.append(
            {
                "release": RELEASE,
                "food_code": food_code,
                "nutrients_per_100g": nutrients_doc,
                "source_refs": source_refs(food_code, source_food, data_type="FNDDS Nutrient Values"),
            }
        )
        for portion in portion_options:
            portion_rows.append(
                {
                    "release": RELEASE,
                    "food_code": food_code,
                    **portion,
                    "source_refs": source_refs(food_code, source_food, data_type="FNDDS Portions and Weights"),
                }
            )

    food_rows.sort(key=lambda row: row["food_code"])
    nutrient_rows.sort(key=lambda row: row["food_code"])
    portion_rows.sort(key=lambda row: (row["food_code"], row["description"], row["grams"]))

    resolver_candidates = [row for row in food_rows if row["candidate_status"] in {STATUS_ELIGIBLE_SPECIFIC, STATUS_ELIGIBLE_GENERIC}]
    quarantined_foods = [row for row in food_rows if row["candidate_status"].startswith("quarantined_")]
    review_required_foods = [row for row in food_rows if row["candidate_status"] == STATUS_REVIEW_REQUIRED]
    summary = classification_summary(food_rows, resolver_candidates, quarantined_foods, review_required_foods)

    out_dir.mkdir(parents=True, exist_ok=True)
    write_jsonl(out_dir / "foods.jsonl", food_rows)
    write_jsonl(out_dir / "nutrients.jsonl", nutrient_rows)
    write_jsonl(out_dir / "portions.jsonl", portion_rows)
    write_jsonl(out_dir / "resolver-candidates.jsonl", resolver_candidates)
    write_jsonl(out_dir / "quarantined-foods.jsonl", quarantined_foods)
    write_jsonl(out_dir / "review-required-foods.jsonl", review_required_foods)
    write_json(out_dir / "food-index.json", food_index(food_rows))
    write_json(out_dir / "classification-summary.json", summary)
    write_json(out_dir / "manifest.json", release_manifest(summary))
    write_sqlite(out_dir / "fndds.sqlite", food_rows)
    ROOT_MANIFEST_PATH.parent.mkdir(parents=True, exist_ok=True)
    write_json(ROOT_MANIFEST_PATH, source_manifest())

    print(f"wrote {out_dir}")
    print(json.dumps(summary, indent=2))


def nutrients_for(row: dict[str, str] | None) -> dict[str, float | None]:
    if row is None:
        return {key: None for key in REQUIRED_NUTRIENT_KEYS}
    return {
        "energy_kcal": round1(number(row["Energy (kcal)"])),
        "protein_g": round1(number(row["Protein (g)"])),
        "carbohydrate_g": round1(number(row["Carbohydrate (g)"])),
        "fat_g": round1(number(row["Total Fat (g)"])),
        "saturated_fat_g": round1(number(row["Fatty acids, total saturated (g)"])),
        "sodium_mg": round1(number(row["Sodium (mg)"])),
        "total_sugar_g": round1(number(row["Sugars, total (g)"])),
        "fiber_g": round1(number(row["Fiber, total dietary (g)"])),
    }


def portions_for(rows: list[dict[str, str]]) -> list[dict[str, Any]]:
    portions = []
    seen = set()
    for row in rows:
        description = normalize_label(row.get("Portion description", ""))
        grams = round2(number(row.get("Portion weight (g)", "0")))
        if not description or grams <= 0:
            continue
        key = (description.lower(), grams)
        if key in seen:
            continue
        seen.add(key)
        portions.append({"description": description, "grams": grams})
    portions.sort(key=lambda row: (row["description"].lower(), row["grams"]))
    return portions


def classify_food(source_food: dict[str, str], nutrients: dict[str, float], portions: list[dict[str, Any]]) -> dict[str, Any]:
    food_code = normalize_food_code(source_food["Food code"])
    main = normalize_label(source_food["Main food description"])
    additional = normalize_label(source_food.get("Additional food description", ""))
    category = normalize_label(source_food["WWEIA Category description"])
    text = " ".join([main, additional, category]).lower()

    flags = set()
    status = STATUS_ELIGIBLE_SPECIFIC
    reason = "Specific source food with complete required nutrients."

    for pattern, flag in AMBIGUOUS_PATTERNS:
        if re.search(pattern, text):
            flags.add(flag)
    for pattern, flag in MIXED_DISH_PATTERNS:
        if re.search(pattern, text):
            flags.add(flag)
    for pattern, flag in RESTAURANT_OR_BRAND_PATTERNS:
        if re.search(pattern, text):
            flags.add(flag)
    for pattern, flag in PREPARATION_PATTERNS:
        if re.search(pattern, text):
            flags.add(flag)
    if "home recipe" in text:
        flags.add("home_recipe")
    if contains_multiple_components(text):
        flags.add("multi_component_allergen_risk")
    if missing_required_nutrients(nutrients):
        flags.add("missing_required_nutrients")
    if not portions:
        flags.add("missing_portion_data")
    if generic_name(main):
        flags.add("generic_name")

    if food_code in ALLOWLIST_CODES:
        hard_flags = {"missing_required_nutrients"}
        if flags & hard_flags:
            status = STATUS_REVIEW_REQUIRED
            reason = "Allowlisted food still has required-data gaps and needs review."
        else:
            status = STATUS_ELIGIBLE_GENERIC
            reason = "Allowlisted generic source food is safe for candidate review."
            flags -= {
                "nfs",
                "generic_name",
                "generic_other",
                "not_further_specified",
                "not_specified_as_to",
                "missing_portion_data",
            }
        return classification(status, flags, reason)

    if flags & {"missing_required_nutrients"}:
        status = STATUS_REVIEW_REQUIRED
        reason = "Missing one or more required MealCheck nutrient fields."
    elif flags & {"nfs", "not_further_specified", "not_specified_as_to", "generic_other"}:
        status = STATUS_QUARANTINED_AMBIGUOUS
        reason = "Food description is not specific enough for automatic resolver-candidate use."
    elif flags & {"sandwich", "pizza", "burrito", "taco", "casserole", "soup_or_stew", "mixed_dish", "home_recipe", "multi_component_allergen_risk"}:
        status = STATUS_QUARANTINED_MIXED_DISH
        reason = "Food appears to be a composed or multi-component dish requiring manual review."
    elif flags & {"restaurant_or_fast_food", "brand_or_product_style"}:
        status = STATUS_QUARANTINED_RESTAURANT_OR_BRAND
        reason = "Food appears restaurant-, cafeteria-, commercial-, or product-style."
    elif flags & {"preparation_unclear", "added_fat_unspecified"}:
        status = STATUS_QUARANTINED_PREPARATION_UNCLEAR
        reason = "Preparation or added-fat source is unclear."
    elif flags & {"generic_name"}:
        status = STATUS_REVIEW_REQUIRED
        reason = "Food name is broad and needs review before resolver-candidate use."
    elif flags & {"missing_portion_data"}:
        status = STATUS_ELIGIBLE_SPECIFIC
        reason = "Specific source food; household portions absent, but grams remain usable."
    return classification(status, flags, reason)


def classification(status: str, flags: set[str], reason: str) -> dict[str, Any]:
    invalid_flags = sorted(flags - VALID_FLAGS)
    if invalid_flags:
        raise ValueError(f"invalid ambiguity flags {invalid_flags}")
    if status not in VALID_STATUSES:
        raise ValueError(f"invalid candidate status {status}")
    return {
        "candidate_status": status,
        "ambiguity_flags": sorted(flags),
        "candidate_reason": reason,
    }


def contains_multiple_components(text: str) -> bool:
    component_terms = [
        " with cheese",
        " with meat",
        " with sauce",
        " with gravy",
        " with vegetables",
        " with egg",
        " and cheese",
        " and meat",
        " and beans",
        " and rice",
    ]
    return any(term in text for term in component_terms)


def generic_name(main: str) -> bool:
    cleaned = re.sub(r"[^a-zA-Z ]+", " ", main).strip().lower()
    words = [word for word in cleaned.split() if word]
    return len(words) == 1 and words[0] in GENERIC_ONE_WORDS


def missing_required_nutrients(nutrients: dict[str, float]) -> bool:
    for key in REQUIRED_NUTRIENT_KEYS:
        value = nutrients.get(key)
        if value is None or value < 0:
            return True
    return False


def source_refs(food_code: str, source_food: dict[str, str], *, data_type: str = "FNDDS At A Glance") -> list[dict[str, str]]:
    return [
        {
            "source": SOURCE_ID,
            "source_id": food_code,
            "data_type": data_type,
            "note": f"{normalize_label(source_food['WWEIA Category number'])} {normalize_label(source_food['WWEIA Category description'])}",
        }
    ]


def allergens_for(source_food: dict[str, str]) -> list[str]:
    text = source_text(source_food)
    allergens = set()
    if any(term in text for term in ["milk", "cheese", "yogurt", "butter", "cream", "ice cream", "ranch", "caesar", "pizza", "cheeseburger", "macaroni and cheese", "lasagna"]):
        if not any(term in text for term in ["almond milk", "oat milk", "coconut milk", "plant-based milk", "soy milk"]):
            allergens.add("milk")
    if any(term in text for term in ["egg", "mayonnaise", "mayo"]) and "vegan" not in text:
        allergens.add("eggs")
    if any(term in text for term in ["fish", "salmon", "tuna", "tilapia", "cod", "sardine", "anchovy", "trout"]):
        allergens.add("fish")
    if any(term in text for term in ["shrimp", "crab", "lobster", "crayfish", "shellfish", "clam", "oyster", "mussel"]):
        allergens.add("crustacean shellfish")
    if "peanut" in text:
        allergens.add("peanuts")
    if any(term in text for term in ["almond", "cashew", "walnut", "hazelnut", "pecan", "pistachio", "macadamia"]):
        allergens.add("tree nuts")
    if any(term in text for term in ["wheat", "bread", "bagel", "flour tortilla", "pasta", "spaghetti", "macaroni", "lasagna", "cereal", "granola", "pancake", "waffle", "muffin", "cookie", "brownie", "cake", "pizza", "burger", "hot dog", "ramen"]):
        allergens.add("wheat")
    if any(term in text for term in ["soy", "tofu", "soybean", "shoyu", "tamari", "teriyaki"]):
        allergens.add("soybeans")
    if any(term in text for term in ["sesame", "hummus", "tahini"]):
        allergens.add("sesame")
    order = ["milk", "eggs", "fish", "crustacean shellfish", "tree nuts", "peanuts", "wheat", "soybeans", "sesame"]
    return [allergen for allergen in order if allergen in allergens]


def food_groups_for(source_food: dict[str, str]) -> list[str]:
    text = source_text(source_food)
    groups = set()
    if any(term in text for term in ["water", "coffee", "tea", "juice", "soft drink", "soda", "lemonade", "sports drink", "smoothie", "beer", "wine"]):
        groups.add("beverages")
    if any(term in text for term in ["sauce", "ketchup", "mustard", "salsa", "vinegar", "lemon juice", "hot sauce", "syrup", "jelly", "jam", "pickle"]):
        groups.add("condiments")
    if any(term in text for term in ["milk", "yogurt", "cheese", "cream", "ice cream"]):
        groups.add("dairy")
    if any(term in text for term in ["oil", "butter", "mayonnaise", "dressing", "shortening", "margarine"]):
        groups.add("fats")
    if any(term in text for term in ["apple", "banana", "orange", "strawberr", "blueberr", "grape", "watermelon", "pineapple", "cantaloupe", "clementine", "pear", "peach", "fruit"]):
        groups.add("fruits")
    if any(term in text for term in ["chicken", "beef", "pork", "turkey", "ham", "bacon", "sausage", "hot dog", "salmon", "tuna", "shrimp", "tilapia", "cod", "egg", "tofu", "bean", "chickpea", "lentil", "hummus", "peanut butter", "almond", "cashew", "walnut", "protein"]):
        groups.add("protein")
    if any(term in text for term in ["oatmeal", "brown rice", "quinoa", "whole wheat", "whole grain", "barley", "oat cereal"]):
        groups.add("whole_grains")
    if any(term in text for term in ["white rice", "pasta", "white bread", "bagel", "flour tortilla", "corn tortilla", "corn flakes", "cereal", "pancake", "waffle", "muffin", "macaroni", "lasagna", "ramen", "pizza", "burrito", "taco", "roll"]):
        groups.add("refined_grains")
    if any(term in text for term in ["vegetable", "broccoli", "spinach", "carrot", "tomato", "lettuce", "cucumber", "onion", "potato", "corn", "green bean", "pea", "cauliflower", "cabbage", "mushroom", "zucchini", "squash", "asparagus", "kale", "brussels", "garlic", "pepper", "avocado"]):
        groups.add("vegetables")
    if any(term in text for term in ["fries", "chips", "pretzel", "popcorn", "granola bar", "cookie", "brownie", "cake", "candy", "chocolate", "snack"]):
        groups.add("snacks")
    if not groups:
        groups.add("snacks")
    order = ["beverages", "condiments", "dairy", "fats", "fruits", "protein", "refined_grains", "snacks", "vegetables", "whole_grains"]
    return [group for group in order if group in groups]


def source_text(source_food: dict[str, str]) -> str:
    return " ".join(
        [
            normalize_label(source_food["Main food description"]),
            normalize_label(source_food.get("Additional food description", "")),
            normalize_label(source_food["WWEIA Category description"]),
        ]
    ).lower()


def classification_summary(
    foods: list[dict[str, Any]],
    resolver_candidates: list[dict[str, Any]],
    quarantined_foods: list[dict[str, Any]],
    review_required_foods: list[dict[str, Any]],
) -> dict[str, Any]:
    status_counts = Counter(row["candidate_status"] for row in foods)
    flag_counts = Counter(flag for row in foods for flag in row["ambiguity_flags"])
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "food_count": len(foods),
        "resolver_candidate_count": len(resolver_candidates),
        "quarantined_count": len(quarantined_foods),
        "review_required_count": len(review_required_foods),
        "status_counts": dict(sorted(status_counts.items())),
        "flag_counts": dict(sorted(flag_counts.items())),
        "allowlist_codes": dict(sorted(ALLOWLIST_CODES.items())),
    }


def food_index(foods: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "food_count": len(foods),
        "foods": {
            row["food_code"]: {
                "main_description": row["main_description"],
                "wweia_category_description": row["wweia_category_description"],
                "candidate_status": row["candidate_status"],
                "ambiguity_flags": row["ambiguity_flags"],
            }
            for row in foods
        },
    }


def release_manifest(summary: dict[str, Any]) -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "source": SOURCE_ID,
        "source_workbooks": [
            "foods-and-beverages.xlsx",
            "nutrient-values.xlsx",
            "portions-and-weights.xlsx",
        ],
        "generated_artifacts": [
            "foods.jsonl",
            "nutrients.jsonl",
            "portions.jsonl",
            "resolver-candidates.jsonl",
            "quarantined-foods.jsonl",
            "review-required-foods.jsonl",
            "food-index.json",
            "classification-summary.json",
            "fndds.sqlite",
        ],
        "generation_command": "python3 scripts/import-fndds-reference.py",
        "summary": summary,
    }


def source_manifest() -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "purpose": "Local FNDDS reference imports used for candidate resolver review and WWEIA catalog-gap mining.",
        "cycles": [
            {
                "release": RELEASE,
                "source": SOURCE_ID,
                "source_urls": [
                    "https://www.ars.usda.gov/northeast-area/beltsville-md-bhnrc/beltsville-human-nutrition-research-center/food-surveys-research-group/docs/fndds-download-databases/",
                    "https://fdc.nal.usda.gov/download-datasets/",
                ],
                "expected_local_raw_paths": [
                    str(DEFAULT_SOURCE_DIR / "foods-and-beverages.xlsx"),
                    str(DEFAULT_SOURCE_DIR / "nutrient-values.xlsx"),
                    str(DEFAULT_SOURCE_DIR / "portions-and-weights.xlsx"),
                ],
                "generated_artifact_dir": str(DEFAULT_OUT_DIR),
                "generation_command": "python3 scripts/import-fndds-reference.py",
            }
        ],
    }


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


def normalize_food_code(value: str) -> str:
    return str(value).strip().split(".")[0]


def normalize_lookup(value: str) -> str:
    return " ".join(value.lower().strip().split())


def normalize_header(value: str) -> str:
    return re.sub(r"\s+", " ", value.replace("\n", " ")).strip()


def normalize_label(value: str) -> str:
    return re.sub(r"\s+", " ", value.replace("\n", " ")).strip().strip("; ")


def number(value: str) -> float:
    if value == "":
        return 0.0
    return float(value)


def round1(value: float) -> float:
    if math.isclose(value, round(value)):
        return int(round(value))
    return round(value, 1)


def round2(value: float) -> float:
    if math.isclose(value, round(value)):
        return int(round(value))
    return round(value, 2)


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as file:
        for row in rows:
            file.write(json.dumps(row, sort_keys=True) + "\n")


def write_sqlite(path: Path, foods: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        path.unlink()
    conn = sqlite3.connect(path)
    try:
        conn.execute("pragma journal_mode = delete")
        conn.execute("pragma synchronous = full")
        conn.execute("pragma foreign_keys = on")
        conn.executescript(
            """
            create table fndds_foods(
              release text not null,
              food_code text primary key,
              main_description text not null,
              additional_description text,
              normalized_description text not null,
              wweia_category_number text,
              wweia_category_description text,
              candidate_status text not null,
              candidate_reason text not null
            );

            create table fndds_nutrients(
              food_code text primary key references fndds_foods(food_code),
              energy_kcal real,
              protein_g real,
              carbohydrate_g real,
              fat_g real,
              saturated_fat_g real,
              sodium_mg real,
              total_sugar_g real,
              fiber_g real
            );

            create table fndds_portions(
              food_code text not null references fndds_foods(food_code),
              description text not null,
              normalized_description text not null,
              grams real not null,
              primary key(food_code, description, grams)
            );

            create table fndds_ambiguity_flags(
              food_code text not null references fndds_foods(food_code),
              flag text not null,
              primary key(food_code, flag)
            );

            create table fndds_allergens(
              food_code text not null references fndds_foods(food_code),
              allergen text not null,
              primary key(food_code, allergen)
            );

            create table fndds_food_groups(
              food_code text not null references fndds_foods(food_code),
              food_group text not null,
              primary key(food_code, food_group)
            );

            create index idx_fndds_foods_normalized_description
              on fndds_foods(normalized_description);
            create index idx_fndds_foods_candidate_status
              on fndds_foods(candidate_status);
            create index idx_fndds_portions_food_code
              on fndds_portions(food_code);
            create index idx_fndds_flags_food_code
              on fndds_ambiguity_flags(food_code);
            create index idx_fndds_allergens_food_code
              on fndds_allergens(food_code);
            create index idx_fndds_food_groups_food_code
              on fndds_food_groups(food_code);
            """
        )
        with conn:
            for food in foods:
                conn.execute(
                    """
                    insert into fndds_foods(
                      release, food_code, main_description, additional_description,
                      normalized_description, wweia_category_number,
                      wweia_category_description, candidate_status, candidate_reason
                    ) values (?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        food["release"],
                        food["food_code"],
                        food["main_description"],
                        food["additional_description"],
                        normalize_lookup(food["main_description"]),
                        food["wweia_category_number"],
                        food["wweia_category_description"],
                        food["candidate_status"],
                        food["candidate_reason"],
                    ),
                )
                nutrients = food["nutrients_per_100g"]
                conn.execute(
                    """
                    insert into fndds_nutrients(
                      food_code, energy_kcal, protein_g, carbohydrate_g, fat_g,
                      saturated_fat_g, sodium_mg, total_sugar_g, fiber_g
                    ) values (?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        food["food_code"],
                        nutrients["energy_kcal"],
                        nutrients["protein_g"],
                        nutrients["carbohydrate_g"],
                        nutrients["fat_g"],
                        nutrients["saturated_fat_g"],
                        nutrients["sodium_mg"],
                        nutrients["total_sugar_g"],
                        nutrients["fiber_g"],
                    ),
                )
                for portion in food["portion_options"]:
                    conn.execute(
                        """
                        insert into fndds_portions(
                          food_code, description, normalized_description, grams
                        ) values (?, ?, ?, ?)
                        """,
                        (
                            food["food_code"],
                            portion["description"],
                            normalize_lookup(portion["description"]),
                            portion["grams"],
                        ),
                    )
                for flag in food["ambiguity_flags"]:
                    conn.execute(
                        "insert into fndds_ambiguity_flags(food_code, flag) values (?, ?)",
                        (food["food_code"], flag),
                    )
                for allergen in food["allergens"]:
                    conn.execute(
                        "insert into fndds_allergens(food_code, allergen) values (?, ?)",
                        (food["food_code"], allergen),
                    )
                for food_group in food["food_groups"]:
                    conn.execute(
                        "insert into fndds_food_groups(food_code, food_group) values (?, ?)",
                        (food["food_code"], food_group),
                    )
        conn.execute("pragma optimize")
    finally:
        conn.close()


if __name__ == "__main__":
    main()
