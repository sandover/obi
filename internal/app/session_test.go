package app

import (
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func TestAliasFromRequestUsesRequestWhenProvided(t *testing.T) {
	epic := config.EpicConfig{Alias: "foo"}
	if got := aliasFromRequest("custom", "foo", epic); got != "foo" {
		t.Fatalf("expected canonical alias, got %s", got)
	}
}

func TestAliasFromRequestFallsBackToConfiguredAlias(t *testing.T) {
	epic := config.EpicConfig{Alias: "foo"}
	if got := aliasFromRequest("", "foo", epic); got != "foo" {
		t.Fatalf("expected fallback alias foo, got %s", got)
	}
}
