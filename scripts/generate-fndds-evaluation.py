#!/usr/bin/env python3
"""Generate MealCheck's FNDDS-grounded catalog and evaluation fixtures.

The script reads the USDA FNDDS 2021-2023 "At A Glance" workbooks and writes:

- data/nutrients/fixture-catalog-v1.json
- data/evaluation/fndds-grounded-meal-plans-v1.json

The checked-in JSON is deterministic so CI and local development do not require
network access. Regeneration requires the source workbooks listed in
docs/evaluation.md.
"""

from __future__ import annotations

import argparse
import json
import math
import re
from pathlib import Path
from typing import Any
from xml.etree import ElementTree
from zipfile import ZipFile


SOURCE_ID = "fndds-2021-2023"
CATALOG_PATH = Path("data/nutrients/fixture-catalog-v1.json")
DATASET_PATH = Path("data/evaluation/fndds-grounded-meal-plans-v1.json")

ALLOWED_GROUPS = {
    "beverages",
    "condiments",
    "dairy",
    "fats",
    "fruits",
    "protein",
    "refined_grains",
    "snacks",
    "vegetables",
    "whole_grains",
}


FOOD_SELECTION = [
    {"code": "56203056", "food_id": "oatmeal_cooked_plain", "name": "cooked oatmeal", "aliases": ["oatmeal", "plain oatmeal"]},
    {"code": "63203010", "food_id": "blueberries_raw", "name": "blueberries", "aliases": ["blueberry"]},
    {"code": "11411390", "food_id": "greek_yogurt_plain", "name": "plain Greek yogurt", "aliases": ["greek yogurt"]},
    {"code": "24122131", "food_id": "chicken_breast_cooked", "name": "chicken breast", "aliases": ["cooked chicken breast", "grilled chicken breast", "baked chicken breast"]},
    {"code": "56205011", "food_id": "brown_rice_cooked", "name": "brown rice", "aliases": ["cooked brown rice"]},
    {"code": "72201211", "food_id": "broccoli_cooked", "name": "broccoli", "aliases": ["cooked broccoli"]},
    {"code": "26137120", "food_id": "salmon_cooked", "name": "salmon", "aliases": ["cooked salmon", "baked salmon"]},
    {"code": "73403010", "food_id": "sweet_potato_baked", "name": "sweet potato", "aliases": ["baked sweet potato"]},
    {"code": "72125211", "food_id": "spinach_cooked", "name": "spinach", "aliases": ["cooked spinach"]},
    {"code": "82104000", "food_id": "olive_oil", "name": "olive oil", "aliases": ["extra virgin olive oil"]},
    {"code": "31103010", "food_id": "egg_boiled", "name": "boiled egg", "aliases": ["egg", "hard boiled egg"]},
    {"code": "63101000", "food_id": "apple_raw", "name": "apple", "aliases": ["raw apple"]},
    {"code": "41102080", "food_id": "black_beans", "name": "black beans", "aliases": ["canned black beans"]},
    {"code": "51300110", "food_id": "whole_wheat_bread", "name": "whole wheat bread", "aliases": ["wheat bread"]},
    {"code": "42204050", "food_id": "peanut_sauce", "name": "peanut sauce", "aliases": ["satay sauce"]},
    {"code": "58407030", "food_id": "ramen_noodles", "name": "ramen noodles", "aliases": ["instant ramen", "ramen soup"]},
    {"code": "41420300", "food_id": "soy_sauce", "name": "soy sauce", "aliases": ["shoyu", "tamari"]},
    {"code": "11111000", "food_id": "whole_milk", "name": "whole milk", "aliases": ["milk whole"]},
    {"code": "11112110", "food_id": "reduced_fat_milk", "name": "2% milk", "aliases": ["reduced fat milk"]},
    {"code": "11113000", "food_id": "skim_milk", "name": "skim milk", "aliases": ["fat free milk"]},
    {"code": "11350020", "food_id": "almond_milk_unsweetened", "name": "unsweetened almond milk", "aliases": ["almond milk"]},
    {"code": "11360200", "food_id": "oat_milk", "name": "oat milk", "aliases": []},
    {"code": "11370000", "food_id": "coconut_milk_beverage", "name": "coconut milk", "aliases": ["coconut milk beverage"]},
    {"code": "11411010", "food_id": "yogurt_plain", "name": "plain yogurt", "aliases": ["yogurt"]},
    {"code": "11511200", "food_id": "chocolate_milk", "name": "chocolate milk", "aliases": []},
    {"code": "14104100", "food_id": "cheddar_cheese", "name": "cheddar cheese", "aliases": ["cheddar"]},
    {"code": "14107030", "food_id": "mozzarella_cheese", "name": "mozzarella cheese", "aliases": ["part skim mozzarella"]},
    {"code": "14201010", "food_id": "cottage_cheese", "name": "cottage cheese", "aliases": []},
    {"code": "21500100", "food_id": "ground_beef_cooked", "name": "ground beef", "aliases": ["cooked ground beef"]},
    {"code": "21500310", "food_id": "beef_patty", "name": "beef patty", "aliases": ["hamburger patty"]},
    {"code": "21001000", "food_id": "steak", "name": "steak", "aliases": ["beef steak"]},
    {"code": "22101020", "food_id": "pork_chop", "name": "pork chop", "aliases": []},
    {"code": "24201120", "food_id": "roasted_turkey", "name": "roasted turkey", "aliases": ["turkey breast"]},
    {"code": "22311000", "food_id": "ham", "name": "ham", "aliases": ["sliced ham"]},
    {"code": "22600200", "food_id": "bacon", "name": "bacon", "aliases": ["pork bacon"]},
    {"code": "25221405", "food_id": "pork_sausage", "name": "pork sausage", "aliases": ["breakfast sausage"]},
    {"code": "25210210", "food_id": "hot_dog_beef", "name": "beef hot dog", "aliases": ["hot dog"]},
    {"code": "24152231", "food_id": "chicken_thigh_cooked", "name": "chicken thigh", "aliases": ["cooked chicken thigh"]},
    {"code": "24162130", "food_id": "chicken_wing_cooked", "name": "chicken wing", "aliases": ["baked chicken wing"]},
    {"code": "26155110", "food_id": "tuna_canned", "name": "canned tuna", "aliases": ["tuna"]},
    {"code": "26319130", "food_id": "shrimp_boiled", "name": "shrimp", "aliases": ["boiled shrimp", "steamed shrimp"]},
    {"code": "26158010", "food_id": "tilapia_cooked", "name": "tilapia", "aliases": ["baked tilapia"]},
    {"code": "26109120", "food_id": "cod_cooked", "name": "cod", "aliases": ["baked cod"]},
    {"code": "32129990", "food_id": "scrambled_egg", "name": "scrambled egg", "aliases": ["scrambled eggs"]},
    {"code": "41420010", "food_id": "tofu", "name": "tofu", "aliases": ["soybean curd"]},
    {"code": "41104020", "food_id": "pinto_beans", "name": "pinto beans", "aliases": []},
    {"code": "41106020", "food_id": "kidney_beans", "name": "kidney beans", "aliases": ["red kidney beans"]},
    {"code": "41302080", "food_id": "chickpeas", "name": "chickpeas", "aliases": ["garbanzo beans"]},
    {"code": "41305000", "food_id": "lentils", "name": "lentils", "aliases": []},
    {"code": "41205010", "food_id": "refried_beans", "name": "refried beans", "aliases": []},
    {"code": "41205070", "food_id": "hummus", "name": "hummus", "aliases": []},
    {"code": "42202000", "food_id": "peanut_butter", "name": "peanut butter", "aliases": []},
    {"code": "42101000", "food_id": "almonds", "name": "almonds", "aliases": ["raw almonds"]},
    {"code": "42104050", "food_id": "cashews", "name": "cashews", "aliases": ["raw cashews"]},
    {"code": "42116000", "food_id": "walnuts", "name": "walnuts", "aliases": []},
    {"code": "56205001", "food_id": "white_rice_cooked", "name": "white rice", "aliases": ["cooked white rice"]},
    {"code": "56204005", "food_id": "quinoa_cooked", "name": "quinoa", "aliases": ["cooked quinoa"]},
    {"code": "56130000", "food_id": "pasta_cooked", "name": "pasta", "aliases": ["cooked pasta", "spaghetti"]},
    {"code": "56132990", "food_id": "whole_grain_pasta", "name": "whole grain pasta", "aliases": ["whole wheat pasta"]},
    {"code": "56207160", "food_id": "couscous_cooked", "name": "couscous", "aliases": ["cooked couscous"]},
    {"code": "56200410", "food_id": "barley_cooked", "name": "barley", "aliases": ["cooked barley"]},
    {"code": "51101000", "food_id": "white_bread", "name": "white bread", "aliases": []},
    {"code": "51180010", "food_id": "bagel", "name": "bagel", "aliases": []},
    {"code": "51301750", "food_id": "whole_wheat_bagel", "name": "whole wheat bagel", "aliases": []},
    {"code": "52215200", "food_id": "flour_tortilla", "name": "flour tortilla", "aliases": []},
    {"code": "52215100", "food_id": "corn_tortilla", "name": "corn tortilla", "aliases": []},
    {"code": "57134000", "food_id": "corn_flakes", "name": "corn flakes", "aliases": ["plain cereal"]},
    {"code": "57123000", "food_id": "o_cereal", "name": "o cereal", "aliases": ["plain oat cereal"]},
    {"code": "57119000", "food_id": "sweetened_cereal", "name": "sweetened cereal", "aliases": ["crunch cereal"]},
    {"code": "57227000", "food_id": "granola", "name": "granola", "aliases": []},
    {"code": "55101000", "food_id": "pancakes_plain", "name": "pancakes", "aliases": ["plain pancakes"]},
    {"code": "55200010", "food_id": "waffle_plain", "name": "waffle", "aliases": ["waffles"]},
    {"code": "52304100", "food_id": "oatmeal_muffin", "name": "oatmeal muffin", "aliases": ["muffin"]},
    {"code": "58145110", "food_id": "macaroni_and_cheese", "name": "macaroni and cheese", "aliases": ["mac and cheese"]},
    {"code": "58130015", "food_id": "lasagna_meat", "name": "lasagna", "aliases": ["meat lasagna"]},
    {"code": "63107010", "food_id": "banana_raw", "name": "banana", "aliases": []},
    {"code": "61119010", "food_id": "orange_raw", "name": "orange", "aliases": ["raw orange"]},
    {"code": "63223020", "food_id": "strawberries_raw", "name": "strawberries", "aliases": ["strawberry"]},
    {"code": "63123000", "food_id": "grapes_raw", "name": "grapes", "aliases": []},
    {"code": "63149010", "food_id": "watermelon_raw", "name": "watermelon", "aliases": []},
    {"code": "63105010", "food_id": "avocado_raw", "name": "avocado", "aliases": []},
    {"code": "63141010", "food_id": "pineapple_raw", "name": "pineapple", "aliases": []},
    {"code": "63109010", "food_id": "cantaloupe_raw", "name": "cantaloupe", "aliases": []},
    {"code": "61100600", "food_id": "clementine_raw", "name": "clementine", "aliases": ["mandarin orange"]},
    {"code": "63137010", "food_id": "pear_raw", "name": "pear", "aliases": []},
    {"code": "63135010", "food_id": "peach_raw", "name": "peach", "aliases": []},
    {"code": "74101000", "food_id": "tomato_raw", "name": "tomato", "aliases": ["raw tomato"]},
    {"code": "73101010", "food_id": "carrots_raw", "name": "carrots", "aliases": ["raw carrots", "carrot"]},
    {"code": "75113000", "food_id": "lettuce_raw", "name": "lettuce", "aliases": ["iceberg lettuce"]},
    {"code": "72116000", "food_id": "romaine_lettuce", "name": "romaine lettuce", "aliases": ["romaine"]},
    {"code": "75111000", "food_id": "cucumber_raw", "name": "cucumber", "aliases": []},
    {"code": "75117020", "food_id": "onion_raw", "name": "onion", "aliases": ["raw onion"]},
    {"code": "71508001", "food_id": "potato_baked", "name": "baked potato", "aliases": ["potato"]},
    {"code": "75215990", "food_id": "corn_cooked", "name": "corn", "aliases": ["cooked corn"]},
    {"code": "75205021", "food_id": "green_beans_cooked", "name": "green beans", "aliases": ["cooked green beans"]},
    {"code": "75224000", "food_id": "green_peas_cooked", "name": "green peas", "aliases": ["peas"]},
    {"code": "75214011", "food_id": "cauliflower_cooked", "name": "cauliflower", "aliases": ["cooked cauliflower"]},
    {"code": "75103000", "food_id": "cabbage_raw", "name": "cabbage", "aliases": ["raw cabbage"]},
    {"code": "75115000", "food_id": "mushrooms_raw", "name": "mushrooms", "aliases": ["raw mushrooms"]},
    {"code": "75233011", "food_id": "zucchini_cooked", "name": "zucchini", "aliases": ["summer squash"]},
    {"code": "75202011", "food_id": "asparagus_cooked", "name": "asparagus", "aliases": ["cooked asparagus"]},
    {"code": "72119211", "food_id": "kale_cooked", "name": "kale", "aliases": ["cooked kale"]},
    {"code": "75209011", "food_id": "brussels_sprouts_cooked", "name": "brussels sprouts", "aliases": []},
    {"code": "75111500", "food_id": "garlic_raw", "name": "garlic", "aliases": ["raw garlic"]},
    {"code": "75121000", "food_id": "hot_pepper_raw", "name": "hot pepper", "aliases": ["chili pepper"]},
    {"code": "82105500", "food_id": "canola_oil", "name": "canola oil", "aliases": []},
    {"code": "82107000", "food_id": "sesame_oil", "name": "sesame oil", "aliases": []},
    {"code": "81101000", "food_id": "butter", "name": "butter", "aliases": []},
    {"code": "83107000", "food_id": "mayonnaise", "name": "mayonnaise", "aliases": ["mayo"]},
    {"code": "83108000", "food_id": "vegan_mayonnaise", "name": "vegan mayonnaise", "aliases": ["vegan mayo"]},
    {"code": "83113500", "food_id": "ranch_dressing", "name": "ranch dressing", "aliases": ["ranch"]},
    {"code": "83102000", "food_id": "caesar_dressing", "name": "caesar dressing", "aliases": []},
    {"code": "83106000", "food_id": "italian_dressing", "name": "italian dressing", "aliases": []},
    {"code": "74401010", "food_id": "ketchup", "name": "ketchup", "aliases": []},
    {"code": "75506010", "food_id": "mustard", "name": "mustard", "aliases": []},
    {"code": "41420350", "food_id": "soy_sauce_reduced_sodium", "name": "reduced sodium soy sauce", "aliases": ["low sodium soy sauce"]},
    {"code": "74402100", "food_id": "salsa", "name": "salsa", "aliases": []},
    {"code": "75511010", "food_id": "hot_sauce", "name": "hot sauce", "aliases": []},
    {"code": "64401000", "food_id": "vinegar", "name": "vinegar", "aliases": []},
    {"code": "61204010", "food_id": "lemon_juice", "name": "lemon juice", "aliases": []},
    {"code": "58403010", "food_id": "chicken_noodle_soup", "name": "chicken noodle soup", "aliases": []},
    {"code": "74601000", "food_id": "tomato_soup", "name": "tomato soup", "aliases": []},
    {"code": "58106210", "food_id": "cheese_pizza", "name": "cheese pizza", "aliases": []},
    {"code": "58106514", "food_id": "pepperoni_pizza", "name": "pepperoni pizza", "aliases": []},
    {"code": "27510195", "food_id": "cheeseburger", "name": "cheeseburger", "aliases": []},
    {"code": "58102650", "food_id": "bean_burrito", "name": "bean burrito", "aliases": ["burrito"]},
    {"code": "27416300", "food_id": "beef_taco_filling", "name": "beef taco filling", "aliases": ["taco beef"]},
    {"code": "71401030", "food_id": "french_fries", "name": "french fries", "aliases": ["fries"]},
    {"code": "71200100", "food_id": "potato_chips", "name": "potato chips", "aliases": ["chips"]},
    {"code": "54408017", "food_id": "pretzels", "name": "pretzels", "aliases": []},
    {"code": "54403040", "food_id": "popcorn", "name": "popcorn", "aliases": []},
    {"code": "53713010", "food_id": "granola_bar", "name": "granola bar", "aliases": []},
    {"code": "53206000", "food_id": "chocolate_chip_cookie", "name": "chocolate chip cookie", "aliases": ["cookie"]},
    {"code": "53204010", "food_id": "brownie", "name": "brownie", "aliases": []},
    {"code": "53105275", "food_id": "chocolate_cake", "name": "chocolate cake", "aliases": ["cake"]},
    {"code": "13110100", "food_id": "vanilla_ice_cream", "name": "vanilla ice cream", "aliases": ["ice cream"]},
    {"code": "91705010", "food_id": "chocolate_candy", "name": "chocolate candy", "aliases": []},
    {"code": "91745010", "food_id": "gummy_candy", "name": "gummy candy", "aliases": ["fruit candy"]},
    {"code": "91300100", "food_id": "pancake_syrup", "name": "pancake syrup", "aliases": ["syrup"]},
    {"code": "91401000", "food_id": "jelly", "name": "jelly", "aliases": ["jam"]},
    {"code": "92410310", "food_id": "cola", "name": "cola", "aliases": ["soft drink cola", "soda"]},
    {"code": "61210000", "food_id": "orange_juice", "name": "orange juice", "aliases": []},
    {"code": "92101000", "food_id": "coffee_brewed", "name": "coffee", "aliases": ["black coffee"]},
    {"code": "92302000", "food_id": "tea_black_hot", "name": "black tea", "aliases": ["tea"]},
    {"code": "92308000", "food_id": "sweet_tea", "name": "sweet tea", "aliases": []},
    {"code": "94100100", "food_id": "bottled_water", "name": "water", "aliases": ["bottled water"]},
    {"code": "95320200", "food_id": "sports_drink", "name": "sports drink", "aliases": ["gatorade"]},
    {"code": "92510960", "food_id": "lemonade", "name": "lemonade", "aliases": []},
    {"code": "64134015", "food_id": "fruit_smoothie", "name": "fruit smoothie", "aliases": ["smoothie"]},
    {"code": "95201200", "food_id": "protein_powder", "name": "protein powder", "aliases": ["whey protein powder"]},
    {"code": "53720500", "food_id": "protein_bar", "name": "protein bar", "aliases": ["nutrition bar"]},
]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--source-dir",
        default="/tmp/mealcheck-fndds-2021-2023",
        help="Directory containing the three FNDDS 2021-2023 At A Glance XLSX workbooks.",
    )
    parser.add_argument("--catalog-out", default=str(CATALOG_PATH))
    parser.add_argument("--dataset-out", default=str(DATASET_PATH))
    args = parser.parse_args()

    source_dir = Path(args.source_dir)
    foods = records_from_xlsx(source_dir / "foods-and-beverages.xlsx")
    nutrients = records_from_xlsx(source_dir / "nutrient-values.xlsx")
    portions = records_from_xlsx(source_dir / "portions-and-weights.xlsx")

    foods_by_code = {row["Food code"]: row for row in foods}
    nutrients_by_code = {row["Food code"]: row for row in nutrients}
    portions_by_code: dict[str, list[dict[str, str]]] = {}
    for row in portions:
        portions_by_code.setdefault(row["Food code"], []).append(row)

    catalog = build_catalog(foods_by_code, nutrients_by_code, portions_by_code)
    dataset = build_dataset()

    write_json(Path(args.catalog_out), catalog)
    write_json(Path(args.dataset_out), dataset)
    print(f"wrote {args.catalog_out} with {len(catalog['foods'])} foods")
    print(f"wrote {args.dataset_out} with {len(dataset['cases'])} cases")


