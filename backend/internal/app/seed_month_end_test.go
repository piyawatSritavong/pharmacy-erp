package app

import (
	"context"
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/config"
)

func TestSeedMonthEndRejectsProduction(t *testing.T) {
	err := SeedMonthEnd(context.Background(), nil, config.Config{AppEnv: "production", AllowDestructiveSeed: true})
	if err == nil || !strings.Contains(err.Error(), "development or test") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeedMonthEndRequiresExplicitFlag(t *testing.T) {
	err := SeedMonthEnd(context.Background(), nil, config.Config{AppEnv: "development", AllowDestructiveSeed: false})
	if err == nil || !strings.Contains(err.Error(), "ALLOW_DESTRUCTIVE_SEED") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeedInventoryFloorRejectsProduction(t *testing.T) {
	_, err := SeedInventoryFloor(context.Background(), nil, config.Config{AppEnv: "production"})
	if err == nil || !strings.Contains(err.Error(), "development or test") {
		t.Fatalf("unexpected error: %v", err)
	}
}
