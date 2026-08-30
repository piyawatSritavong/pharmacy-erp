package app

import (
	"context"
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/config"
)

func TestReplaceOchaCatalogRequiresEnvironmentGuard(t *testing.T) {
	result, err := ReplaceOchaCatalog(context.Background(), nil, config.Config{}, replaceOchaCatalogConfirmation)
	if err == nil || !strings.Contains(err.Error(), "ALLOW_MASTER_DATA_RESET=true") {
		t.Fatalf("expected environment guard error, got result=%#v err=%v", result, err)
	}
}
