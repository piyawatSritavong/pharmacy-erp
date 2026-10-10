package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/platform"

	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
)

func TestJWTRejectsRevokedAndInactiveSessionAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.OpenTest(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var roleID string
	if err := db.QueryRow(`SELECT id::text FROM roles WHERE role_key='super_admin'`).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	userID := platform.MustUUID()
	if _, err := db.Exec(`
		INSERT INTO users (id,role_id,full_name,email,password_hash,active,auth_version,created_at,updated_at)
		VALUES ($1,$2,'Session test',$3,'unused',TRUE,1,NOW(),NOW())
	`, userID, roleID, "session-"+userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, userID) })

	secret := "integration-session-secret-at-least-32-chars"
	signed := func(version int) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
			UserID: userID, AuthVersion: version,
			StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()},
		})
		raw, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	request := func(rawToken string) int {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+rawToken)
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)
		handler := JWT(secret, db)(func(c echo.Context) error {
			if platform.CurrentUser(c).ID != userID {
				t.Fatal("middleware did not load current database user")
			}
			return c.NoContent(http.StatusNoContent)
		})
		_ = handler(ctx)
		return rec.Code
	}

	versionOne := signed(1)
	if status := request(versionOne); status != http.StatusNoContent {
		t.Fatalf("fresh session status=%d", status)
	}
	if _, err := db.Exec(`UPDATE users SET auth_version=2 WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if status := request(versionOne); status != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d", status)
	}
	versionTwo := signed(2)
	if status := request(versionTwo); status != http.StatusNoContent {
		t.Fatalf("new session status=%d", status)
	}
	if _, err := db.Exec(`UPDATE users SET active=FALSE WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if status := request(versionTwo); status != http.StatusUnauthorized {
		t.Fatalf("inactive account session status=%d", status)
	}
}
