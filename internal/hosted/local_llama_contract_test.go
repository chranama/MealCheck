package hosted

import "testing"

func TestDecodeLocalLlamaCompactPlanExpandsCanonicalPlan(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"b":[["cooked oatmeal",1,"cup"]],
		"l":[["grilled chicken breast",4,"oz"]],
		"d":[["baked salmon",4,"oz"]]
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
	if len(plan.Days) != 1 {
		t.Fatalf("days length = %d, want 1", len(plan.Days))
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
			name: "unknown top level field",
			text: `{"b":[["oatmeal",1,"cup"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]],"s":[]}`,
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
			text: `{"b":[["toast",1,"slice"]],"l":[["rice",1,"cup"]],"d":[["salmon",4,"oz"]]}`,
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
	if len(required) != 3 {
		t.Fatalf("required length = %d, want 3", len(required))
	}
	for index, want := range []string{"b", "l", "d"} {
		if required[index] != want {
			t.Fatalf("required[%d] = %q, want %q", index, required[index], want)
		}
	}
	properties := schema["properties"].(map[string]any)
	breakfast := properties["b"].(map[string]any)
	item := breakfast["items"].(map[string]any)
	tuple := item["items"].([]map[string]any)
	if len(tuple) != 3 {
		t.Fatalf("tuple length = %d, want 3", len(tuple))
	}
	if tuple[0]["type"] != "string" || tuple[1]["type"] != "number" || tuple[2]["type"] != "string" {
		t.Fatalf("tuple item types = %#v, want string/number/string", tuple)
	}
}
