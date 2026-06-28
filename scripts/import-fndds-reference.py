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

MATCH_STATUS_AUTO = "auto"
MATCH_STATUS_REVIEW = "review"
MATCH_STATUS_DECOMPOSE = "decompose"
MATCH_STATUS_BLOCKED = "blocked"

MATCH_CONFIDENCE_EXACT = "exact"
MATCH_CONFIDENCE_HIGH = "high"
MATCH_CONFIDENCE_REVIEW = "review"

VALID_STATUSES = {
    STATUS_ELIGIBLE_SPECIFIC,
    STATUS_ELIGIBLE_GENERIC,
    STATUS_REVIEW_REQUIRED,
    STATUS_QUARANTINED_AMBIGUOUS,
    STATUS_QUARANTINED_MIXED_DISH,
    STATUS_QUARANTINED_RESTAURANT_OR_BRAND,
    STATUS_QUARANTINED_PREPARATION_UNCLEAR,
}

VALID_MATCH_STATUSES = {
    MATCH_STATUS_AUTO,
    MATCH_STATUS_REVIEW,
    MATCH_STATUS_DECOMPOSE,
    MATCH_STATUS_BLOCKED,
}

VALID_MATCH_CONFIDENCES = {
    MATCH_CONFIDENCE_EXACT,
    MATCH_CONFIDENCE_HIGH,
    MATCH_CONFIDENCE_REVIEW,
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
    "56205001": "Rice, white, cooked, NS as to fat",
    "56205011": "Rice, brown, cooked, NS as to fat",
    "61210000": "Orange juice, 100%",
    "64104010": "Apple juice, 100%",
    "92101000": "Coffee, brewed",
    "92302000": "Tea, black, brewed",
    "94000100": "Water, tap",
    "94100100": "Water, bottled",
}

MATCH_KEY_AUTO_ALLOWLIST_CODES = {
    "51101010": "Bread, white, toasted",
    "51150000": "Roll, white, soft",
    "54325000": "Crackers, saltine",
    "91101010": "Sugar, white, granulated or lump",
    "92103000": "Coffee, instant, reconstituted",
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

AUTO_BLOCK_FLAGS = {
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
}

BENIGN_ALIAS_QUALIFIERS = {
    "no added fat",
    "reconstituted",
    "for use on a sandwich",
}

SANDWICH_COMPONENT_BASES = {
    "avocado",
    "bacon",
    "cucumber",
    "lettuce",
    "mushrooms",
    "onions",
    "pepper",
    "spinach",
    "tomatoes",
}

APPROXIMATION_PROXIES = [
    {
        "input_key": "water",
        "proxy_food_code": "94000100",
        "proxy_description": "Water, tap",
        "confidence": "medium",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad water proxy using tap water.",
        "source_food_codes": ["94000100"],
    },
    {
        "input_key": "tea",
        "proxy_food_code": "92302000",
        "proxy_description": "Tea, black, brewed",
        "confidence": "medium",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad tea proxy using brewed black tea.",
        "source_food_codes": ["92302000"],
    },
    {
        "input_key": "coffee",
        "proxy_food_code": "92101000",
        "proxy_description": "Coffee, brewed",
        "confidence": "medium",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad coffee proxy using brewed coffee.",
        "source_food_codes": ["92101000"],
    },
    {
        "input_key": "rice",
        "proxy_food_code": "56205001",
        "proxy_description": "Rice, white, cooked, NS as to fat",
        "confidence": "medium",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad rice proxy using cooked white rice.",
        "source_food_codes": ["56205001", "56205007", "56205011"],
    },
    {
        "input_key": "pasta",
        "proxy_food_code": "56130000",
        "proxy_description": "Pasta, cooked",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad pasta proxy using cooked pasta.",
        "source_food_codes": [],
    },
    {
        "input_key": "bread",
        "proxy_food_code": "51101000",
        "proxy_description": "Bread, white",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Broad bread proxy using white bread.",
        "source_food_codes": [],
    },
    {
        "input_key": "cheese",
        "proxy_food_code": "14010000",
        "proxy_description": "Cheese, NFS",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic cheese approximation using the FNDDS not-further-specified cheese row.",
        "source_food_codes": ["14010000"],
    },
    {
        "input_key": "butter",
        "proxy_food_code": "81100500",
        "proxy_description": "Butter, NFS",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic butter approximation using the FNDDS not-further-specified butter row.",
        "source_food_codes": ["81100500"],
    },
    {
        "input_key": "pickles",
        "proxy_food_code": "75511100",
        "proxy_description": "Pickles, NFS",
        "confidence": "low",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic pickle approximation using the FNDDS not-further-specified pickle row.",
        "source_food_codes": ["75511100"],
    },
    {
        "input_key": "home fries",
        "proxy_food_code": "71403020",
        "proxy_description": "Potato, home fries, NFS",
        "confidence": "low",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic home-fries approximation using the FNDDS not-further-specified home-fries row.",
        "source_food_codes": ["71403020"],
    },
    {
        "input_key": "pretzels",
        "proxy_food_code": "54408000",
        "proxy_description": "Pretzels, NFS",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic pretzel approximation using the FNDDS not-further-specified pretzel row.",
        "source_food_codes": ["54408000"],
    },
    {
        "input_key": "cream",
        "proxy_food_code": "12100100",
        "proxy_description": "Cream, NS as to light, heavy, or half and half",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic cream approximation using the FNDDS not-specified cream row.",
        "source_food_codes": ["12100100"],
    },
    {
        "input_key": "hot chocolate",
        "proxy_food_code": "11512005",
        "proxy_description": "Hot chocolate / cocoa, NFS",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic hot-chocolate approximation using the FNDDS not-further-specified cocoa row.",
        "source_food_codes": ["11512005"],
    },
    {
        "input_key": "fruit",
        "proxy_food_code": "63100100",
        "proxy_description": "Fruit, NFS",
        "confidence": "low",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic fruit approximation using the FNDDS not-further-specified fruit row.",
        "source_food_codes": ["63100100"],
    },
    {
        "input_key": "nutritional drink",
        "proxy_food_code": "95120010",
        "proxy_description": "Nutritional drink or shake, high protein, ready-to-drink, NFS",
        "confidence": "low",
        "allow_when_allergies_present": False,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic nutritional-drink approximation using the FNDDS not-further-specified ready-to-drink row.",
        "source_food_codes": ["95120010"],
    },
    {
        "input_key": "chicken",
        "proxy_food_code": "24100020",
        "proxy_description": "Chicken, NS as to part and cooking method, skin not eaten",
        "confidence": "low",
        "allow_when_allergies_present": True,
        "allow_when_exclusions_present": False,
        "estimate_reason": "Generic chicken approximation using the FNDDS not-specified part and cooking-method row.",
        "source_food_codes": ["24100020"],
    },
]

DECOMPOSITION_TEMPLATES = [
    {
        "template_id": "ham_sandwich_white_cheese_v1",
        "pattern": "ham sandwich on white, with cheese",
        "confidence": "medium",
        "notes": "Curated sandwich split from total grams.",
        "components": [
            {"food_code": "51101000", "role": "bread, white", "fraction": 0.40, "required": True},
            {"food_code": "25230210", "role": "ham, deli", "fraction": 0.35, "required": True},
            {"food_code": "14410110", "role": "cheese, american", "fraction": 0.25, "required": True},
        ],
    },
    {
        "template_id": "ham_sandwich_wheat_cheese_v1",
        "pattern": "ham sandwich on wheat, with cheese",
        "confidence": "medium",
        "notes": "Curated sandwich split from total grams.",
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.40, "required": True},
            {"food_code": "25230210", "role": "ham, deli", "fraction": 0.35, "required": True},
            {"food_code": "14410110", "role": "cheese, american", "fraction": 0.25, "required": True},
        ],
    },
    {
        "template_id": "turkey_sandwich_wheat_cheese_v1",
        "pattern": "turkey sandwich on wheat, with cheese",
        "confidence": "medium",
        "notes": "Curated sandwich split from total grams.",
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.40, "required": True},
            {"food_code": "25230780", "role": "turkey, deli", "fraction": 0.35, "required": True},
            {"food_code": "14410110", "role": "cheese, american", "fraction": 0.25, "required": True},
        ],
    },
    {
        "template_id": "peanut_butter_sandwich_wheat_v1",
        "pattern": "peanut butter sandwich, with regular peanut butter, on wheat bread",
        "confidence": "medium",
        "notes": "Curated sandwich split from total grams.",
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.65, "required": True},
            {"food_code": "42202000", "role": "peanut butter", "fraction": 0.35, "required": True},
        ],
    },
    {
        "template_id": "pbj_sandwich_wheat_v1",
        "pattern": "peanut butter and jelly sandwich, with regular peanut butter, regular jelly, on wheat bread",
        "confidence": "medium",
        "notes": "Curated sandwich split from total grams.",
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.55, "required": True},
            {"food_code": "42202000", "role": "peanut butter", "fraction": 0.30, "required": True},
            {"food_code": "91401000", "role": "jelly", "fraction": 0.15, "required": True},
        ],
    },
    {
        "template_id": "taco_corn_beef_cheese_v1",
        "pattern": "taco, corn tortilla, beef, cheese",
        "confidence": "medium",
        "notes": "Curated taco split from total grams.",
        "components": [
            {"food_code": "52215100", "role": "corn tortilla", "fraction": 0.35, "required": True},
            {"food_code": "21500100", "role": "ground beef", "fraction": 0.45, "required": True},
            {"food_code": "14410110", "role": "cheese, american", "fraction": 0.20, "required": True},
        ],
    },
]

