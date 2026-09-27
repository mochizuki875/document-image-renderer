package renderer

import (
	"strings"
	"testing"
)

func TestCharacterBudgetAppendEnforcesLimit(t *testing.T) {
	budget := &characterBudget{max: 3}
	var builder strings.Builder
	if err := budget.append(&builder, "abc"); err != nil {
		t.Fatalf("append within limit: %v", err)
	}
	if err := budget.append(&builder, "d"); err == nil {
		t.Fatal("expected limit error")
	}
}

func TestCharacterBudgetAppendCountsRunes(t *testing.T) {
	budget := &characterBudget{max: 2}
	var builder strings.Builder
	if err := budget.append(&builder, "あ"); err != nil {
		t.Fatal(err)
	}
	if budget.count != 1 {
		t.Fatalf("count = %d, want 1", budget.count)
	}
}

func TestAppendBoundedEnforcesLimit(t *testing.T) {
	count := 0
	var builder strings.Builder
	if err := appendBounded(&builder, &count, "abc", 3, 3, true); err != nil {
		t.Fatalf("append within limit: %v", err)
	}
	if err := appendBounded(&builder, &count, "d", 3, 3, true); err == nil {
		t.Fatal("expected limit error")
	}
}
