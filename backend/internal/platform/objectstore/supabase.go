// Package objectstore keeps product photographs in Supabase Storage.
//
// They used to be files under UPLOAD_DIR. That works on one long-lived box and
// not at all on a platform that rebuilds the container on every deploy: the
// rows in product_images survive, the files do not, and the gallery starts
// returning 404 for pictures the database still believes in. Nothing announces
// it — it reads like a bug in the image feature.
//
// The bucket is private. Nothing here ever hands out a durable URL: reads go
// through a signed URL minted per request with a short expiry, so an object
// path is not a credential and a path that leaks grants nothing on its own.
package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SignedURLTTL is how long a minted read URL stays valid. Long enough for a
// browser to follow the redirect and fetch the bytes, short enough that a URL
// captured from a log or a referrer header is useless by the time anyone reads
// it.
const SignedURLTTL = 5 * time.Minute

// ErrNotConfigured is returned when no Supabase credentials are present. It is
// distinguishable on purpose: development without storage should say so rather
// than fail as though the file were missing.
var ErrNotConfigured = errors.New("supabase storage is not configured")

// ErrNotFound is returned when the bucket has no object at that path.
var ErrNotFound = errors.New("object not found")

type Client struct {
	baseURL string
	key     string
	bucket  string
	http    *http.Client
}

// ErrNotAnAPIURL is returned when SUPABASE_URL holds something that is not the
// project's REST endpoint — most often the database connection string, which
// sits a few lines away from it in the Supabase dashboard and is the easier of
// the two to copy by mistake.
var ErrNotAnAPIURL = errors.New("SUPABASE_URL must be the project API URL (https://<ref>.supabase.co), not a database connection string")

// ValidateURL reports whether a configured SUPABASE_URL can address the Storage
// API at all. It exists so the mistake is caught once, at startup, rather than
// surfacing as "unsupported protocol scheme" partway through an upload run.
func ValidateURL(supabaseURL string) error {
	supabaseURL = strings.TrimSpace(supabaseURL)
	if supabaseURL == "" {
		return ErrNotConfigured
	}
	parsed, err := url.Parse(supabaseURL)
	if err != nil {
		return ErrNotAnAPIURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrNotAnAPIURL
	}
	if parsed.Host == "" || parsed.User != nil {
		return ErrNotAnAPIURL
	}
	return nil
}

// New returns a client, or nil when the credentials are absent or the URL is
// not an API URL. A nil *Client is usable: every method returns
// ErrNotConfigured, so callers do not need a separate "is storage on" flag
// threaded through them.
func New(supabaseURL, serviceRoleKey, bucket string) *Client {
	supabaseURL = strings.TrimRight(strings.TrimSpace(supabaseURL), "/")
	serviceRoleKey = strings.TrimSpace(serviceRoleKey)
	if serviceRoleKey == "" || ValidateURL(supabaseURL) != nil {
		return nil
	}
	return &Client{
		baseURL: supabaseURL,
		key:     serviceRoleKey,
		bucket:  bucket,
		// Generous enough for a 10 MB upload on a slow connection, bounded so a
		// stalled storage backend cannot hold a request handler open forever.
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Configured reports whether uploads and reads can happen at all.
func (c *Client) Configured() bool { return c != nil }

// objectURL builds a URL for one object.
//
// The endpoint prefix ("object", "object/sign", "object/info") is a fixed part
// of the API's shape and goes in as written. Only the bucket and the object
// path are escaped, segment by segment, so a "/" inside a key stays a path
// separator while everything else is encoded.
//
// Escaping the prefix too — which is what this did at first — turns
// "object/sign" into "object%2Fsign", and Supabase answers 404 for an endpoint
// that does not exist. It survived the tests because they asserted on
// http.Request.URL.Path, which decodes %2F back to "/" before you see it; the
// tests now read RequestURI, which is the raw request line.
func (c *Client) objectURL(prefix, objectPath string) string {
	segments := []string{url.PathEscape(c.bucket)}
	for _, part := range strings.Split(strings.TrimPrefix(objectPath, "/"), "/") {
		segments = append(segments, url.PathEscape(part))
	}
	return c.baseURL + "/storage/v1/" + prefix + "/" + strings.Join(segments, "/")
}

func (c *Client) do(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+c.key)
	// Supabase's gateway wants the key in both places; sending only the bearer
	// token works today and has broken on past gateway versions.
	request.Header.Set("apikey", c.key)
	return c.http.Do(request)
}

// readError turns a non-2xx storage response into an error carrying enough to
// diagnose it, without letting a multi-megabyte body into the log.
func readError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("supabase storage: %s: %s", response.Status, strings.TrimSpace(string(body)))
}

// Upload writes an object, replacing anything already at that path. Upsert is
// deliberate: re-running the seed or the one-off migration has to converge
// rather than fail on what it wrote last time.
func (c *Client) Upload(ctx context.Context, objectPath string, content []byte, contentType string) error {
	if c == nil {
		return ErrNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.objectURL("object", objectPath), bytes.NewReader(content))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("x-upsert", "true")
	request.ContentLength = int64(len(content))

	response, err := c.do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return readError(response)
	}
	return nil
}

// SignedURL mints a short-lived absolute URL for reading one object.
func (c *Client) SignedURL(ctx context.Context, objectPath string, ttl time.Duration) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	if ttl <= 0 {
		ttl = SignedURLTTL
	}
	payload, err := json.Marshal(map[string]any{"expiresIn": int(ttl.Seconds())})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.objectURL("object/sign", objectPath), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", readError(response)
	}

	var signed struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(response.Body).Decode(&signed); err != nil {
		return "", err
	}
	if strings.TrimSpace(signed.SignedURL) == "" {
		return "", errors.New("supabase storage: signed url was empty")
	}
	// The API answers with a path relative to /storage/v1, not an absolute URL.
	return c.baseURL + "/storage/v1" + ensureLeadingSlash(signed.SignedURL), nil
}

// Delete removes an object. An object that is already gone is not an error:
// the caller's intent — "this should not exist" — is satisfied either way.
func (c *Client) Delete(ctx context.Context, objectPath string) error {
	if c == nil {
		return ErrNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.objectURL("object", objectPath), nil)
	if err != nil {
		return err
	}
	response, err := c.do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return readError(response)
	}
	return nil
}

// Exists reports whether the bucket holds an object at that path. The migration
// uses it to skip what it has already uploaded.
func (c *Client) Exists(ctx context.Context, objectPath string) (bool, error) {
	if c == nil {
		return false, ErrNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL("object/info", objectPath), nil)
	if err != nil {
		return false, err
	}
	response, err := c.do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return false, readError(response)
	}
	return true, nil
}

func ensureLeadingSlash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}
