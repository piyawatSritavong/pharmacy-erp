package transfers

import (
	"context"
	"net/http"
	"testing"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

func TestTransfersRejectGhostForEveryRole(t *testing.T) {
	service := &Service{}
	for _, test := range []struct {
		role string
		code int
	}{
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
		{role: "super_admin", code: http.StatusBadRequest},
	} {
		user := platform.AuthUser{RoleKey: test.role, Permissions: []string{"transfer.request"}}
		_, err := service.Create(context.Background(), user, audit.LogEntry{}, CreateTransferRequest{
			SourceBranchID: "source", DestinationBranchID: "destination",
			Items: []TransferLine{{ProductID: "product", StockBucket: "ghost", Quantity: 1}},
		})
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}

func TestLegacyGhostTransferCannotDispatchOrReceive(t *testing.T) {
	lines := []TransferLine{{StockBucket: "ghost"}}
	for _, test := range []struct {
		role string
		code int
	}{{"branch_admin", http.StatusForbidden}, {"super_admin", http.StatusBadRequest}} {
		err := requireVisibleTransferLines(platform.AuthUser{RoleKey: test.role}, lines)
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}