DECOMPOSITION_RULES = [
    {
        "rule_id": "pasta_tomato_meat_v1",
        "family": "pasta_tomato_meat",
        "priority": 100,
        "confidence": "low",
        "notes": "Low-confidence family split for pasta with tomato-based meat sauce; source-code bindings are preferred over text matching.",
        "source_food_codes": ["58146322", "58146323"],
        "match_terms": ["pasta", "sauce", "meat"],
        "exclude_terms": ["cream", "alfredo", "poultry", "chicken", "seafood"],
        "components": [
            {"food_code": "56130000", "role": "pasta, cooked", "fraction": 0.58, "required": True},
            {"food_code": "74404010", "role": "spaghetti sauce", "fraction": 0.32, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.10, "required": True},
        ],
    },
    {
        "rule_id": "pasta_cream_poultry_v1",
        "family": "pasta_cream_poultry",
        "priority": 110,
        "confidence": "low",
        "notes": "Low-confidence family split for pasta with cream sauce and poultry.",
        "source_food_codes": ["58146422"],
        "match_terms": ["pasta", "cream", "poultry"],
        "exclude_terms": ["tomato", "seafood"],
        "components": [
            {"food_code": "56130000", "role": "pasta, cooked", "fraction": 0.55, "required": True},
            {"food_code": "14650160", "role": "alfredo sauce", "fraction": 0.30, "required": True},
            {"food_code": "24122130", "role": "chicken breast, cooked", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "macaroni_cheese_v1",
        "family": "macaroni_cheese",
        "priority": 120,
        "confidence": "low",
        "notes": "Low-confidence family split for macaroni or noodles with cheese.",
        "source_food_codes": ["58145117"],
        "match_terms": ["macaroni", "cheese"],
        "exclude_terms": ["meat", "tuna", "chicken", "turkey", "frankfurter", "hot dog"],
        "components": [
            {"food_code": "56130000", "role": "pasta, cooked", "fraction": 0.70, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
            {"food_code": "11112110", "role": "milk, reduced fat", "fraction": 0.10, "required": True},
        ],
    },
    {
        "rule_id": "macaroni_cheese_meat_v1",
        "family": "macaroni_cheese_meat",
        "priority": 125,
        "confidence": "low",
        "notes": "Low-confidence family split for macaroni or noodles with cheese and meat.",
        "source_food_codes": ["58145135", "58145136"],
        "match_terms": ["macaroni", "cheese", "meat"],
        "exclude_terms": ["tuna", "chicken", "turkey", "frankfurter", "hot dog"],
        "components": [
            {"food_code": "56130000", "role": "pasta, cooked", "fraction": 0.55, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
            {"food_code": "11112110", "role": "milk, reduced fat", "fraction": 0.10, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "egg_cheese_no_fat_v1",
        "family": "egg_cheese",
        "priority": 130,
        "confidence": "low",
        "notes": "Low-confidence family split for omelet or scrambled egg with cheese and no added fat.",
        "source_food_codes": ["32130170"],
        "match_terms": ["egg", "cheese"],
        "exclude_terms": ["oil", "butter", "meat", "potato"],
        "components": [
            {"food_code": "31102000", "role": "egg, whole cooked", "fraction": 0.75, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.25, "required": True},
        ],
    },
    {
        "rule_id": "egg_cheese_oil_v1",
        "family": "egg_cheese",
        "priority": 131,
        "confidence": "low",
        "notes": "Low-confidence family split for omelet or scrambled egg with cheese made with oil.",
        "source_food_codes": ["32130110"],
        "match_terms": ["egg", "cheese", "oil"],
        "exclude_terms": ["butter", "meat", "potato"],
        "components": [
            {"food_code": "31105030", "role": "egg, whole fried with oil", "fraction": 0.75, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
            {"food_code": "82104000", "role": "olive oil", "fraction": 0.05, "required": True},
        ],
    },
    {
        "rule_id": "mashed_potato_gravy_v1",
        "family": "potato_gravy",
        "priority": 140,
        "confidence": "low",
        "notes": "Low-confidence family split for mashed potato with gravy.",
        "source_food_codes": ["71501012"],
        "match_terms": ["potato", "mashed", "gravy"],
        "exclude_terms": ["cheese"],
        "components": [
            {"food_code": "71501010", "role": "mashed potato with milk", "fraction": 0.80, "required": True},
            {"food_code": "28520010", "role": "gravy", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "french_fries_cheese_v1",
        "family": "fries_cheese",
        "priority": 145,
        "confidence": "low",
        "notes": "Low-confidence family split for french fries with cheese.",
        "source_food_codes": ["71402500"],
        "match_terms": ["fries", "cheese"],
        "exclude_terms": ["chili"],
        "components": [
            {"food_code": "71401010", "role": "french fries", "fraction": 0.80, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "ham_sandwich_white_v1",
        "family": "sandwich_ham",
        "priority": 150,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27520210"],
        "match_terms": ["ham", "sandwich", "white"],
        "exclude_terms": ["cheese", "salad", "wheat"],
        "components": [
            {"food_code": "51101000", "role": "bread, white", "fraction": 0.55, "required": True},
            {"food_code": "25230210", "role": "ham, deli", "fraction": 0.45, "required": True},
        ],
    },
    {
        "rule_id": "chicken_salad_sandwich_wheat_v1",
        "family": "sandwich_chicken_salad",
        "priority": 155,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27540121"],
        "match_terms": ["chicken salad", "sandwich", "wheat"],
        "exclude_terms": ["wrap", "white"],
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.50, "required": True},
            {"food_code": "25240110", "role": "chicken salad spread", "fraction": 0.50, "required": True},
        ],
    },
    {
        "rule_id": "tuna_salad_sandwich_white_v1",
        "family": "sandwich_tuna_salad",
        "priority": 156,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27550720"],
        "match_terms": ["tuna salad", "sandwich", "white"],
        "exclude_terms": ["cheese", "wheat", "wrap"],
        "components": [
            {"food_code": "51101000", "role": "bread, white", "fraction": 0.50, "required": True},
            {"food_code": "27450060", "role": "tuna salad", "fraction": 0.50, "required": True},
        ],
    },
    {
        "rule_id": "tuna_salad_sandwich_white_cheese_v1",
        "family": "sandwich_tuna_salad",
        "priority": 157,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27550730"],
        "match_terms": ["tuna salad", "sandwich", "white", "cheese"],
        "exclude_terms": ["wheat", "wrap"],
        "components": [
            {"food_code": "51101000", "role": "bread, white", "fraction": 0.45, "required": True},
            {"food_code": "27450060", "role": "tuna salad", "fraction": 0.40, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "tuna_salad_sandwich_wheat_v1",
        "family": "sandwich_tuna_salad",
        "priority": 158,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27550735"],
        "match_terms": ["tuna salad", "sandwich", "wheat"],
        "exclude_terms": ["cheese", "white", "wrap"],
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.50, "required": True},
            {"food_code": "27450060", "role": "tuna salad", "fraction": 0.50, "required": True},
        ],
    },
    {
        "rule_id": "tuna_salad_sandwich_wheat_cheese_v1",
        "family": "sandwich_tuna_salad",
        "priority": 159,
        "confidence": "medium",
        "notes": "Curated sandwich family split from total grams.",
        "source_food_codes": ["27550737"],
        "match_terms": ["tuna salad", "sandwich", "wheat", "cheese"],
        "exclude_terms": ["white", "wrap"],
        "components": [
            {"food_code": "51300110", "role": "bread, whole wheat", "fraction": 0.45, "required": True},
            {"food_code": "27450060", "role": "tuna salad", "fraction": 0.40, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "barbecue_beef_sandwich_v1",
        "family": "sandwich_barbecue_beef",
        "priority": 160,
        "confidence": "low",
        "notes": "Low-confidence family split for barbecue beef sandwich on a white bun.",
        "source_food_codes": ["27510130"],
        "match_terms": ["barbecue", "beef", "sandwich"],
        "exclude_terms": ["pork", "chicken", "wheat"],
        "components": [
            {"food_code": "51150000", "role": "white roll", "fraction": 0.45, "required": True},
            {"food_code": "21401000", "role": "beef roast", "fraction": 0.45, "required": True},
            {"food_code": "74406010", "role": "barbecue sauce", "fraction": 0.10, "required": True},
        ],
    },
    {
        "rule_id": "barbecue_pork_sauce_v1",
        "family": "barbecue_pork",
        "priority": 165,
        "confidence": "low",
        "notes": "Low-confidence family split for barbecue pork with sauce.",
        "source_food_codes": ["27120030"],
        "match_terms": ["barbecue", "pork", "sauce"],
        "exclude_terms": ["sandwich", "bun", "bread"],
        "components": [
            {"food_code": "22400100", "role": "pork roast", "fraction": 0.85, "required": True},
            {"food_code": "74406010", "role": "barbecue sauce", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "chili_meat_beans_v1",
        "family": "chili",
        "priority": 170,
        "confidence": "low",
        "notes": "Low-confidence family split for chili with meat and beans.",
        "source_food_codes": ["27111406"],
        "match_terms": ["chili", "meat", "beans"],
        "exclude_terms": ["vegetarian", "macaroni", "hot dog"],
        "components": [
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.35, "required": True},
            {"food_code": "41106020", "role": "kidney beans", "fraction": 0.35, "required": True},
            {"food_code": "74404010", "role": "spaghetti sauce", "fraction": 0.20, "required": True},
            {"food_code": "75221011", "role": "onions, cooked", "fraction": 0.10, "required": True},
        ],
    },
    {
        "rule_id": "chili_meat_canned_v1",
        "family": "chili",
        "priority": 171,
        "confidence": "low",
        "notes": "Low-confidence family split for canned chili with meat.",
        "source_food_codes": ["27111407"],
        "match_terms": ["chili", "meat", "canned"],
        "exclude_terms": ["vegetarian", "macaroni", "hot dog"],
        "components": [
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.45, "required": True},
            {"food_code": "74404010", "role": "spaghetti sauce", "fraction": 0.40, "required": True},
            {"food_code": "41106020", "role": "kidney beans", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "lentil_soup_v1",
        "family": "soup_lentil",
        "priority": 180,
        "confidence": "low",
        "notes": "Low-confidence soup split; only lentil soup is covered because the main legume component is explicit.",
        "source_food_codes": ["41603010"],
        "match_terms": ["lentil", "soup"],
        "exclude_terms": ["meat"],
        "components": [
            {"food_code": "41305000", "role": "lentils", "fraction": 0.40, "required": True},
            {"food_code": "28340120", "role": "broth", "fraction": 0.60, "required": True},
        ],
    },
    {
        "rule_id": "vegetable_soup_v1",
        "family": "soup_vegetable",
        "priority": 181,
        "confidence": "low",
        "notes": "Low-confidence soup split for plain vegetable soup where vegetables and broth are explicit.",
        "source_food_codes": ["75649010"],
        "match_terms": ["vegetable", "soup"],
        "exclude_terms": ["beef", "chicken", "cream", "cheese", "meat"],
        "components": [
            {"food_code": "75311020", "role": "mixed vegetables", "fraction": 0.35, "required": True},
            {"food_code": "28340120", "role": "broth", "fraction": 0.65, "required": True},
        ],
    },
    {
        "rule_id": "beef_soup_v1",
        "family": "soup_beef",
        "priority": 182,
        "confidence": "low",
        "notes": "Low-confidence soup split for beef soup; generic noodle, pho, or restaurant soup variants remain unresolved.",
        "source_food_codes": ["75652010"],
        "match_terms": ["beef", "soup"],
        "exclude_terms": ["pho", "noodle", "cream", "restaurant"],
        "components": [
            {"food_code": "21401000", "role": "beef roast", "fraction": 0.20, "required": True},
            {"food_code": "75311020", "role": "mixed vegetables", "fraction": 0.20, "required": True},
            {"food_code": "28340120", "role": "broth", "fraction": 0.60, "required": True},
        ],
    },
    {
        "rule_id": "beef_stew_pasta_v1",
        "family": "stew_beef_pasta",
        "priority": 190,
        "confidence": "low",
        "notes": "Low-confidence stew split for beef stew with pasta.",
        "source_food_codes": ["27311430"],
        "match_terms": ["stew", "beef", "pasta"],
        "exclude_terms": ["pork", "chicken"],
        "components": [
            {"food_code": "21401000", "role": "beef roast", "fraction": 0.25, "required": True},
            {"food_code": "56130000", "role": "pasta, cooked", "fraction": 0.30, "required": True},
            {"food_code": "75317020", "role": "stew vegetables", "fraction": 0.25, "required": True},
            {"food_code": "13411000", "role": "gravy or white sauce", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beef_cheese_v1",
        "family": "burrito",
        "priority": 191,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beef and cheese burritos.",
        "source_food_codes": ["58102310"],
        "match_terms": ["burrito", "beef", "cheese"],
        "exclude_terms": ["beans", "rice", "pork", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.35, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.40, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.25, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beef_rice_cheese_v1",
        "family": "burrito",
        "priority": 192,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beef, rice, and cheese burritos.",
        "source_food_codes": ["58102320"],
        "match_terms": ["burrito", "beef", "rice", "cheese"],
        "exclude_terms": ["beans", "pork", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.30, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.25, "required": True},
            {"food_code": "56205001", "role": "white rice", "fraction": 0.25, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beef_beans_cheese_v1",
        "family": "burrito",
        "priority": 193,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beef, beans, and cheese burritos.",
        "source_food_codes": ["58102330"],
        "match_terms": ["burrito", "beef", "beans", "cheese"],
        "exclude_terms": ["rice", "pork", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.30, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.25, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.25, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beef_beans_rice_cheese_v1",
        "family": "burrito",
        "priority": 194,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beef, beans, rice, and cheese burritos.",
        "source_food_codes": ["58102340"],
        "match_terms": ["burrito", "beef", "beans", "rice", "cheese"],
        "exclude_terms": ["pork", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.25, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.20, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.20, "required": True},
            {"food_code": "56205001", "role": "white rice", "fraction": 0.20, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "burrito_pork_beans_rice_cheese_v1",
        "family": "burrito",
        "priority": 195,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit pork, beans, rice, and cheese burritos.",
        "source_food_codes": ["58102440"],
        "match_terms": ["burrito", "pork", "beans", "rice", "cheese"],
        "exclude_terms": ["beef", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.25, "required": True},
            {"food_code": "22400100", "role": "pork roast", "fraction": 0.20, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.20, "required": True},
            {"food_code": "56205001", "role": "white rice", "fraction": 0.20, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "burrito_chicken_beans_rice_cheese_v1",
        "family": "burrito",
        "priority": 196,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit chicken, beans, rice, and cheese burritos.",
        "source_food_codes": ["58102540"],
        "match_terms": ["burrito", "chicken", "beans", "rice", "cheese"],
        "exclude_terms": ["beef", "pork"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.25, "required": True},
            {"food_code": "24122130", "role": "chicken breast, cooked", "fraction": 0.20, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.20, "required": True},
            {"food_code": "56205001", "role": "white rice", "fraction": 0.20, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.15, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beans_cheese_v1",
        "family": "burrito",
        "priority": 197,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beans and cheese burritos.",
        "source_food_codes": ["58102610"],
        "match_terms": ["burrito", "beans", "cheese"],
        "exclude_terms": ["beef", "pork", "chicken", "rice"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.30, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.45, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.25, "required": True},
        ],
    },
    {
        "rule_id": "burrito_beans_rice_cheese_v1",
        "family": "burrito",
        "priority": 198,
        "confidence": "low",
        "notes": "Low-confidence burrito split for explicit beans, rice, and cheese burritos.",
        "source_food_codes": ["58102620"],
        "match_terms": ["burrito", "beans", "rice", "cheese"],
        "exclude_terms": ["beef", "pork", "chicken"],
        "components": [
            {"food_code": "52215200", "role": "flour tortilla", "fraction": 0.25, "required": True},
            {"food_code": "41205010", "role": "refried beans", "fraction": 0.30, "required": True},
            {"food_code": "56205001", "role": "white rice", "fraction": 0.25, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.20, "required": True},
        ],
    },
    {
        "rule_id": "taco_salad_meat_sour_cream_v1",
        "family": "taco_salad",
        "priority": 200,
        "confidence": "low",
        "notes": "Low-confidence family split for taco or tostada salad with meat and sour cream.",
        "source_food_codes": ["58101945"],
        "match_terms": ["taco", "salad", "meat", "sour cream"],
        "exclude_terms": ["chicken", "meatless"],
        "components": [
            {"food_code": "75143050", "role": "lettuce salad", "fraction": 0.35, "required": True},
            {"food_code": "21500100", "role": "beef, ground", "fraction": 0.25, "required": True},
            {"food_code": "54401075", "role": "tortilla chips", "fraction": 0.20, "required": True},
            {"food_code": "12310100", "role": "sour cream", "fraction": 0.10, "required": True},
            {"food_code": "14104100", "role": "cheese, cheddar", "fraction": 0.10, "required": True},
        ],
    },
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
    resolver_match_keys = resolver_match_keys_for(food_rows)
    unit_conversions = unit_conversions_for(food_rows)
    food_by_code = {row["food_code"]: row for row in food_rows}
    validate_curated_resolution_references(food_by_code, APPROXIMATION_PROXIES, DECOMPOSITION_TEMPLATES, DECOMPOSITION_RULES)
    approximation_proxy_doc = approximation_proxy_document(APPROXIMATION_PROXIES)
    decomposition_template_doc = decomposition_template_document(DECOMPOSITION_TEMPLATES)
    decomposition_rule_doc = decomposition_rule_document(DECOMPOSITION_RULES)
    summary = classification_summary(
        food_rows,
        resolver_candidates,
        quarantined_foods,
        review_required_foods,
        resolver_match_keys,
        unit_conversions,
        APPROXIMATION_PROXIES,
        DECOMPOSITION_TEMPLATES,
        DECOMPOSITION_RULES,
    )

    out_dir.mkdir(parents=True, exist_ok=True)
    write_jsonl(out_dir / "foods.jsonl", food_rows)
    write_jsonl(out_dir / "nutrients.jsonl", nutrient_rows)
    write_jsonl(out_dir / "portions.jsonl", portion_rows)
    write_jsonl(out_dir / "resolver-candidates.jsonl", resolver_candidates)
    write_jsonl(out_dir / "resolver-match-keys.jsonl", resolver_match_keys)
    write_jsonl(out_dir / "unit-conversions.jsonl", unit_conversions)
    write_jsonl(out_dir / "quarantined-foods.jsonl", quarantined_foods)
    write_jsonl(out_dir / "review-required-foods.jsonl", review_required_foods)
    write_json(out_dir / "approximation-proxies.json", approximation_proxy_doc)
    write_json(out_dir / "decomposition-templates.json", decomposition_template_doc)
    write_json(out_dir / "decomposition-rules.json", decomposition_rule_doc)
    write_json(out_dir / "food-index.json", food_index(food_rows))
    write_json(out_dir / "classification-summary.json", summary)
    write_json(out_dir / "manifest.json", release_manifest(summary))
    write_sqlite(
        out_dir / "fndds.sqlite",
        food_rows,
        resolver_match_keys,
        unit_conversions,
        APPROXIMATION_PROXIES,
        DECOMPOSITION_TEMPLATES,
        DECOMPOSITION_RULES,
    )
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


def resolver_match_keys_for(foods: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows = []
    for food in foods:
        rows.extend(match_keys_for_food(food))
    rows.sort(
        key=lambda row: (
            row["normalized_match_key"],
            row["resolver_status"],
            row["food_code"],
            row["key_type"],
            row["match_key"],
        )
    )
    return rows


def match_keys_for_food(food: dict[str, Any]) -> list[dict[str, Any]]:
    resolver_status = resolver_status_for(food)
    block_reason = "" if resolver_status == MATCH_STATUS_AUTO else food["candidate_status"]
    rows: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    add_match_key(
        rows,
        seen,
        food,
        food["main_description"],
        "canonical",
        MATCH_CONFIDENCE_EXACT if resolver_status == MATCH_STATUS_AUTO else MATCH_CONFIDENCE_REVIEW,
        resolver_status,
        block_reason,
    )
    if resolver_status == MATCH_STATUS_AUTO and alias_generation_allowed(food):
        for key, key_type in alias_match_keys(food["main_description"]):
            add_match_key(
                rows,
                seen,
                food,
                key,
                key_type,
                MATCH_CONFIDENCE_HIGH,
                resolver_status,
                block_reason,
            )
    return rows


def add_match_key(
    rows: list[dict[str, Any]],
    seen: set[tuple[str, str]],
    food: dict[str, Any],
    match_key: str,
    key_type: str,
    confidence: str,
    resolver_status: str,
    block_reason: str,
) -> None:
    match_key = normalize_label(match_key)
    normalized = normalize_match_key(match_key)
    if not normalized:
        return
    if resolver_status not in VALID_MATCH_STATUSES:
        raise ValueError(f"invalid resolver_status {resolver_status}")
    if confidence not in VALID_MATCH_CONFIDENCES:
        raise ValueError(f"invalid match confidence {confidence}")
    dedupe_key = (normalized, key_type)
    if dedupe_key in seen:
        return
    seen.add(dedupe_key)
    rows.append(
        {
            "release": RELEASE,
            "food_code": food["food_code"],
            "source_description": food["main_description"],
            "match_key": match_key,
            "normalized_match_key": normalized,
            "key_type": key_type,
            "confidence": confidence,
            "resolver_status": resolver_status,
            "block_reason": block_reason,
        }
    )


def resolver_status_for(food: dict[str, Any]) -> str:
    if auto_resolver_candidate(food):
        return MATCH_STATUS_AUTO
    status = food["candidate_status"]
    if status == STATUS_REVIEW_REQUIRED:
        return MATCH_STATUS_REVIEW
    if status == STATUS_QUARANTINED_MIXED_DISH:
        return MATCH_STATUS_DECOMPOSE
    return MATCH_STATUS_BLOCKED


def auto_resolver_candidate(food: dict[str, Any]) -> bool:
    flags = set(food["ambiguity_flags"])
    if "missing_required_nutrients" in flags:
        return False
    if food["food_code"] in MATCH_KEY_AUTO_ALLOWLIST_CODES:
        return True
    if sandwich_component_base(food["main_description"]):
        return True
    if food["candidate_status"] not in {STATUS_ELIGIBLE_SPECIFIC, STATUS_ELIGIBLE_GENERIC}:
        return False
    return not (flags & AUTO_BLOCK_FLAGS)


def alias_generation_allowed(food: dict[str, Any]) -> bool:
    if food["food_code"] in MATCH_KEY_AUTO_ALLOWLIST_CODES:
        return True
    if sandwich_component_base(food["main_description"]):
        return True
    return "no added fat" in normalize_match_key(food["main_description"])


def alias_match_keys(main_description: str) -> list[tuple[str, str]]:
    aliases: list[tuple[str, str]] = []
    component = sandwich_component_base(main_description)
    if component:
        aliases.append((component, "component_alias"))
        singular = singularize(component)
        if singular != component:
            aliases.append((singular, "component_alias"))
        return aliases

    parts = [part.strip() for part in main_description.split(",") if part.strip()]
    if len(parts) < 2:
        return aliases

    base = parts[0].lower()
    qualifiers = normalized_alias_qualifiers(parts[1:])
    if not qualifiers:
        return aliases

    aliases.append((" ".join(qualifiers + [base]), "reordered"))
    if len(qualifiers) > 1:
        aliases.append((" ".join(list(reversed(qualifiers)) + [base]), "reordered"))
    for qualifier in qualifiers:
        aliases.append((f"{qualifier} {base}", "stripped_qualifier"))
    if "cooked" in qualifiers:
        without_cooked = [qualifier for qualifier in qualifiers if qualifier != "cooked"]
        if without_cooked:
            aliases.append((" ".join(["cooked", *without_cooked, base]), "stripped_qualifier"))
            aliases.append((f"cooked {base}", "stripped_qualifier"))
    if base.endswith("s"):
        singular_base = singularize(base)
        for key, key_type in list(aliases):
            aliases.append((key.replace(base, singular_base), key_type))
    return aliases


def normalized_alias_qualifiers(parts: list[str]) -> list[str]:
    qualifiers = []
    for part in parts:
        value = normalize_match_key(part)
        if value in BENIGN_ALIAS_QUALIFIERS:
            continue
        if value.startswith("ns as to") or value in {"nfs", "not specified", "quantity not specified"}:
            continue
        if " or " in value:
            value = value.split(" or ", 1)[0].strip()
        if value and value not in qualifiers:
            qualifiers.append(value)
    return qualifiers


def sandwich_component_base(main_description: str) -> str:
    normalized = normalize_match_key(main_description)
    suffix = " for use on a sandwich"
    if not normalized.endswith(suffix):
        return ""
    base = normalized[: -len(suffix)].strip()
    if base in SANDWICH_COMPONENT_BASES:
        return base
    return ""


def singularize(value: str) -> str:
    words = value.split()
    if not words:
        return value
    last = words[-1]
    if last == "tomatoes":
        words[-1] = "tomato"
    elif last.endswith("ies") and len(last) > 3:
        words[-1] = last[:-3] + "y"
    elif last.endswith("s") and not last.endswith("ss") and len(last) > 3:
        words[-1] = last[:-1]
    return " ".join(words)


def unit_conversions_for(foods: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows = []
    for food in foods:
        rows.extend(unit_conversions_for_food(food))
    rows.sort(key=lambda row: (row["food_code"], row["normalized_unit"], row["grams"]))
    return rows


def unit_conversions_for_food(food: dict[str, Any]) -> list[dict[str, Any]]:
    candidates: dict[str, list[tuple[float, str, str]]] = {}
    add_unit_candidates(candidates, ["g", "gram", "grams"], 1.0, "100 g base quantity", MATCH_CONFIDENCE_EXACT)
    for portion in food["portion_options"]:
        aliases = unit_aliases_for_portion(portion["description"])
        if not aliases:
            continue
        add_unit_candidates(candidates, aliases, portion["grams"], portion["description"], MATCH_CONFIDENCE_EXACT)

    rows = []
    for unit, values in sorted(candidates.items()):
        grams_values = {round(value[0], 6) for value in values}
        if len(grams_values) != 1:
            continue
        grams, source_description, confidence = values[0]
        rows.append(
            {
                "release": RELEASE,
                "food_code": food["food_code"],
                "unit": unit,
                "normalized_unit": normalize_unit_lookup(unit),
                "grams": grams,
                "source_description": source_description,
                "confidence": confidence,
            }
        )
    return rows


def add_unit_candidates(
    candidates: dict[str, list[tuple[float, str, str]]],
    units: list[str],
    grams: float,
    source_description: str,
    confidence: str,
) -> None:
    for unit in units:
        normalized = normalize_unit_lookup(unit)
        if not normalized:
            continue
        candidates.setdefault(normalized, []).append((grams, source_description, confidence))


def unit_aliases_for_portion(description: str) -> list[str]:
    normalized = normalize_unit_lookup(description)
    if (
        not normalized
        or "nfs" in normalized
        or "quantity not specified" in normalized
        or normalized.startswith("guideline amount")
        or "(" in description
        or ")" in description
    ):
        return []
    exact_aliases = {
        "1 cup": ["cup", "cups"],
        "1 fl oz": ["fl oz", "fluid ounce", "fluid ounces"],
        "1 fluid ounce": ["fl oz", "fluid ounce", "fluid ounces"],
        "1 oz": ["oz", "ounce", "ounces"],
        "1 ounce": ["oz", "ounce", "ounces"],
        "1 tbsp": ["tbsp", "tablespoon", "tablespoons"],
        "1 tablespoon": ["tbsp", "tablespoon", "tablespoons"],
        "1 tsp": ["tsp", "teaspoon", "teaspoons"],
        "1 teaspoon": ["tsp", "teaspoon", "teaspoons"],
        "1 slice": ["slice", "slices"],
        "1 piece": ["piece", "pieces"],
        "1 container": ["container", "containers"],
        "1 packet": ["packet", "packets"],
        "1 roll": ["roll", "rolls"],
        "1 cracker": ["cracker", "crackers"],
        "1 patty": ["patty", "patties"],
        "1 link": ["link", "links"],
    }
    return exact_aliases.get(normalized, [])


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


def validate_curated_resolution_references(
    food_by_code: dict[str, dict[str, Any]],
    approximation_proxies: list[dict[str, Any]],
    decomposition_templates: list[dict[str, Any]],
    decomposition_rules: list[dict[str, Any]],
) -> None:
    seen_proxy_keys: set[str] = set()
    for proxy in approximation_proxies:
        input_key = proxy["input_key"]
        normalized_key = normalize_match_key(input_key)
        if normalized_key in seen_proxy_keys:
            raise ValueError(f"duplicate approximation proxy input_key {input_key!r}")
        seen_proxy_keys.add(normalized_key)
        code = proxy["proxy_food_code"]
        if code not in food_by_code:
            raise ValueError(f"approximation proxy {input_key!r} references unknown food_code {code}")
        seen_source_codes: set[str] = set()
        for source_code in proxy.get("source_food_codes", []):
            if source_code in seen_source_codes:
                raise ValueError(f"approximation proxy {input_key!r} contains duplicate source_food_code {source_code}")
            seen_source_codes.add(source_code)
            if source_code not in food_by_code:
                raise ValueError(f"approximation proxy {input_key!r} references unknown source_food_code {source_code}")

    seen_template_ids: set[str] = set()
    seen_patterns: set[str] = set()
    for template in decomposition_templates:
        template_id = template["template_id"]
        if template_id in seen_template_ids:
            raise ValueError(f"duplicate decomposition template_id {template_id!r}")
        seen_template_ids.add(template_id)
        pattern = template["pattern"]
        normalized_pattern = normalize_match_key(pattern)
        if normalized_pattern in seen_patterns:
            raise ValueError(f"duplicate decomposition template pattern {pattern!r}")
        seen_patterns.add(normalized_pattern)
        components = template["components"]
        if not components:
            raise ValueError(f"decomposition template {template_id!r} must define components")
        fraction_total = 0.0
        for component in components:
            code = component["food_code"]
            if code not in food_by_code:
                raise ValueError(f"decomposition template {template_id!r} references unknown food_code {code}")
            fraction = component["fraction"]
            if fraction <= 0 or fraction >= 1:
                raise ValueError(f"decomposition template {template_id!r} has invalid fraction {fraction}")
            fraction_total += fraction
        if not math.isclose(fraction_total, 1.0, rel_tol=0.0, abs_tol=0.001):
            raise ValueError(f"decomposition template {template_id!r} fractions sum to {fraction_total}")

    seen_rule_ids: set[str] = set()
    seen_rule_source_codes: set[str] = set()
    for rule in decomposition_rules:
        rule_id = rule["rule_id"]
        if rule_id in seen_rule_ids:
            raise ValueError(f"duplicate decomposition rule_id {rule_id!r}")
        seen_rule_ids.add(rule_id)
        if rule["confidence"] not in {"low", "medium", "high"}:
            raise ValueError(f"decomposition rule {rule_id!r} has invalid confidence {rule['confidence']!r}")
        if rule["priority"] <= 0:
            raise ValueError(f"decomposition rule {rule_id!r} has invalid priority {rule['priority']}")
        seen_source_codes: set[str] = set()
        for source_code in rule.get("source_food_codes", []):
            if source_code in seen_source_codes:
                raise ValueError(f"decomposition rule {rule_id!r} contains duplicate source_food_code {source_code}")
            if source_code in seen_rule_source_codes:
                raise ValueError(f"decomposition source_food_code {source_code} is assigned to multiple rules")
            seen_source_codes.add(source_code)
            seen_rule_source_codes.add(source_code)
            if source_code not in food_by_code:
                raise ValueError(f"decomposition rule {rule_id!r} references unknown source_food_code {source_code}")
        for term_field in ["match_terms", "exclude_terms"]:
            terms = rule.get(term_field, [])
            seen_terms: set[str] = set()
            for term in terms:
                normalized = normalize_match_key(term)
                if not normalized:
                    raise ValueError(f"decomposition rule {rule_id!r} has empty {term_field} term {term!r}")
                if normalized in seen_terms:
                    raise ValueError(f"decomposition rule {rule_id!r} has duplicate {term_field} term {term!r}")
                seen_terms.add(normalized)
        if not rule.get("source_food_codes") and not rule.get("match_terms"):
            raise ValueError(f"decomposition rule {rule_id!r} must define source_food_codes or match_terms")
        components = rule["components"]
        if not components:
            raise ValueError(f"decomposition rule {rule_id!r} must define components")
        fraction_total = 0.0
        for component in components:
            code = component["food_code"]
            if code not in food_by_code:
                raise ValueError(f"decomposition rule {rule_id!r} references unknown food_code {code}")
            fraction = component["fraction"]
            if fraction <= 0 or fraction >= 1:
                raise ValueError(f"decomposition rule {rule_id!r} has invalid fraction {fraction}")
            fraction_total += fraction
        if not math.isclose(fraction_total, 1.0, rel_tol=0.0, abs_tol=0.001):
            raise ValueError(f"decomposition rule {rule_id!r} fractions sum to {fraction_total}")


def approximation_proxy_document(proxies: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "description": "Curated broad-food proxy mappings used only after exact resolver gates block a one-word broad food.",
        "proxies": [
            {
                **proxy,
                "normalized_input_key": normalize_match_key(proxy["input_key"]),
            }
            for proxy in sorted(proxies, key=lambda row: normalize_match_key(row["input_key"]))
        ],
    }


def decomposition_template_document(templates: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "description": "Curated decomposition templates for composed foods whose total gram weight can be split across known FNDDS components.",
        "templates": [
            {
                **template,
                "normalized_pattern": normalize_match_key(template["pattern"]),
            }
            for template in sorted(templates, key=lambda row: normalize_match_key(row["pattern"]))
        ],
    }


def decomposition_rule_document(rules: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "description": "Curated family-level decomposition rules for composed foods, matched by FNDDS source food code first and narrow text terms second.",
        "rules": [
            {
                **rule,
                "normalized_match_terms": [normalize_match_key(term) for term in rule.get("match_terms", [])],
                "normalized_exclude_terms": [normalize_match_key(term) for term in rule.get("exclude_terms", [])],
            }
            for rule in sorted(rules, key=lambda row: (row["priority"], row["rule_id"]))
        ],
    }


def classification_summary(
    foods: list[dict[str, Any]],
    resolver_candidates: list[dict[str, Any]],
    quarantined_foods: list[dict[str, Any]],
    review_required_foods: list[dict[str, Any]],
    resolver_match_keys: list[dict[str, Any]],
    unit_conversions: list[dict[str, Any]],
    approximation_proxies: list[dict[str, Any]],
    decomposition_templates: list[dict[str, Any]],
    decomposition_rules: list[dict[str, Any]],
) -> dict[str, Any]:
    status_counts = Counter(row["candidate_status"] for row in foods)
    flag_counts = Counter(flag for row in foods for flag in row["ambiguity_flags"])
    match_status_counts = Counter(row["resolver_status"] for row in resolver_match_keys)
    match_key_type_counts = Counter(row["key_type"] for row in resolver_match_keys)
    return {
        "schema_version": "0.1",
        "release": RELEASE,
        "food_count": len(foods),
        "resolver_candidate_count": len(resolver_candidates),
        "quarantined_count": len(quarantined_foods),
        "review_required_count": len(review_required_foods),
        "resolver_match_key_count": len(resolver_match_keys),
        "auto_match_key_count": match_status_counts[MATCH_STATUS_AUTO],
        "unit_conversion_count": len(unit_conversions),
        "approximation_proxy_count": len(approximation_proxies),
        "approximation_proxy_source_code_count": sum(len(proxy.get("source_food_codes", [])) for proxy in approximation_proxies),
        "decomposition_template_count": len(decomposition_templates),
        "decomposition_rule_count": len(decomposition_rules),
        "decomposition_rule_source_code_count": sum(len(rule.get("source_food_codes", [])) for rule in decomposition_rules),
        "decomposition_rule_component_count": sum(len(rule.get("components", [])) for rule in decomposition_rules),
        "status_counts": dict(sorted(status_counts.items())),
        "flag_counts": dict(sorted(flag_counts.items())),
        "match_status_counts": dict(sorted(match_status_counts.items())),
        "match_key_type_counts": dict(sorted(match_key_type_counts.items())),
        "allowlist_codes": dict(sorted(ALLOWLIST_CODES.items())),
        "match_key_auto_allowlist_codes": dict(sorted(MATCH_KEY_AUTO_ALLOWLIST_CODES.items())),
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
            "resolver-match-keys.jsonl",
            "unit-conversions.jsonl",
            "quarantined-foods.jsonl",
            "review-required-foods.jsonl",
            "approximation-proxies.json",
            "decomposition-templates.json",
            "decomposition-rules.json",
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


def normalize_match_key(value: str) -> str:
    value = value.lower().replace("&", " and ")
    value = re.sub(r"[^a-z0-9]+", " ", value)
    return " ".join(value.split())


def normalize_unit_lookup(value: str) -> str:
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


def write_sqlite(
    path: Path,
    foods: list[dict[str, Any]],
    resolver_match_keys: list[dict[str, Any]],
    unit_conversions: list[dict[str, Any]],
    approximation_proxies: list[dict[str, Any]],
    decomposition_templates: list[dict[str, Any]],
    decomposition_rules: list[dict[str, Any]],
) -> None:
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

            create table fndds_match_keys(
              food_code text not null references fndds_foods(food_code),
              source_description text not null,
              match_key text not null,
              normalized_match_key text not null,
              key_type text not null,
              confidence text not null,
              resolver_status text not null,
              block_reason text,
              primary key(food_code, normalized_match_key, key_type)
            );

            create table fndds_unit_conversions(
              food_code text not null references fndds_foods(food_code),
              unit text not null,
              normalized_unit text not null,
              grams real not null,
              source_description text not null,
              confidence text not null,
              primary key(food_code, normalized_unit)
            );

            create table fndds_approximation_proxies(
              input_key text primary key,
              normalized_input_key text not null,
              proxy_food_code text not null references fndds_foods(food_code),
              proxy_description text not null,
              confidence text not null,
              allow_when_allergies_present integer not null,
              allow_when_exclusions_present integer not null,
              estimate_reason text not null
            );

            create table fndds_approximation_proxy_source_codes(
              input_key text not null references fndds_approximation_proxies(input_key),
              source_food_code text not null references fndds_foods(food_code),
              primary key(input_key, source_food_code)
            );

            create table fndds_decomposition_templates(
              template_id text primary key,
              pattern text not null,
              normalized_pattern text not null,
              confidence text not null,
              notes text
            );

            create table fndds_decomposition_components(
              template_id text not null references fndds_decomposition_templates(template_id),
              position integer not null,
              food_code text not null references fndds_foods(food_code),
              role text not null,
              fraction real not null,
              required integer not null,
              primary key(template_id, position)
            );

            create table fndds_decomposition_rules(
              rule_id text primary key,
              family text not null,
              priority integer not null,
              confidence text not null,
              notes text
            );

            create table fndds_decomposition_rule_source_codes(
              rule_id text not null references fndds_decomposition_rules(rule_id),
              source_food_code text not null references fndds_foods(food_code),
              primary key(rule_id, source_food_code)
            );

            create table fndds_decomposition_rule_terms(
              rule_id text not null references fndds_decomposition_rules(rule_id),
              term_type text not null,
              position integer not null,
              term text not null,
              normalized_term text not null,
              primary key(rule_id, term_type, position)
            );

            create table fndds_decomposition_rule_components(
              rule_id text not null references fndds_decomposition_rules(rule_id),
              position integer not null,
              food_code text not null references fndds_foods(food_code),
              role text not null,
              fraction real not null,
              required integer not null,
              primary key(rule_id, position)
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
            create index idx_fndds_match_keys_normalized_status
              on fndds_match_keys(normalized_match_key, resolver_status);
            create index idx_fndds_match_keys_food_code
              on fndds_match_keys(food_code);
            create index idx_fndds_unit_conversions_food_code
              on fndds_unit_conversions(food_code);
            create index idx_fndds_approximation_proxies_normalized_input_key
              on fndds_approximation_proxies(normalized_input_key);
            create index idx_fndds_approximation_proxy_source_codes_source
              on fndds_approximation_proxy_source_codes(source_food_code);
            create index idx_fndds_decomposition_templates_normalized_pattern
              on fndds_decomposition_templates(normalized_pattern);
            create index idx_fndds_decomposition_components_template_id
              on fndds_decomposition_components(template_id);
            create index idx_fndds_decomposition_rule_source_codes_source
              on fndds_decomposition_rule_source_codes(source_food_code);
            create index idx_fndds_decomposition_rule_terms_type
              on fndds_decomposition_rule_terms(term_type, normalized_term);
            create index idx_fndds_decomposition_rule_components_rule_id
              on fndds_decomposition_rule_components(rule_id);
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
            for match_key in resolver_match_keys:
                conn.execute(
                    """
                    insert into fndds_match_keys(
                      food_code, source_description, match_key, normalized_match_key,
                      key_type, confidence, resolver_status, block_reason
                    ) values (?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        match_key["food_code"],
                        match_key["source_description"],
                        match_key["match_key"],
                        match_key["normalized_match_key"],
                        match_key["key_type"],
                        match_key["confidence"],
                        match_key["resolver_status"],
                        match_key["block_reason"],
                    ),
                )
            for conversion in unit_conversions:
                conn.execute(
                    """
                    insert into fndds_unit_conversions(
                      food_code, unit, normalized_unit, grams, source_description, confidence
                    ) values (?, ?, ?, ?, ?, ?)
                    """,
                    (
                        conversion["food_code"],
                        conversion["unit"],
                        conversion["normalized_unit"],
                        conversion["grams"],
                        conversion["source_description"],
                        conversion["confidence"],
                    ),
                )
            for proxy in approximation_proxies:
                conn.execute(
                    """
                    insert into fndds_approximation_proxies(
                      input_key, normalized_input_key, proxy_food_code,
                      proxy_description, confidence, allow_when_allergies_present,
                      allow_when_exclusions_present, estimate_reason
                    ) values (?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        proxy["input_key"],
                        normalize_match_key(proxy["input_key"]),
                        proxy["proxy_food_code"],
                        proxy["proxy_description"],
                        proxy["confidence"],
                        int(bool(proxy["allow_when_allergies_present"])),
                        int(bool(proxy["allow_when_exclusions_present"])),
                        proxy["estimate_reason"],
                    ),
                )
                for source_code in proxy.get("source_food_codes", []):
                    conn.execute(
                        """
                        insert into fndds_approximation_proxy_source_codes(
                          input_key, source_food_code
                        ) values (?, ?)
                        """,
                        (
                            proxy["input_key"],
                            source_code,
                        ),
                    )
            for template in decomposition_templates:
                conn.execute(
                    """
                    insert into fndds_decomposition_templates(
                      template_id, pattern, normalized_pattern, confidence, notes
                    ) values (?, ?, ?, ?, ?)
                    """,
                    (
                        template["template_id"],
                        template["pattern"],
                        normalize_match_key(template["pattern"]),
                        template["confidence"],
                        template["notes"],
                    ),
                )
                for position, component in enumerate(template["components"], start=1):
                    conn.execute(
                        """
                        insert into fndds_decomposition_components(
                          template_id, position, food_code, role, fraction, required
                        ) values (?, ?, ?, ?, ?, ?)
                        """,
                        (
                            template["template_id"],
                            position,
                            component["food_code"],
                            component["role"],
                            component["fraction"],
                            int(bool(component["required"])),
                        ),
                    )
            for rule in decomposition_rules:
                conn.execute(
                    """
                    insert into fndds_decomposition_rules(
                      rule_id, family, priority, confidence, notes
                    ) values (?, ?, ?, ?, ?)
                    """,
                    (
                        rule["rule_id"],
                        rule["family"],
                        rule["priority"],
                        rule["confidence"],
                        rule["notes"],
                    ),
                )
                for source_code in rule.get("source_food_codes", []):
                    conn.execute(
                        """
                        insert into fndds_decomposition_rule_source_codes(
                          rule_id, source_food_code
                        ) values (?, ?)
                        """,
                        (
                            rule["rule_id"],
                            source_code,
                        ),
                    )
                for term_type, field in [("match", "match_terms"), ("exclude", "exclude_terms")]:
                    for position, term in enumerate(rule.get(field, []), start=1):
                        conn.execute(
                            """
                            insert into fndds_decomposition_rule_terms(
                              rule_id, term_type, position, term, normalized_term
                            ) values (?, ?, ?, ?, ?)
                            """,
                            (
                                rule["rule_id"],
                                term_type,
                                position,
                                term,
                                normalize_match_key(term),
                            ),
                        )
                for position, component in enumerate(rule["components"], start=1):
                    conn.execute(
                        """
                        insert into fndds_decomposition_rule_components(
                          rule_id, position, food_code, role, fraction, required
                        ) values (?, ?, ?, ?, ?, ?)
                        """,
                        (
                            rule["rule_id"],
                            position,
                            component["food_code"],
                            component["role"],
                            component["fraction"],
                            int(bool(component["required"])),
                        ),
                    )
        conn.execute("pragma optimize")
    finally:
        conn.close()


if __name__ == "__main__":
    main()