def build_catalog(
    foods_by_code: dict[str, dict[str, str]],
    nutrients_by_code: dict[str, dict[str, str]],
    portions_by_code: dict[str, list[dict[str, str]]],
) -> dict[str, Any]:
    catalog_foods = []
    seen_food_ids: set[str] = set()
    seen_labels: dict[str, str] = {}

    for selected in FOOD_SELECTION:
        code = selected["code"]
        if selected["food_id"] in seen_food_ids:
            raise ValueError(f"duplicate selected food_id {selected['food_id']}")
        seen_food_ids.add(selected["food_id"])
        if code not in foods_by_code:
            raise ValueError(f"FNDDS food code {code} not found")
        if code not in nutrients_by_code:
            raise ValueError(f"FNDDS nutrient code {code} not found")

        source_food = foods_by_code[code]
        nutrient_row = nutrients_by_code[code]
        aliases = aliases_for(selected, source_food, seen_labels)
        groups = food_groups(selected, source_food)
        invalid_groups = sorted(set(groups) - ALLOWED_GROUPS)
        if invalid_groups:
            raise ValueError(f"{selected['food_id']} has invalid groups {invalid_groups}")

        food = {
            "food_id": selected["food_id"],
            "name": selected["name"],
            "aliases": aliases,
            "base_quantity_g": 100,
            "nutrients_per_100g": nutrients_for(nutrient_row, selected, source_food),
            "unit_conversions": unit_conversions(portions_by_code.get(code, [])),
            "allergens": allergens_for(selected, source_food),
            "food_groups": groups,
            "source_refs": [
                {
                    "source": SOURCE_ID,
                    "source_id": code,
                    "data_type": "FNDDS At A Glance",
                    "note": f"{source_food['WWEIA Category number']} {source_food['WWEIA Category description']}",
                }
            ],
        }
        catalog_foods.append(food)

    return {
        "schema_version": "0.1",
        "catalog_id": "fixture-catalog-v1",
        "source": "usda_fndds_2021_2023_reviewed_subset",
        "fixture_note": (
            "Deterministic USDA FNDDS 2021-2023 reviewed subset for tests, demos, "
            "and evaluation-driven resolver coverage. FNDDS nutrient values are per "
            "100g; household-unit conversions come from FNDDS portions and weights "
            "where available. Added sugar uses a documented proxy because FNDDS At "
            "A Glance does not publish added-sugar grams."
        ),
        "foods": catalog_foods,
    }


