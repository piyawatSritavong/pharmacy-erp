package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactAuditValueRemovesNestedSecrets(t *testing.T) {
	redacted := redactAuditValue(map[string]any{
		"provider":    "shopee",
		"credentials": map[string]any{"api_key": "top-secret"},
		"settings":    []any{map[string]any{"access_token": "token-value", "region": "TH"}},
	}).(map[string]any)
	if redacted["provider"] != "shopee" {
		t.Fatalf("expected non-sensitive audit field to remain, got %#v", redacted)
	}
	if redacted["credentials"] != "[REDACTED]" {
		t.Fatalf("credentials were not redacted: %#v", redacted)
	}
	settings := redacted["settings"].([]any)[0].(map[string]any)
	if settings["access_token"] != "[REDACTED]" || settings["region"] != "TH" {
		t.Fatalf("nested audit redaction is incorrect: %#v", settings)
	}
}

func TestRedactAuditValueHandlesAliasesEmbeddedJSONAndLargeIntegers(t *testing.T) {
	const largeInteger int64 = 9_223_372_036_854_775_000
	redacted := redactAuditValue(map[string]any{
		"accessToken": "top-secret",
		"nestedJSON":  `{"clientSecret":"hidden","safe":"visible"}`,
		"sequence":    largeInteger,
	}).(map[string]any)
	if redacted["accessToken"] != "[REDACTED]" {
		t.Fatalf("camelCase token alias was not redacted: %#v", redacted)
	}
	if embedded, _ := redacted["nestedJSON"].(string); strings.Contains(embedded, "hidden") || !strings.Contains(embedded, "[REDACTED]") {
		t.Fatalf("embedded JSON secret was not redacted: %q", embedded)
	}
	if got := redacted["sequence"].(json.Number).String(); got != "9223372036854775000" {
		t.Fatalf("large audit integer changed: %s", got)
	}
}
