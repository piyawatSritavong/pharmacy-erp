package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// One line of JSON per request on stdout, which is where Cloud Run collects it.
//
// Until now the only request-level output in the whole service was a plain-text
// line for 5xx, carrying the method and path and not the query string. That is
// why, when two endpoints turned out to be handing another shop's rows to
// anyone who asked for them by id, there was no record to go back to: nothing
// stored the branch a request asked for, and nothing stored the branch it
// ended up acting on. Both are fields here.
type accessLogEntry struct {
	// Cloud Logging reads "severity" and "time" off the JSON payload and lifts
	// them onto the log entry itself; everything else stays as structured
	// fields you can filter on.
	Severity  string `json:"severity"`
	Time      string `json:"time"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`

	Method    string `json:"method"`
	Path      string `json:"path"`
	Query     string `json:"query,omitempty"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Bytes     int64  `json:"bytes_out"`

	UserID string `json:"user_id,omitempty"`
	Email  string `json:"user_email,omitempty"`
	Role   string `json:"role_key,omitempty"`

	// TokenBranch is the branch on the caller's token. EffectiveBranch is what
	// the branch rule settled on after clamping — "*" where a global caller was
	// given every branch, several ids where one request resolved more than
	// once, and empty where the request never asked about a branch at all.
	TokenBranch     string `json:"token_branch,omitempty"`
	EffectiveBranch string `json:"effective_branch,omitempty"`
	BranchRefused   bool   `json:"branch_refused,omitempty"`

	RemoteIP  string `json:"remote_ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Anything whose name looks like a credential is replaced rather than logged.
// The check is on the parameter name and is substring-based, so token,
// access_token and csrf_token are all caught by "token".
var redactedParams = []string{"password", "passwd", "token", "secret", "authorization", "api_key", "apikey", "credential", "signature"}

const redacted = "[REDACTED]"

// stdout is serialised so two requests finishing together cannot interleave
// half a line each and produce JSON that nothing can parse.
var logMutex sync.Mutex

// AccessLog writes one structured line per request. It is registered before
// the JWT middleware so that it also covers the requests that never get a
// user — 401s included — and reads the user afterwards, by which time the JWT
// middleware has set it.
func AccessLog() echo.MiddlewareFunc {
	encoder := json.NewEncoder(os.Stdout)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Attached before the handler runs so the branch rule has somewhere
			// to record what it decided.
			branchAudit := platform.NewBranchAudit()
			c.Set(platform.ContextBranchAuditKey, branchAudit)

			err := next(c)
			if err != nil {
				// Echo's own error handler has not run yet, so the response
				// status is not final. Letting it run first keeps the logged
				// status equal to what the client was actually sent.
				c.Error(err)
			}

			request := c.Request()
			response := c.Response()
			entry := accessLogEntry{
				Severity:  severityFor(response.Status),
				Time:      start.UTC().Format(time.RFC3339Nano),
				Message:   request.Method + " " + request.URL.Path,
				RequestID: requestIDOf(c),
				Method:    request.Method,
				Path:      request.URL.Path,
				Query:     redactQuery(request.URL.RawQuery),
				Status:    response.Status,
				LatencyMS: time.Since(start).Milliseconds(),
				Bytes:     response.Size,
				RemoteIP:  c.RealIP(),
				UserAgent: request.UserAgent(),
			}

			user := platform.CurrentUser(c)
			entry.UserID = user.ID
			entry.Email = user.Email
			entry.Role = user.RoleKey
			if user.BranchID != nil {
				entry.TokenBranch = strings.TrimSpace(*user.BranchID)
			}
			resolved, refused := branchAudit.Resolved()
			entry.EffectiveBranch = strings.Join(resolved, ",")
			entry.BranchRefused = refused

			if cause, ok := c.Get(platform.ContextServerErrorKey).(string); ok {
				entry.Error = cause
			}

			logMutex.Lock()
			_ = encoder.Encode(entry)
			logMutex.Unlock()

			// c.Error above has already written the response, so returning the
			// error again would have Echo handle it twice.
			return nil
		}
	}
}

func severityFor(status int) string {
	switch {
	case status >= http.StatusInternalServerError:
		return "ERROR"
	case status >= http.StatusBadRequest:
		return "WARNING"
	default:
		return "INFO"
	}
}

func requestIDOf(c echo.Context) string {
	if id, ok := c.Get(platform.ContextRequestIDKey).(string); ok && id != "" {
		return id
	}
	return c.Response().Header().Get(echo.HeaderXRequestID)
}

// redactQuery keeps the shape of the query string — which parameters were sent
// is the part an audit needs — while replacing the value of anything that reads
// like a credential. A query string that will not parse is dropped whole rather
// than logged unexamined.
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return redacted
	}
	for key := range values {
		if isSensitiveParam(key) {
			for index := range values[key] {
				values[key][index] = redacted
			}
		}
	}
	return values.Encode()
}

func isSensitiveParam(key string) bool {
	lowered := strings.ToLower(key)
	for _, needle := range redactedParams {
		if strings.Contains(lowered, needle) {
			return true
		}
	}
	return false
}