def aliases_for(selected: dict[str, Any], source_food: dict[str, str], seen_labels: dict[str, str]) -> list[str]:
    labels = [selected["name"], *selected.get("aliases", []), source_food["Main food description"]]
    result = []
    for label in labels:
        clean = normalize_label(label)
        if not clean:
            continue
        key = label_key(clean)
        existing = seen_labels.get(key)
        if existing and existing != selected["food_id"]:
            if clean == source_food["Additional food description"]:
                continue
            raise ValueError(f"label {clean!r} is used by {existing} and {selected['food_id']}")
        seen_labels[key] = selected["food_id"]
        if key != label_key(selected["name"]) and clean not in result:
            result.append(clean)
    return result


def normalize_label(value: str) -> str:
    value = re.sub(r"\s+", " ", value.replace("\n", " ")).strip()
    return value.strip("; ")


def label_key(value: str) -> str:
    return " ".join(value.lower().split())


def nutrients_for(nutrient_row: dict[str, str], selected: dict[str, Any], source_food: dict[str, str]) -> dict[str, float]:
    total_sugar = number(nutrient_row["Sugars, total (g)"])
    return {
        "energy_kcal": round1(number(nutrient_row["Energy (kcal)"])),
        "protein_g": round1(number(nutrient_row["Protein (g)"])),
        "carbohydrate_g": round1(number(nutrient_row["Carbohydrate (g)"])),
        "fat_g": round1(number(nutrient_row["Total Fat (g)"])),
        "saturated_fat_g": round1(number(nutrient_row["Fatty acids, total saturated (g)"])),
        "sodium_mg": round1(number(nutrient_row["Sodium (mg)"])),
        "added_sugar_g": round1(added_sugar_proxy(total_sugar, selected, source_food)),
        "fiber_g": round1(number(nutrient_row["Fiber, total dietary (g)"])),
    }


