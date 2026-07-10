package branches

import "testing"

func TestValidateSequenceUpdateAllowsUnlockedEdit(t *testing.T) {
	prefix, err := validateSequenceUpdate("BL", 5, false, UpdateSequenceRequest{
		Prefix:     "qt",
		NextNumber: 9,
		IsLocked:   true,
	})
	if err != nil {
		t.Fatalf("expected unlocked edit to pass: %v", err)
	}
	if prefix != "QT" {
		t.Fatalf("expected normalized prefix QT, got %s", prefix)
	}
}

func TestValidateSequenceUpdateRejectsLockedFieldEdit(t *testing.T) {
	_, err := validateSequenceUpdate("BL", 5, true, UpdateSequenceRequest{
		Prefix:     "BLX",
		NextNumber: 5,
		IsLocked:   true,
	})
	if err == nil {
		t.Fatal("expected locked edit to be rejected")
	}
}

func TestValidateSequenceUpdateAllowsUnlockOnly(t *testing.T) {
	prefix, err := validateSequenceUpdate("BL", 5, true, UpdateSequenceRequest{
		Prefix:     "BL",
		NextNumber: 5,
		IsLocked:   false,
	})
	if err != nil {
		t.Fatalf("expected unlock-only update to pass: %v", err)
	}
	if prefix != "BL" {
		t.Fatalf("unexpected prefix: %s", prefix)
	}
}
