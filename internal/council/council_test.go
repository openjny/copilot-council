package council

import (
	"testing"
)

func TestDefaultModels(t *testing.T) {
	models := DefaultModels()
	if len(models) == 0 {
		t.Error("DefaultModels() returned empty list")
	}

	expectedModels := []string{
		"claude-sonnet-4.5",
		"gpt-5.2",
		"gemini-3-pro-preview",
	}

	for i, model := range models {
		if model != expectedModels[i] {
			t.Errorf("Expected model %s at index %d, got %s", expectedModels[i], i, model)
		}
	}
}

func TestDefaultAggregator(t *testing.T) {
	aggregator := DefaultAggregator()
	if aggregator == "" {
		t.Error("DefaultAggregator() returned empty string")
	}
	
	expected := "gpt-4.1"
	if aggregator != expected {
		t.Errorf("Expected aggregator %s, got %s", expected, aggregator)
	}
}