def added_sugar_proxy(total_sugar: float, selected: dict[str, Any], source_food: dict[str, str]) -> float:
    text = " ".join(
        [
            selected["name"],
            source_food["Main food description"],
            source_food.get("Additional food description", ""),
            source_food["WWEIA Category description"],
        ]
    ).lower()
    natural_sugar_terms = [
        "apple",
        "banana",
        "orange",
        "strawberr",
        "blueberr",
        "grape",
        "watermelon",
        "pineapple",
        "cantaloupe",
        "clementine",
        "pear",
        "peach",
        "plain yogurt",
        "greek",
        "milk, reduced",
        "milk, whole",
        "milk, fat free",
    ]
    if any(term in text for term in natural_sugar_terms):
        return 0

    added_proxy_terms = [
        "soft drink",
        "soda",
        "cola",
        "lemonade",
        "sports drink",
        "sweet tea",
        "chocolate milk",
        "candy",
        "cookie",
        "brownie",
        "cake",
        "ice cream",
        "syrup",
        "jelly",
        "jam",
        "sweetened",
        "higher sugar",
        "granola bar",
        "chocolate",
        "fruit flavored drink",
    ]
    if any(term in text for term in added_proxy_terms):
        return total_sugar
    return 0


def unit_conversions(portions: list[dict[str, str]]) -> dict[str, float]:
    conversions = {"g": 1, "oz": 28.35}
    first_portion = 0.0
    for portion in portions:
        grams = number(portion.get("Portion weight (g)", "0"))
        if grams <= 0:
            continue
        desc = normalize_label(portion.get("Portion description", "")).lower()
        if first_portion == 0:
            first_portion = grams
        for unit, patterns in {
            "cup": [r"^1 cup$", r"^1 cup,"],
            "tbsp": [r"^1 tablespoon$", r"^1 tbsp$", r"^1 tablespoon,"],
            "tsp": [r"^1 teaspoon$", r"^1 tsp$", r"^1 teaspoon,"],
            "slice": [r"^1 slice$", r"^1 slice,"],
            "piece": [r"^1 piece$", r"^1 piece,"],
            "bar": [r"^1 bar$", r"^1 bar,"],
            "can": [r"^1 can$", r"^1 can,"],
            "bottle": [r"^1 bottle$", r"^1 bottle,"],
            "medium": [r"^1 medium$", r"^1 medium,"],
            "large": [r"^1 large$", r"^1 large,"],
            "small": [r"^1 small$", r"^1 small,"],
            "egg": [r"^1 egg$", r"^1 large egg$", r"^1 medium egg$"],
        }.items():
            if unit not in conversions and any(re.search(pattern, desc) for pattern in patterns):
                conversions[unit] = round2(grams)
    serving_units = ["cup", "piece", "slice", "medium", "egg", "bar", "can", "bottle", "large", "small", "tbsp", "tsp"]
    serving = next((conversions[unit] for unit in serving_units if unit in conversions), first_portion or 100)
    conversions["serving"] = round2(serving)
    return dict(sorted(conversions.items()))


