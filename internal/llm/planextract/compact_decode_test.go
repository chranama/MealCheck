package planextract

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeLocalLlamaCompactPlanExpandsCanonicalPlan(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"i":[
			[1,1,"b","cooked oatmeal",1,"cup"],
			[2,1,"l","grilled chicken breast",4,"oz"],
			[3,1,"d","baked salmon",4,"oz"],
			[4,2,"b","plain Greek yogurt",1,"cup"],
			[5,2,"l","brown rice",1,"cup"],
			[6,2,"d","spinach",1,"cup"]
		]
	}`, "compact-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan error: %v", err)
	}
	if plan.SchemaVersion != "0.1" {
		t.Fatalf("schema_version = %q, want 0.1", plan.SchemaVersion)
	}
	if plan.PlanID != "compact-test" {
		t.Fatalf("plan_id = %q, want compact-test", plan.PlanID)
	}
	if len(plan.Days) != 2 {
		t.Fatalf("days length = %d, want 2", len(plan.Days))
	}
	meals := plan.Days[0].Meals
	if len(meals) != 3 {
		t.Fatalf("meals length = %d, want 3", len(meals))
	}
	for index, wantName := range []string{"breakfast", "lunch", "dinner"} {
		if meals[index].Name != wantName {
			t.Fatalf("meal %d name = %q, want %q", index, meals[index].Name, wantName)
		}
		if len(meals[index].Items) != 1 {
			t.Fatalf("meal %s items length = %d, want 1", wantName, len(meals[index].Items))
		}
		if meals[index].Items[0].Quantity == nil {
			t.Fatalf("meal %s item quantity = nil", wantName)
		}
	}
}

func TestDecodeLocalLlamaCompactPlanAcceptsLegacyV3Rows(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"i":[
			[1,"b","cooked oatmeal",1,"cup"],
			[1,"l","grilled chicken breast",4,"oz"],
			[1,"d","baked salmon",4,"oz"]
		]
	}`, "legacy-row-compact-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan legacy row error: %v", err)
	}
	if plan.PlanID != "legacy-row-compact-test" {
		t.Fatalf("plan_id = %q, want legacy-row-compact-test", plan.PlanID)
	}
	if len(plan.Days) != 1 || len(plan.Days[0].Meals) != 3 {
		t.Fatalf("plan days/meals = %d/%d, want 1/3", len(plan.Days), len(plan.Days[0].Meals))
	}
}

func TestDecodeLocalLlamaCompactPlanAcceptsUnresolvedRows(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"i":[
			[1,1,"b","cooked oatmeal",1,"cup"],
			[2,1,"b","banana",null,"","missing quantity","missing_quantity"],
			[3,1,"l","grilled chicken breast",4,"oz"]
		]
	}`, "unresolved-row-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan unresolved row error: %v", err)
	}
	if len(plan.Days) != 1 || len(plan.Days[0].Meals) != 2 {
		t.Fatalf("plan days/meals = %d/%d, want 1/2", len(plan.Days), len(plan.Days[0].Meals))
	}
	item := plan.Days[0].Meals[0].Items[1]
	if item.Food != "banana" || item.Quantity != nil || item.QuantityText != "missing quantity" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason != "missing_quantity" {
		t.Fatalf("unresolved item = %+v", item)
	}
}

func TestDecodeLocalLlamaCompactPlanAcceptsV2TupleItems(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"b":[["cooked oatmeal",1,"cup"]],
		"l":[["grilled chicken breast",4,"oz"]],
		"d":[["baked salmon",4,"oz"]]
	}`, "tuple-compact-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan tuple error: %v", err)
	}
	if plan.PlanID != "tuple-compact-test" {
		t.Fatalf("plan_id = %q, want tuple-compact-test", plan.PlanID)
	}
	if len(plan.Days) != 1 || len(plan.Days[0].Meals) != 3 {
		t.Fatalf("plan days/meals = %d/%d, want 1/3", len(plan.Days), len(plan.Days[0].Meals))
	}
}

