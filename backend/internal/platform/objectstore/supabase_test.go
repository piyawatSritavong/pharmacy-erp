package objectstore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A stand-in for the Storage API that records what it was asked, so the tests
// assert on the request the client actually sends rather than on a mock that
// was written to agree with it.
type recorder struct {
	method      string
	path        string
	auth        string
	apikey      string
	contentType string
	upsert      string
	body        []byte
	status      int
	response    string
}

func (r *recorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.method = req.Method
		r.path = req.URL.Path
		r.auth = req.Header.Get("Authorization")
		r.apikey = req.Header.Get("apikey")
		r.contentType = req.Header.Get("Content-Type")
		r.upsert = req.Header.Get("x-upsert")
		r.body, _ = io.ReadAll(req.Body)
		if r.status == 0 {
			r.status = http.StatusOK
		}
		w.WriteHeader(r.status)
		_, _ = io.WriteString(w, r.response)
	}))
}

func TestUploadSendsTheObjectWithServiceCredentials(t *testing.T) {
	rec := &recorder{response: `{"Key":"product-images/a.jpg"}`}
	server := rec.server(t)
	defer server.Close()

	client := New(server.URL, "service-key", "product-images")
	if err := client.Upload(context.Background(), "a.jpg", []byte("bytes"), "image/jpeg"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	if rec.method != http.MethodPost {
		t.Fatalf("expected POST, got %s", rec.method)
	}
	if rec.path != "/storage/v1/object/product-images/a.jpg" {
		t.Fatalf("unexpected path %s", rec.path)
	}
	if rec.auth != "Bearer service-key" || rec.apikey != "service-key" {
		t.Fatalf("credentials not sent: auth=%q apikey=%q", rec.auth, rec.apikey)
	}
	if rec.contentType != "image/jpeg" {
		t.Fatalf("expected the image content type, got %q", rec.contentType)
	}
	// Re-running the seed or the migration must converge, not collide.
	if rec.upsert != "true" {
		t.Fatalf("expected x-upsert true, got %q", rec.upsert)
	}
	if string(rec.body) != "bytes" {
		t.Fatalf("body not sent verbatim: %q", rec.body)
	}
}

func TestSignedURLIsAbsoluteAndCarriesTheExpiry(t *testing.T) {
	rec := &recorder{response: `{"signedURL":"/object/sign/product-images/a.jpg?token=abc"}`}
	server := rec.server(t)
	defer server.Close()

	client := New(server.URL, "service-key", "product-images")
	signed, err := client.SignedURL(context.Background(), "a.jpg", 90*time.Second)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(rec.body, &sent); err != nil {
		t.Fatalf("request body was not json: %v", err)
	}
	if sent["expiresIn"] != float64(90) {
		t.Fatalf("expected expiresIn 90, got %v", sent["expiresIn"])
	}
	// The API answers with a path relative to /storage/v1; a caller redirecting
	// a browser to that would send it to our own host and 404.
	want := server.URL + "/storage/v1/object/sign/product-images/a.jpg?token=abc"
	if signed != want {
		t.Fatalf("expected %s, got %s", want, signed)
	}
}

func TestSignedURLDefaultsToTheShortExpiry(t *testing.T) {
	rec := &recorder{response: `{"signedURL":"/object/sign/product-images/a.jpg?token=abc"}`}
	server := rec.server(t)
	defer server.Close()

	if _, err := New(server.URL, "k", "product-images").SignedURL(context.Background(), "a.jpg", 0); err != nil {
		t.Fatalf("sign: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(rec.body, &sent)
	if sent["expiresIn"] != float64(SignedURLTTL.Seconds()) {
		t.Fatalf("expected the default ttl, got %v", sent["expiresIn"])
	}
}

func TestAnEmptySignedURLIsAnError(t *testing.T) {
	rec := &recorder{response: `{"signedURL":""}`}
	server := rec.server(t)
	defer server.Close()
	if _, err := New(server.URL, "k", "b").SignedURL(context.Background(), "a.jpg", time.Minute); err == nil {
		t.Fatal("an empty signed url must not be handed back as a usable one")
	}
}

func TestMissingObjectsAreDistinguishable(t *testing.T) {
	rec := &recorder{status: http.StatusNotFound, response: `{"error":"not_found"}`}
	server := rec.server(t)
	defer server.Close()
	client := New(server.URL, "k", "b")

	if _, err := client.SignedURL(context.Background(), "gone.jpg", time.Minute); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	exists, err := client.Exists(context.Background(), "gone.jpg")
	if err != nil || exists {
		t.Fatalf("expected a clean false, got %v %v", exists, err)
	}
	// Deleting what is already absent satisfies the caller's intent.
	if err := client.Delete(context.Background(), "gone.jpg"); err != nil {
		t.Fatalf("deleting a missing object must not be an error: %v", err)
	}
}

func TestAnUnconfiguredClientSaysSoRatherThanPanicking(t *testing.T) {
	for _, client := range []*Client{
		New("", "key", "bucket"),
		New("https://x.supabase.co", "", "bucket"),
		New("   ", "   ", "bucket"),
	} {
		if client.Configured() {
			t.Fatal("missing credentials must not produce a configured client")
		}
		if err := client.Upload(context.Background(), "a.jpg", nil, "image/jpeg"); err != ErrNotConfigured {
			t.Fatalf("expected ErrNotConfigured, got %v", err)
		}
		if _, err := client.SignedURL(context.Background(), "a.jpg", 0); err != ErrNotConfigured {
			t.Fatalf("expected ErrNotConfigured, got %v", err)
		}
		if err := client.Delete(context.Background(), "a.jpg"); err != ErrNotConfigured {
			t.Fatalf("expected ErrNotConfigured, got %v", err)
		}
	}
}

func TestATrailingSlashOnTheProjectURLDoesNotDoubleUp(t *testing.T) {
	rec := &recorder{response: `{}`}
	server := rec.server(t)
	defer server.Close()
	if err := New(server.URL+"/", "k", "product-images").Upload(context.Background(), "a.jpg", []byte("x"), "image/jpeg"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if strings.Contains(rec.path, "//") {
		t.Fatalf("path has a doubled slash: %s", rec.path)
	}
}

func TestNestedObjectPathsKeepTheirSeparators(t *testing.T) {
	rec := &recorder{response: `{}`}
	server := rec.server(t)
	defer server.Close()
	if err := New(server.URL, "k", "product-images").Upload(context.Background(), "catalog/2026/a b.jpg", []byte("x"), "image/jpeg"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	// Slashes stay separators; everything else is escaped.
	if rec.path != "/storage/v1/object/product-images/catalog/2026/a b.jpg" {
		t.Fatalf("unexpected path %q", rec.path)
	}
}

func TestAServerErrorIsReportedWithItsCause(t *testing.T) {
	rec := &recorder{status: http.StatusInsufficientStorage, response: `{"error":"quota exceeded"}`}
	server := rec.server(t)
	defer server.Close()
	err := New(server.URL, "k", "b").Upload(context.Background(), "a.jpg", []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected the cause to survive, got %v", err)
	}
}