def allergens_for(selected: dict[str, Any], source_food: dict[str, str]) -> list[str]:
    text = " ".join(
        [
            selected["name"],
            *selected.get("aliases", []),
            source_food["Main food description"],
            source_food.get("Additional food description", ""),
            source_food["WWEIA Category description"],
        ]
    ).lower()
    allergens = set()
    if any(term in text for term in ["cheese", "yogurt", "butter", "ice cream", "chocolate milk", "ranch", "caesar", "pizza", "cheeseburger", "macaroni and cheese", "lasagna"]):
        allergens.add("milk")
    if "milk" in text and not any(term in text for term in ["almond milk", "oat milk", "coconut milk", "plant-based milk", "soy milk"]):
        allergens.add("milk")
    if any(term in text for term in ["egg", "mayonnaise", "mayo"]) and "vegan" not in text:
        allergens.add("eggs")
    if any(term in text for term in ["fish", "salmon", "tuna", "tilapia", "cod"]):
        allergens.add("fish")
    if any(term in text for term in ["shrimp", "crab", "lobster", "crayfish"]):
        allergens.add("crustacean shellfish")
    if "peanut" in text:
        allergens.add("peanuts")
    if any(term in text for term in ["almond", "cashew", "walnut", "hazelnut", "macadamia"]):
        allergens.add("tree nuts")
    if any(term in text for term in ["bread", "bagel", "flour tortilla", "pasta", "spaghetti", "macaroni", "lasagna", "cereal", "granola", "pancake", "waffle", "muffin", "cookie", "brownie", "cake", "pizza", "burger", "hot dog", "ramen", "wheat"]):
        allergens.add("wheat")
    if any(term in text for term in ["soy", "tofu", "soybean", "shoyu", "tamari", "teriyaki"]):
        allergens.add("soybeans")
    if any(term in text for term in ["sesame", "hummus"]):
        allergens.add("sesame")
    order = ["milk", "eggs", "fish", "crustacean shellfish", "tree nuts", "peanuts", "wheat", "soybeans", "sesame"]
    return [allergen for allergen in order if allergen in allergens]


