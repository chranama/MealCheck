package normalization

import (
	"strings"
	"testing"
)

func TestBuildDeterministicPlanFromNaturalMealText(t *testing.T) {
	plan, parsedItems, err := BuildDeterministicPlan(strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal, 0.5 cup blueberries, and 1 cup plain Greek yogurt.",
		"Day 1 lunch: 4 oz grilled chicken breast, 1 cup brown rice, and 1 cup steamed broccoli.",
		"Day 2 dinner: 5 oz turkey meatballs, 1 cup whole wheat pasta, and 1 cup tomato sauce.",
	}, "\n"), "test-plan")
	if err != nil {
		t.Fatalf("BuildDeterministicPlan error: %v", err)
	}
	if len(parsedItems) != 9 {
		t.Fatalf("parsed item count = %d, want 9", len(parsedItems))
	}
	if len(plan.Days) != 2 {
		t.Fatalf("day count = %d, want 2", len(plan.Days))
	}
	if plan.Days[0].Day != 1 || plan.Days[1].Day != 2 {
		t.Fatalf("days = %+v, want day 1 then day 2", plan.Days)
	}
	if got := plan.Days[0].Meals[0].Name; got != "breakfast" {
		t.Fatalf("first meal name = %q, want breakfast", got)
	}
	item := plan.Days[0].Meals[0].Items[1]
	if item.Food != "blueberries" || item.Quantity == nil || *item.Quantity != 0.5 || item.Unit != "cup" {
		t.Fatalf("blueberry item = %+v, want 0.5 cup blueberries", item)
	}
}

func TestBuildDeterministicPlanRejectsMissingMealCode(t *testing.T) {
	_, _, err := BuildDeterministicPlan("- 1 cup cooked oatmeal", "test-plan")
	if err == nil {
		t.Fatal("BuildDeterministicPlan error = nil, want missing meal code error")
	}
}
