package hosted

import "testing"

func TestDecodeLocalLlamaCompactPlanExpandsCanonicalPlan(t *testing.T) {
	plan, err := DecodeLocalLlamaCompactPlan(`{
		"breakfast":[{"f":"cooked oatmeal","q":1,"u":"cup"}],
		"lunch":[{"f":"grilled chicken breast","q":4,"u":"oz"}],
		"dinner":[{"f":"baked salmon","q":4,"u":"oz"}]
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

func TestDecodeLocalLlamaCompactPlanRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "unknown top level field",
			text: `{"breakfast":[{"f":"oatmeal","q":1,"u":"cup"}],"lunch":[{"f":"rice","q":1,"u":"cup"}],"dinner":[{"f":"salmon","q":4,"u":"oz"}],"snack":[]}`,
		},
		{
			name: "unknown item field",
			text: `{"breakfast":[{"f":"oatmeal","q":1,"u":"cup","food":"oatmeal"}],"lunch":[{"f":"rice","q":1,"u":"cup"}],"dinner":[{"f":"salmon","q":4,"u":"oz"}]}`,
		},
		{
			name: "missing meal",
			text: `{"breakfast":[{"f":"oatmeal","q":1,"u":"cup"}],"lunch":[{"f":"rice","q":1,"u":"cup"}]}`,
		},
		{
			name: "unsupported unit",
			text: `{"breakfast":[{"f":"toast","q":1,"u":"slice"}],"lunch":[{"f":"rice","q":1,"u":"cup"}],"dinner":[{"f":"salmon","q":4,"u":"oz"}]}`,
		},
		{
			name: "nonpositive quantity",
			text: `{"breakfast":[{"f":"oatmeal","q":0,"u":"cup"}],"lunch":[{"f":"rice","q":1,"u":"cup"}],"dinner":[{"f":"salmon","q":4,"u":"oz"}]}`,
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
	for index, want := range []string{"breakfast", "lunch", "dinner"} {
		if required[index] != want {
			t.Fatalf("required[%d] = %q, want %q", index, required[index], want)
		}
	}
	properties := schema["properties"].(map[string]any)
	breakfast := properties["breakfast"].(map[string]any)
	item := breakfast["items"].(map[string]any)
	itemRequired := item["required"].([]string)
	for index, want := range []string{"f", "q", "u"} {
		if itemRequired[index] != want {
			t.Fatalf("item required[%d] = %q, want %q", index, itemRequired[index], want)
		}
	}
}