def food_groups(selected: dict[str, Any], source_food: dict[str, str]) -> list[str]:
    text = " ".join([selected["name"], source_food["Main food description"], source_food["WWEIA Category description"]]).lower()
    groups = set()
    if any(term in text for term in ["milk", "yogurt", "cheese", "ice cream"]):
        groups.add("dairy")
    if any(term in text for term in ["apple", "banana", "orange", "strawberr", "blueberr", "grape", "watermelon", "pineapple", "cantaloupe", "clementine", "pear", "peach", "fruit smoothie"]):
        groups.add("fruits")
    if any(term in text for term in ["vegetable", "broccoli", "spinach", "carrot", "tomato", "lettuce", "cucumber", "onion", "potato", "corn", "green bean", "pea", "cauliflower", "cabbage", "mushroom", "zucchini", "squash", "asparagus", "kale", "brussels", "garlic", "pepper", "avocado"]):
        groups.add("vegetables")
    if any(term in text for term in ["chicken", "beef", "pork", "turkey", "ham", "bacon", "sausage", "hot dog", "salmon", "tuna", "shrimp", "tilapia", "cod", "egg", "tofu", "bean", "chickpea", "lentil", "hummus", "peanut butter", "almond", "cashew", "walnut", "protein powder", "protein bar"]):
        groups.add("protein")
    if any(term in text for term in ["oatmeal", "brown rice", "quinoa", "whole wheat", "whole grain", "barley", "o cereal"]):
        groups.add("whole_grains")
    if any(term in text for term in ["white rice", "pasta", "white bread", "bagel", "flour tortilla", "corn tortilla", "corn flakes", "cereal", "pancake", "waffle", "muffin", "macaroni", "lasagna", "ramen", "pizza", "burrito", "taco"]):
        groups.add("refined_grains")
    if any(term in text for term in ["oil", "butter", "mayonnaise", "dressing"]):
        groups.add("fats")
    if any(term in text for term in ["sauce", "ketchup", "mustard", "salsa", "vinegar", "lemon juice", "hot sauce", "syrup", "jelly", "jam"]):
        groups.add("condiments")
    category = source_food["WWEIA Category description"].lower()
    if any(term in category for term in ["water", "coffee", "tea", "soft drinks", "fruit drinks", "citrus juice", "sport and energy drinks", "smoothies"]) or selected["name"] in {"water", "coffee", "black tea", "sweet tea", "cola", "orange juice", "sports drink", "lemonade", "fruit smoothie"}:
        groups.add("beverages")
    if any(term in text for term in ["french fries", "potato chips", "pretzel", "popcorn", "granola bar", "cookie", "brownie", "cake", "candy", "chocolate"]):
        groups.add("snacks")
    if not groups:
        groups.add("snacks")
    return [group for group in ["beverages", "condiments", "dairy", "fats", "fruits", "protein", "refined_grains", "snacks", "vegetables", "whole_grains"] if group in groups]


