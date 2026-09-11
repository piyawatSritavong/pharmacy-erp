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

// maxAttempts bounds the retries below. A migration run is hundreds of
// sequential requests, and one rate-limit response partway through should not
// end it — the command is idempotent, but making somebody re-run it for a
// hiccup is not the same as handling the hiccup.
const maxAttempts = 3

func (c *Client) do(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+c.key)
	// Supabase's gateway wants the key in both places; sending only the bearer
	// token works today and has broken on past gateway versions.
	request.Header.Set("apikey", c.key)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// The body was consumed by the previous attempt, so rewind it.
			// GetBody is set for us because every request here is built over a
			// bytes.Reader; a request without one is not retried.
			if request.Body != nil {
				if request.GetBody == nil {
					return nil, lastErr
				}
				body, err := request.GetBody()
				if err != nil {
					return nil, err
				}
				request.Body = body
			}
			select {
			case <-request.Context().Done():
				return nil, request.Context().Err()
			case <-time.After(time.Duration(attempt-1) * 500 * time.Millisecond):
			}
		}

		response, err := c.http.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		if !worthRetrying(response.StatusCode) || attempt == maxAttempts {
			return response, nil
		}
		// Drain and close so the connection can be reused for the retry.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		response.Body.Close()
		lastErr = fmt.Errorf("supabase storage: %s", response.Status)
	}
	return nil, lastErr
}

// worthRetrying covers the answers that mean "not now" rather than "no": rate
// limiting and the gateway being briefly unavailable. A 4xx about the request
// itself is not retried, because it will fail identically every time.
func worthRetrying(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// classify reads a failed response and says whether it means "no such object".
//
// Supabase does not signal that with an HTTP 404. The object/info endpoint
// answers 400 Bad Request and puts the real answer in the body:
//
//	{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}
//
// Reading only the HTTP status therefore turns the ordinary "this object is not
// here yet" — which is every object, the first time the migration runs against
// an empty bucket — into a hard failure.
func classify(response *http.Response) (body []byte, notFound bool) {
	body, _ = io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode == http.StatusNotFound {
		return body, true
	}
	var reported struct {
		StatusCode string `json:"statusCode"`
		Error      string `json:"error"`
		Code       string `json:"code"`
	}
	if json.Unmarshal(body, &reported) == nil {
		if reported.StatusCode == "404" || reported.Error == "not_found" || reported.Code == "NoSuchKey" {
			return body, true
		}
	}
	return body, false
}

// failure turns a non-2xx response into an error. It takes the body already
// read by classify rather than reading the response again: a response body can
// only be consumed once, and re-reading it yields nothing, which stripped the
// cause out of every error that had been classified first.
func failure(status string, body []byte) error {
	return fmt.Errorf("supabase storage: %s: %s", status, strings.TrimSpace(string(body)))
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
		body, notFound := classify(response)
		if notFound {
			return ErrNotFound
		}
		return failure(response.Status, body)
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
		body, notFound := classify(response)
		if notFound {
			return "", ErrNotFound
		}
		return "", failure(response.Status, body)
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
	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return nil
	}
	// Already gone satisfies the caller's intent either way.
	body, notFound := classify(response)
	if notFound {
		return nil
	}
	return failure(response.Status, body)
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
	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return true, nil
	}
	body, notFound := classify(response)
	if notFound {
		return false, nil
	}
	return false, failure(response.Status, body)
}

func ensureLeadingSlash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}
