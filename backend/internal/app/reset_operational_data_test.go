package app

import (
	"context"
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/config"
)

func TestResetOperationalDataRequiresExplicitFlag(t *testing.T) {
	result, err := ResetOperationalData(
		context.Background(),
		nil,
		config.Config{AllowOperationalDataReset: false},
		operationalResetConfirmation,
	)
	if err == nil || !strings.Contains(err.Error(), "ALLOW_OPERATIONAL_DATA_RESET=true") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DryRun {
		t.Fatal("exact confirmation should select execution mode")
	}
}