def build_dataset() -> dict[str, Any]:
    cases: list[dict[str, Any]] = []
    add_cases(cases, "balanced_common", 20, balanced_case)
    add_cases(cases, "vegetarian", 15, vegetarian_case)
    add_cases(cases, "vegan", 10, vegan_case)
    add_cases(cases, "high_sodium", 10, high_sodium_case)
    add_cases(cases, "high_added_sugar", 10, high_added_sugar_case)
    add_cases(cases, "allergen_risk", 10, allergen_case)
    add_cases(cases, "low_protein", 10, low_protein_case)
    add_cases(cases, "long_tail_unresolved", 8, unresolved_case)
    add_cases(cases, "vague_quantity", 7, vague_quantity_case)
    if len(cases) != 100:
        raise ValueError(f"expected 100 cases, got {len(cases)}")
    return {
        "schema_version": "0.1",
        "dataset_id": "fndds-grounded-meal-plans-v1",
        "description": (
            "One hundred evaluation meal-plan cases grounded in the USDA FNDDS 2021-2023 reviewed local catalog subset."
        ),
        "catalog_path": str(CATALOG_PATH),
        "source_refs": [
            {
                "source": SOURCE_ID,
                "data_type": "FNDDS At A Glance",
                "note": "Food descriptions, nutrients per 100g, and household portion weights.",
            }
        ],
        "cases": cases,
    }


def add_cases(cases: list[dict[str, Any]], category: str, count: int, builder: Any) -> None:
    for offset in range(count):
        number = len(cases) + 1
        cases.append(builder(number, offset))


def base_settings(
    *,
    allergies: list[str] | None = None,
    protein_target: int = 50,
    sodium_limit: int = 2300,
    added_sugar_limit: float = 10,
) -> dict[str, Any]:
    return {
        "nutrition_targets": {"calorie_target_kcal": 2000, "protein_target_g": protein_target},
        "verification_constraints": {
            "days": 1,
            "meals_per_day": 3,
            "allergies": allergies or [],
            "excluded_foods": [],
            "max_sodium_mg_per_day": sodium_limit,
            "max_added_sugar_g_per_meal": added_sugar_limit,
            "max_saturated_fat_pct_calories": 10,
            "calorie_tolerance_pct": 40,
            "requires_prep_safety_notes": False,
        },
    }


def balanced_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["cooked oatmeal", "blueberries", "plain Greek yogurt"], ["chicken breast", "brown rice", "broccoli"], ["salmon", "sweet potato", "spinach"]],
        [["boiled egg", "whole wheat bread", "apple"], ["turkey breast", "quinoa", "green beans"], ["tilapia", "white rice", "cauliflower"]],
        [["o cereal", "2% milk", "banana"], ["canned tuna", "whole wheat bagel", "carrots"], ["chicken thigh", "barley", "kale"]],
        [["pancakes", "strawberries", "skim milk"], ["ground beef", "corn tortilla", "romaine lettuce"], ["cod", "brown rice", "asparagus"]],
    ]
    return standard_case(number, "balanced_common", rotations[offset % len(rotations)])


def vegetarian_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["plain yogurt", "granola", "peach"], ["black beans", "brown rice", "corn"], ["scrambled egg", "whole wheat bread", "spinach"]],
        [["cottage cheese", "strawberries", "oatmeal"], ["chickpeas", "quinoa", "cucumber"], ["lentils", "sweet potato", "kale"]],
        [["cheddar cheese", "whole wheat bread", "tomato"], ["hummus", "flour tortilla", "romaine lettuce"], ["refried beans", "corn tortilla", "green peas"]],
    ]
    return standard_case(number, "vegetarian", rotations[offset % len(rotations)])


def vegan_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["oat milk", "cooked oatmeal", "blueberries"], ["tofu", "brown rice", "broccoli"], ["lentils", "sweet potato", "spinach"]],
        [["unsweetened almond milk", "granola", "banana"], ["chickpeas", "quinoa", "cabbage"], ["black beans", "corn tortilla", "salsa"]],
        [["fruit smoothie", "protein powder", "water"], ["hummus", "whole wheat bread", "cucumber"], ["pinto beans", "barley", "zucchini"]],
    ]
    return standard_case(number, "vegan", rotations[offset % len(rotations)])


def high_sodium_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["ramen noodles", "soy sauce", "boiled egg"], ["chicken noodle soup", "pretzels", "carrots"], ["pepperoni pizza", "ranch dressing", "romaine lettuce"]],
        [["beef hot dog", "ketchup", "mustard"], ["tomato soup", "potato chips", "cucumber"], ["macaroni and cheese", "bacon", "broccoli"]],
    ]
    case = standard_case(number, "high_sodium", rotations[offset % len(rotations)], settings=base_settings(sodium_limit=1200))
    case["expected"].update({"decision": "warn", "warn_checks": ["sodium_under_limit"]})
    return case


def high_added_sugar_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["sweetened cereal", "chocolate milk", "banana"], ["cheeseburger", "cola", "french fries"], ["chocolate cake", "vanilla ice cream", "strawberries"]],
        [["pancakes", "pancake syrup", "orange juice"], ["granola bar", "sports drink", "apple"], ["chocolate chip cookie", "sweet tea", "plain yogurt"]],
    ]
    case = standard_case(number, "high_added_sugar", rotations[offset % len(rotations)], settings=base_settings(added_sugar_limit=8))
    case["expected"].update({"decision": "warn", "warn_checks": ["added_sugar_under_limit"]})
    return case


