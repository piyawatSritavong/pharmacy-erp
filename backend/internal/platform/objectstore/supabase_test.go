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
	requestURI  string
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
		r.requestURI = req.RequestURI
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
	if rec.requestURI != "/storage/v1/object/product-images/a.jpg" {
		t.Fatalf("unexpected request line %s", rec.requestURI)
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
	if strings.Contains(rec.requestURI, "//") {
		t.Fatalf("request line has a doubled slash: %s", rec.requestURI)
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
	// A space is escaped; the separators are not.
	if rec.requestURI != "/storage/v1/object/product-images/catalog/2026/a%20b.jpg" {
		t.Fatalf("unexpected request line %q", rec.requestURI)
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

// The endpoint prefix is part of the API's shape and must reach the server as
// written. Escaping it turned "object/sign" into "object%2Fsign", which
// Supabase answers 404 for; the first version of these tests missed it because
// they read URL.Path, and net/http decodes %2F back to "/" before a handler
// sees it. RequestURI is the raw line.
func TestEndpointPrefixesAreNotEscaped(t *testing.T) {
	rec := &recorder{response: `{"signedURL":"/object/sign/product-images/a.jpg?token=t"}`}
	server := rec.server(t)
	defer server.Close()
	client := New(server.URL, "k", "product-images")

	for _, test := range []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "sign",
			call: func() error { _, err := client.SignedURL(context.Background(), "a.jpg", time.Minute); return err },
			want: "/storage/v1/object/sign/product-images/a.jpg",
		},
		{
			name: "info",
			call: func() error { _, err := client.Exists(context.Background(), "a.jpg"); return err },
			want: "/storage/v1/object/info/product-images/a.jpg",
		},
		{
			name: "delete",
			call: func() error { return client.Delete(context.Background(), "a.jpg") },
			want: "/storage/v1/object/product-images/a.jpg",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			if rec.requestURI != test.want {
				t.Fatalf("expected %s, got %s", test.want, rec.requestURI)
			}
			if strings.Contains(rec.requestURI, "%2F") || strings.Contains(rec.requestURI, "%2f") {
				t.Fatalf("a path separator was escaped: %s", rec.requestURI)
			}
		})
	}
}

// The database connection string sits beside the API URL in the Supabase
// dashboard and is the easier one to copy by mistake. Pasting it produced
// "unsupported protocol scheme \"postgresql\"" partway through an upload run,
// which names neither the variable at fault nor the value it wanted.
func TestADatabaseURLIsNotMistakenForTheAPIURL(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		ok   bool
	}{
		{name: "the project API url", url: "https://ref.supabase.co", ok: true},
		{name: "a trailing slash is fine", url: "https://ref.supabase.co/", ok: true},
		{name: "http for a local stack", url: "http://127.0.0.1:54321", ok: true},
		{name: "the pooler connection string", url: "postgresql://postgres.ref:pw@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres"},
		{name: "the direct connection string", url: "postgres://postgres:pw@db.ref.supabase.co:5432/postgres"},
		{name: "a bare host with no scheme", url: "ref.supabase.co"},
		{name: "empty", url: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateURL(test.url)
			if test.ok && err != nil {
				t.Fatalf("expected %q to be accepted, got %v", test.url, err)
			}
			if !test.ok && err == nil {
				t.Fatalf("expected %q to be refused", test.url)
			}
			// A refused URL must not yield a client that then fails obscurely.
			if client := New(test.url, "key", "bucket"); test.ok != client.Configured() {
				t.Fatalf("New disagreed with ValidateURL for %q", test.url)
			}
		})
	}
}

// The exact response Supabase's object/info endpoint gives for an object that
// is not there: HTTP 400, with the real answer in the body. Reading only the
// status turned "this object does not exist yet" — which is every object, the
// first time the migration runs against an empty bucket — into a hard failure
// that stopped the run on its first image.
const notFoundInTheBody = `{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}`

func TestNotFoundIsReadFromTheBodyNotOnlyTheStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound} {
		rec := &recorder{status: status, response: notFoundInTheBody}
		server := rec.server(t)
		client := New(server.URL, "k", "product-images")

		exists, err := client.Exists(context.Background(), "missing.jpg")
		if err != nil {
			t.Fatalf("status %d: a missing object must not be an error: %v", status, err)
		}
		if exists {
			t.Fatalf("status %d: expected the object to be reported absent", status)
		}
		if err := client.Delete(context.Background(), "missing.jpg"); err != nil {
			t.Fatalf("status %d: deleting a missing object must not be an error: %v", status, err)
		}
		if _, err := client.SignedURL(context.Background(), "missing.jpg", time.Minute); err != ErrNotFound {
			t.Fatalf("status %d: expected ErrNotFound, got %v", status, err)
		}
		server.Close()
	}
}

// A genuine failure that happens to carry a 400 must still be a failure — the
// body check must not swallow everything.
func TestAFourHundredThatIsNotANotFoundStaysAnError(t *testing.T) {
	rec := &recorder{status: http.StatusBadRequest, response: `{"statusCode":"400","error":"InvalidMimeType","message":"mime type not supported"}`}
	server := rec.server(t)
	defer server.Close()
	client := New(server.URL, "k", "product-images")

	exists, err := client.Exists(context.Background(), "a.jpg")
	if err == nil {
		t.Fatal("a real error must not be reported as a clean absence")
	}
	if exists {
		t.Fatal("nothing should be reported present")
	}
	if !strings.Contains(err.Error(), "mime type not supported") {
		t.Fatalf("the cause must survive: %v", err)
	}
}

// A rate limit partway through a several-hundred-image run should cost a pause,
// not the run.
func TestTransientFailuresAreRetried(t *testing.T) {
	var attempts int
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := New(server.URL, "k", "b").Upload(context.Background(), "a.jpg", []byte("payload"), "image/jpeg"); err != nil {
		t.Fatalf("upload should have survived the rate limit: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected three attempts, got %d", attempts)
	}
	// Every attempt must carry the whole body; a retry that sends an empty one
	// would upload a truncated image and report success.
	for i, body := range bodies {
		if body != "payload" {
			t.Fatalf("attempt %d sent %q instead of the payload", i+1, body)
		}
	}
}

// A request that is wrong will be wrong again; retrying it just delays the
// error and multiplies the load.
func TestClientErrorsAreNotRetried(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"InvalidMimeType"}`)
	}))
	defer server.Close()

	if err := New(server.URL, "k", "b").Upload(context.Background(), "a.jpg", []byte("x"), "image/tiff"); err == nil {
		t.Fatal("expected the error to surface")
	}
	if attempts != 1 {
		t.Fatalf("expected exactly one attempt, got %d", attempts)
	}
}

func TestRetriesGiveUpAndReportTheLastStatus(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `upstream is down`)
	}))
	defer server.Close()

	err := New(server.URL, "k", "b").Upload(context.Background(), "a.jpg", []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected the final status to be reported, got %v", err)
	}
	if attempts != maxAttempts {
		t.Fatalf("expected %d attempts, got %d", maxAttempts, attempts)
	}
}