func TestDecodeLocalLlamaCompactPlanCleansDuplicateQuantityFromFood(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"i":[
				[1,1,"b","1 cup cooked oatmeal",1,"cup"],
				[2,1,"b","1/2 cup blueberries",0.5,"cup"],
				[5,1,"b","2 slice whole wheat bread",2,"slice"],
				[3,1,"l","1 1/2 cup brown rice",1.5,"cup"],
				[4,1,"d","1 tbsp olive oil",1,"tsp"]
			]
	}`, "duplicate-quantity-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan error: %v", err)
	}
	breakfast := plan.Days[0].Meals[0].Items
	if breakfast[0].Food != "cooked oatmeal" || breakfast[0].Unit != "cup" {
		t.Fatalf("first item = %+v, want cleaned cooked oatmeal cup", breakfast[0])
	}
	if breakfast[1].Food != "blueberries" || breakfast[1].Unit != "cup" {
		t.Fatalf("second item = %+v, want cleaned blueberries cup", breakfast[1])
	}
	if breakfast[2].Food != "whole wheat bread" || breakfast[2].Unit != "slice" {
		t.Fatalf("slice item = %+v, want cleaned whole wheat bread slice", breakfast[2])
	}
	lunch := plan.Days[0].Meals[1].Items
	if lunch[0].Food != "brown rice" || lunch[0].Unit != "cup" {
		t.Fatalf("third item = %+v, want cleaned brown rice cup", lunch[0])
	}
	dinner := plan.Days[0].Meals[2].Items
	if dinner[0].Food != "olive oil" || dinner[0].Unit != "tbsp" {
		t.Fatalf("fourth item = %+v, want cleaned olive oil tbsp", dinner[0])
	}
}

func TestLocalLlamaParseSourceMeasurement(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		food     string
		quantity float64
		unit     string
	}{
		{
			name:     "integer",
			text:     "4 oz chicken breast",
			food:     "chicken breast",
			quantity: 4,
			unit:     "oz",
		},
		{
			name:     "decimal",
			text:     "0.5 cup blueberries",
			food:     "blueberries",
			quantity: 0.5,
			unit:     "cup",
		},
		{
			name:     "fraction",
			text:     "1/2 cup blueberries",
			food:     "blueberries",
			quantity: 0.5,
			unit:     "cup",
		},
		{
			name:     "spaced fraction",
			text:     "1 / 2 cup blueberries",
			food:     "blueberries",
			quantity: 0.5,
			unit:     "cup",
		},
		{
			name:     "mixed number",
			text:     "1 1/2 cups brown rice",
			food:     "brown rice",
			quantity: 1.5,
			unit:     "cup",
		},
		{
			name:     "of cleanup",
			text:     "1 cup of cooked oatmeal.",
			food:     "cooked oatmeal",
			quantity: 1,
			unit:     "cup",
		},
		{
			name:     "unit alias",
			text:     "2 tablespoons olive oil",
			food:     "olive oil",
			quantity: 2,
			unit:     "tbsp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LocalLlamaParseSourceMeasurement(tt.text)
			if got.Status != "parsed" {
				t.Fatalf("status = %q reason=%q, want parsed", got.Status, got.Reason)
			}
			if got.Food != tt.food || got.Quantity != tt.quantity || got.Unit != tt.unit {
				t.Fatalf("measurement = %+v, want food=%q quantity=%v unit=%q", got, tt.food, tt.quantity, tt.unit)
			}
		})
	}
}

func TestDecodeLocalLlamaCompactPlanWithSourceRepairsMeasurementDrift(t *testing.T) {
	source := strings.Join([]string{
		"Day 1 breakfast:",
		"- 1/2 cup blueberries",
		"- 1 tbsp olive oil",
		"- 5 oz turkey meatballs",
		"- 1 cup plain Greek yogurt",
	}, "\n")
	plan, repairs, err := DecodeLocalLlamaCompactPlanWithSource(`{
		"i":[
			[4,1,"b","Greek yogurt",1,"cup"],
			[2,1,"b","1 tbsp olive oil",1,"tsp"],
			[3,1,"b","5 oz turkey meatballs",1,"serving"],
			[1,1,"b","1/2 cup blueberries",1,"cup"]
		]
	}`, "source-repair-test", source)
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlanWithSource error: %v", err)
	}
	if len(repairs) == 0 {
		t.Fatal("repairs length = 0, want source-grounded repairs")
	}
	items := plan.Days[0].Meals[0].Items
	if len(items) != 4 {
		t.Fatalf("items length = %d, want 4", len(items))
	}
	assertItem := func(index int, food string, quantity float64, unit string) {
		t.Helper()
		item := items[index]
		if item.Quantity == nil {
			t.Fatalf("item %d quantity = nil", index)
		}
		if item.Food != food || *item.Quantity != quantity || item.Unit != unit {
			t.Fatalf("item %d = %+v, want food=%q quantity=%v unit=%q", index, item, food, quantity, unit)
		}
	}
	assertItem(0, "blueberries", 0.5, "cup")
	assertItem(1, "olive oil", 1, "tbsp")
	assertItem(2, "turkey meatballs", 5, "oz")
	assertItem(3, "plain Greek yogurt", 1, "cup")
}

func TestDecodeLocalLlamaMealChunkRowsReattachesDeterministicMeal(t *testing.T) {
	chunk := localLlamaMealChunk{
		Day:       1,
		MealCode:  "l",
		MealLabel: "lunch",
		MealText:  "Lunch: chicken, 100 g, and a side salad.",
		Items: []localLlamaSourceItem{
			{ID: 4, Day: 1, MealCode: "l", Text: "100 g chicken", ParseStatus: localLlamaSourceResolved},
			{ID: 5, Day: 1, MealCode: "l", Text: "a side salad", ParseStatus: localLlamaSourceNeedsModelParse},
		},
	}
	rows, repairs, err := decodeLocalLlamaMealChunkRows(`{"i":[[4,"chicken",4,"oz"],[5,"side salad",null,"","missing quantity","missing_quantity"]]}`, chunk)
	if err != nil {
		t.Fatalf("decodeLocalLlamaMealChunkRows error: %v", err)
	}
	if len(repairs) == 0 {
		t.Fatal("repairs length = 0, want source-measurement repair for resolved source")
	}
	if len(rows) != 2 {
		t.Fatalf("rows length = %d, want 2", len(rows))
	}
	if rows[0].Day != 1 || rows[0].MealCode != "l" || rows[0].Food != "chicken" || rows[0].Quantity != 100 || rows[0].Unit != "g" {
		t.Fatalf("first row = %+v, want server meal metadata and repaired source measurement", rows[0])
	}
	if rows[1].Day != 1 || rows[1].MealCode != "l" || rows[1].Food != "side salad" || rows[1].QuantityText != "missing quantity" {
		t.Fatalf("second row = %+v, want server meal metadata and unresolved fields", rows[1])
	}
}

func TestDecodeLocalLlamaMealChunkRowsRejectsUnexpectedSourceID(t *testing.T) {
	chunk := localLlamaMealChunk{
		Day:      1,
		MealCode: "b",
		Items: []localLlamaSourceItem{
			{ID: 1, Day: 1, MealCode: "b", Text: "1 cup oatmeal", ParseStatus: localLlamaSourceResolved},
		},
	}
	_, _, err := decodeLocalLlamaMealChunkRows(`{"i":[[2,"oatmeal",1,"cup"]]}`, chunk)
	if err == nil {
		t.Fatal("decodeLocalLlamaMealChunkRows error = nil, want source ID rejection")
	}
	if !strings.Contains(err.Error(), "unexpected source_item_id 2") {
		t.Fatalf("error = %q, want unexpected source_item_id", err)
	}
}

func TestDecodeLocalLlamaCompactPlanAcceptsLegacyObjectItems(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"breakfast":[{"f":"cooked oatmeal","q":1,"u":"cup"}],
		"lunch":[{"f":"grilled chicken breast","q":4,"u":"oz"}],
		"dinner":[{"f":"baked salmon","q":4,"u":"oz"}]
	}`, "legacy-compact-test")
	if err != nil {
		t.Fatalf("DecodeLocalLlamaCompactPlan legacy error: %v", err)
	}
	if plan.PlanID != "legacy-compact-test" {
		t.Fatalf("plan_id = %q, want legacy-compact-test", plan.PlanID)
	}
	if got := plan.Days[0].Meals[0].Items[0].Food; got != "cooked oatmeal" {
		t.Fatalf("first food = %q, want cooked oatmeal", got)
	}
}

func TestDecodeLocalLlamaCompactPlanRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "row unknown top level field",
			text: `{"i":[[1,1,"b","oatmeal",1,"cup"]],"x":[]}`,
		},
		{
			name: "row tuple has wrong length",
			text: `{"i":[[1,1,"b","oatmeal",1,"cup","extra","extra"]]}`,
		},
		{
			name: "row source item id duplicated",
			text: `{"i":[[1,1,"b","oatmeal",1,"cup"],[1,1,"l","rice",1,"cup"]]}`,
		},
		{
			name: "row source item id missing",
			text: `{"i":[[1,1,"b","oatmeal",1,"cup"],[3,1,"l","rice",1,"cup"]]}`,
		},
		{
			name: "row mixes source id and legacy shapes",
			text: `{"i":[[1,1,"b","oatmeal",1,"cup"],[1,"l","rice",1,"cup"]]}`,
		},
		{
			name: "row unsupported meal code",
			text: `{"i":[[1,1,"x","oatmeal",1,"cup"]]}`,
		},
		{
			name: "row day out of range",
			text: `{"i":[[1,8,"b","oatmeal",1,"cup"]]}`,
		},
		{
			name: "v2 unknown top level field",
			text: `{"b":[["oatmeal",1,"cup"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]],"x":[]}`,
		},
		{
			name: "tuple has wrong length",
			text: `{"b":[["oatmeal",1,"cup","extra"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]]}`,
		},
		{
			name: "missing meal",
			text: `{"b":[["oatmeal",1,"cup"]],"l":[["rice",1,"cup"]]}`,
		},
		{
			name: "unsupported unit",
			text: `{"b":[["toast",1,"loaf"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]]}`,
		},
		{
			name: "nonpositive quantity",
			text: `{"b":[["oatmeal",0,"cup"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]]}`,
		},
		{
			name: "legacy unknown item field",
			text: `{"breakfast":[{"f":"oatmeal","q":1,"u":"cup","food":"oatmeal"}],"lunch":[{"f":"rice","q":1,"u":"cup"}],"dinner":[{"f":"salmon","q":4,"u":"oz"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeLocalLlamaCompactPlan(tt.text, "compact-test"); err == nil {
				t.Fatal("DecodeLocalLlamaCompactPlan error = nil, want error")
			}
		})
	}
}

func TestLocalLlamaCompactResponseSchemaUsesCompactFields(t *testing.T) {
	schema := LocalLlamaCompactResponseSchema()
	required := schema["required"].([]string)
	if len(required) != 1 || required[0] != "i" {
		t.Fatalf("required = %#v, want [i]", required)
	}
	properties := schema["properties"].(map[string]any)
	items := properties["i"].(map[string]any)
	rowSchema := items["items"].(map[string]any)
	options := rowSchema["oneOf"].([]map[string]any)
	if len(options) != 2 {
		t.Fatalf("row schema options = %d, want 2", len(options))
	}
	resolved := options[0]
	if resolved["minItems"] != 6 || resolved["maxItems"] != 6 {
		t.Fatalf("resolved row min/max items = %v/%v, want 6/6", resolved["minItems"], resolved["maxItems"])
	}
	unresolved := options[1]
	if unresolved["minItems"] != 8 || unresolved["maxItems"] != 8 {
		t.Fatalf("unresolved row min/max items = %v/%v, want 8/8", unresolved["minItems"], unresolved["maxItems"])
	}
	tuple := resolved["items"].([]map[string]any)
	if len(tuple) != 6 {
		t.Fatalf("tuple length = %d, want 6", len(tuple))
	}
	if tuple[0]["type"] != "integer" || tuple[1]["type"] != "integer" || tuple[2]["type"] != "string" || tuple[3]["type"] != "string" || tuple[4]["type"] != "number" || tuple[5]["type"] != "string" {
		t.Fatalf("tuple item types = %#v, want integer/integer/string/string/number/string", tuple)
	}
	unresolvedTuple := unresolved["items"].([]map[string]any)
	if len(unresolvedTuple) != 8 || unresolvedTuple[6]["type"] != "string" {
		t.Fatalf("unresolved tuple = %#v, want quantity_text field", unresolvedTuple)
	}
	mealCodes := tuple[2]["enum"].([]string)
	if len(mealCodes) < 6 {
		t.Fatalf("meal code enum = %#v, want at least 6 codes", mealCodes)
	}
}

func TestLocalLlamaCompactResponseSchemaMatchesFixture(t *testing.T) {
	var fixture map[string]any
	decodeJSON(t, readFile(t, filepath.Join(repoRoot(t), "examples/local-llama/full-row-compact-meal-plan-response.schema.json")), &fixture)

	b, err := json.Marshal(LocalLlamaCompactResponseSchema())
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var generated map[string]any
	if err := json.Unmarshal(b, &generated); err != nil {
		t.Fatalf("unmarshal generated schema: %v", err)
	}
	if !reflect.DeepEqual(generated, fixture) {
		t.Fatalf("generated schema does not match fixture\ngot:  %#v\nwant: %#v", generated, fixture)
	}
}

func TestLocalLlamaMealChunkResponseSchemaMatchesFixture(t *testing.T) {
	var fixture map[string]any
	decodeJSON(t, readFile(t, filepath.Join(repoRoot(t), "examples/local-llama/compact-meal-plan-response.schema.json")), &fixture)

	b, err := json.Marshal(LocalLlamaMealChunkResponseSchema())
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var generated map[string]any
	if err := json.Unmarshal(b, &generated); err != nil {
		t.Fatalf("unmarshal generated: %v", err)
	}
	if !reflect.DeepEqual(generated, fixture) {
		t.Fatalf("generated meal-chunk schema does not match fixture\ngot:  %#v\nwant: %#v", generated, fixture)
	}
}
