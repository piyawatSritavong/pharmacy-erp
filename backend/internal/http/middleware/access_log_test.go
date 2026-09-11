package middleware

import "testing"

func TestRedactQuery(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "the branch a request asked for is kept — it is the point of the log",
			in:   "branch_id=b-2&page=1",
			want: "branch_id=b-2&page=1",
		},
		{
			name: "a password never reaches stdout",
			in:   "email=a%40b.com&password=hunter2",
			want: "email=a%40b.com&password=%5BREDACTED%5D",
		},
		{
			name: "anything token-shaped goes too",
			in:   "access_token=abc&csrf_token=def&id=7",
			want: "access_token=%5BREDACTED%5D&csrf_token=%5BREDACTED%5D&id=7",
		},
		{
			name: "the parameter name survives so the shape of the call is still readable",
			in:   "api_key=xyz",
			want: "api_key=%5BREDACTED%5D",
		},
		{
			name: "every value of a repeated sensitive key is replaced",
			in:   "token=one&token=two",
			want: "token=%5BREDACTED%5D&token=%5BREDACTED%5D",
		},
		{name: "empty stays empty", in: "", want: ""},
		{
			name: "a query that will not parse is dropped whole rather than logged raw",
			in:   "%zz",
			want: redacted,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := redactQuery(test.in); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestSeverityFor(t *testing.T) {
	for status, want := range map[int]string{
		200: "INFO", 201: "INFO", 302: "INFO",
		400: "WARNING", 401: "WARNING", 403: "WARNING", 404: "WARNING", 499: "WARNING",
		500: "ERROR", 503: "ERROR",
	} {
		if got := severityFor(status); got != want {
			t.Fatalf("status %d: expected %s, got %s", status, want, got)
		}
	}
}
