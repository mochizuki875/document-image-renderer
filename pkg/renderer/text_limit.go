package renderer

import (
	"strings"
	"unicode/utf8"
)

type characterBudget struct {
	count int
	max   int
}

func (budget *characterBudget) append(builder *strings.Builder, text string) error {
	count := utf8.RuneCountInString(text)
	if budget.max > 0 && (budget.count > budget.max || count > budget.max-budget.count) {
		return &CharacterLimitExceededError{CharacterCount: budget.count + count, MaxCharacters: budget.max}
	}
	budget.count += count
	builder.WriteString(text)
	return nil
}

func appendBounded(builder *strings.Builder, count *int, text string, max, configuredMax int, limited bool) error {
	characters := utf8.RuneCountInString(text)
	if limited && (*count > max || characters > max-*count) {
		return &CharacterLimitExceededError{CharacterCount: configuredMax + 1, MaxCharacters: configuredMax}
	}
	*count += characters
	builder.WriteString(text)
	return nil
}