def allergen_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["cooked oatmeal", "peanut butter", "banana"], ["chicken breast", "brown rice", "broccoli"], ["salmon", "sweet potato", "spinach"]],
        [["plain Greek yogurt", "blueberries", "whole wheat bread"], ["tofu", "quinoa", "kale"], ["shrimp", "white rice", "green beans"]],
    ]
    allergies = ["peanuts"] if offset % 2 == 0 else ["milk", "crustacean shellfish"]
    case = standard_case(number, "allergen_risk", rotations[offset % len(rotations)], settings=base_settings(allergies=allergies))
    case["expected"].update({"decision": "block", "block_checks": ["allergens_absent"]})
    return case


def low_protein_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["water", "apple", "white bread"], ["white rice", "cucumber", "lettuce"], ["pasta", "tomato", "olive oil"]],
        [["black tea", "banana", "corn flakes"], ["baked potato", "salsa", "cabbage"], ["couscous", "zucchini", "lemon juice"]],
    ]
    case = standard_case(number, "low_protein", rotations[offset % len(rotations)], settings=base_settings(protein_target=85))
    case["expected"].update({"decision": "warn", "warn_checks": ["protein_minimum_met"]})
    return case


def unresolved_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["cooked oatmeal", "blueberries", "moringa smoothie powder"], ["chicken breast", "brown rice", "broccoli"], ["salmon", "sweet potato", "spinach"]],
        [["boiled egg", "apple", "spirulina noodles"], ["black beans", "quinoa", "carrots"], ["tofu", "white rice", "kale"]],
    ]
    case = standard_case(number, "long_tail_unresolved", rotations[offset % len(rotations)])
    unknown = case["plan"]["days"][0]["meals"][0]["items"][2]
    unknown["resolution_status"] = "unresolved"
    unknown["unresolved_reason"] = "unknown_food"
    case["expected"].update({"decision": "block", "unresolved_count": 1, "block_checks": ["quantities_resolvable"]})
    return case


def vague_quantity_case(number: int, offset: int) -> dict[str, Any]:
    rotations = [
        [["cooked oatmeal", "blueberries", "plain Greek yogurt"], ["chicken breast", "brown rice", "broccoli"], ["salmon", "some spinach", "sweet potato"]],
        [["boiled egg", "apple", "whole wheat bread"], ["black beans", "quinoa", "a handful of kale"], ["tofu", "white rice", "carrots"]],
    ]
    case = standard_case(number, "vague_quantity", rotations[offset % len(rotations)])
    for meal in case["plan"]["days"][0]["meals"]:
        for item in meal["items"]:
            if item["food"].startswith(("some ", "a handful of ")):
                item["food"] = item["food"].replace("some ", "").replace("a handful of ", "")
                item.pop("quantity", None)
                item["quantity_text"] = "some" if item["food"] == "spinach" else "a handful"
                item["unit"] = ""
                item["resolution_status"] = "unresolved"
                item["unresolved_reason"] = "vague_quantity"
    case["expected"].update({"decision": "block", "unresolved_count": 1, "block_checks": ["quantities_resolvable"]})
    return case


def standard_case(
    number: int,
    category: str,
    meal_foods: list[list[str]],
    *,
    settings: dict[str, Any] | None = None,
) -> dict[str, Any]:
    case_id = f"{category}-{number:03d}"
    meal_names = ["Breakfast", "Lunch", "Dinner"]
    meals = []
    for meal_name, foods in zip(meal_names, meal_foods):
        meals.append({"name": meal_name, "items": [food_item(food) for food in foods]})
    source_text = "; ".join(f"{meal['name']}: {', '.join(item['food'] for item in meal['items'])}" for meal in meals)
    return {
        "case_id": case_id,
        "category": category,
        "description": f"{category.replace('_', ' ')} FNDDS-grounded meal-plan case {number}",
        "source_text": source_text,
        "settings": settings or base_settings(),
        "plan": {
            "schema_version": "0.1",
            "plan_id": f"{case_id}-plan",
            "description": f"{category.replace('_', ' ')} FNDDS-grounded meal plan",
            "days": [{"day": 1, "meals": meals}],
            "shopping_list": [],
            "prep_notes": ["Refrigerate leftovers within 2 hours."],
        },
        "expected": {"unresolved_count": 0, "allow_extra_warnings": True},
        "tags": ["fndds", "catalog-expansion", category],
    }


def food_item(food: str) -> dict[str, Any]:
    tbsp_foods = {
        "olive oil",
        "canola oil",
        "sesame oil",
        "butter",
        "mayonnaise",
        "vegan mayonnaise",
        "ranch dressing",
        "caesar dressing",
        "italian dressing",
        "peanut sauce",
        "soy sauce",
        "reduced sodium soy sauce",
        "ketchup",
        "mustard",
        "salsa",
        "hot sauce",
        "vinegar",
        "pancake syrup",
        "jelly",
    }
    tsp_foods = {"hot sauce"}
    if food in tsp_foods:
        unit = "tsp"
    else:
        unit = "tbsp" if food in tbsp_foods else "serving"
    return {"food": food, "quantity": 1, "unit": unit}


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
                value = cell_value(cell, shared)
                values[idx] = value
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


def normalize_header(value: str) -> str:
    return re.sub(r"\s+", " ", value.replace("\n", " ")).strip()


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


if __name__ == "__main__":
    main()
