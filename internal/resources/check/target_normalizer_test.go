package check

import "testing"

// The cases below are transcriptions of live NodePing API responses. Each one
// was produced by creating a check in the test subaccount with the "in" target
// and reading back parameters.target. Nothing here is inferred: a rule the API
// was not observed to apply must not be asserted, because collapsing two
// targets the API keeps distinct would hide a real change.
func TestNormalizeTarget_MatchesObservedAPIBehaviour(t *testing.T) {
	cases := []struct {
		name      string
		checkType string
		in        string
		want      string
	}{
		{"bare origin gains a root path", "HTTP", "https://example.com", "https://example.com/"},
		{"an existing root path is unchanged", "HTTP", "https://example.com/", "https://example.com/"},
		{"a path without a trailing slash keeps its shape", "HTTP", "https://example.com/path", "https://example.com/path"},
		{"a path with a trailing slash keeps its shape", "HTTP", "https://example.com/path/", "https://example.com/path/"},
		{"host case is folded", "HTTP", "https://EXAMPLE.COM", "https://example.com/"},
		{"scheme case is folded", "HTTP", "HTTPS://example.com/path", "https://example.com/path"},
		{"path case is preserved", "HTTP", "https://EXAMPLE.com/Path", "https://example.com/Path"},
		{"a default port is kept", "HTTP", "https://example.com:443", "https://example.com:443/"},
		{"a non default port is kept", "HTTP", "https://example.com:8443", "https://example.com:8443/"},
		{"a query on a bare origin moves after the root path", "HTTP", "https://example.com?a=1", "https://example.com/?a=1"},
		{"query and fragment case are preserved", "HTTP", "https://example.com/P?a=B#frag", "https://example.com/P?a=B#frag"},
		{"userinfo is preserved", "HTTP", "https://user:pw@example.com", "https://user:pw@example.com/"},
		{"duplicate slashes are not collapsed", "HTTP", "https://example.com//dbl//slash", "https://example.com//dbl//slash"},
		{"dot segments are not resolved", "HTTP", "https://example.com/a/../b", "https://example.com/a/../b"},
		{"a space in the path is percent encoded", "HTTP", "https://example.com/sp ace", "https://example.com/sp%20ace"},
		{"surrounding whitespace is trimmed", "HTTP", "  https://example.com/sp  ", "https://example.com/sp"},
		{"mixed host and port case with a query", "HTTP", "https://Ex-Ample.COM:8443/A/b?Q=Z", "https://ex-ample.com:8443/A/b?Q=Z"},

		// The same URL rules apply to every other check type the API was
		// observed to normalise.
		{"SSL normalises like HTTP", "SSL", "https://EXAMPLE.COM/x", "https://example.com/x"},
		{"HTTPADV normalises like HTTP", "HTTPADV", "https://example.com", "https://example.com/"},
		{"HTTPCONTENT normalises like HTTP", "HTTPCONTENT", "https://example.com", "https://example.com/"},
		{"HTTPPARSE normalises like HTTP", "HTTPPARSE", "https://EXAMPLE.COM", "https://example.com/"},
		{"AUDIO normalises like HTTP", "AUDIO", "https://EXAMPLE.COM", "https://example.com/"},
		{"MONGODB normalises like HTTP", "MONGODB", "https://EXAMPLE.COM", "https://example.com/"},
		{"PGSQL normalises like HTTP", "PGSQL", "https://EXAMPLE.COM", "https://example.com/"},
		{"REDIS normalises like HTTP", "REDIS", "https://EXAMPLE.COM", "https://example.com/"},

		// These types accept a URL shaped target and store it verbatim. Applying
		// the URL rules to them would treat two distinct targets as one.
		{"WEBSOCKET is stored verbatim", "WEBSOCKET", "wss://EXAMPLE.COM", "wss://EXAMPLE.COM"},
		{"WEBSOCKET does not gain a root path", "WEBSOCKET", "https://example.com", "https://example.com"},
		{"FTP is stored verbatim", "FTP", "https://EXAMPLE.COM", "https://EXAMPLE.COM"},
		{"RDAP is stored verbatim", "RDAP", "https://EXAMPLE.COM", "https://EXAMPLE.COM"},
		{"SIP is stored verbatim", "SIP", "https://EXAMPLE.COM", "https://EXAMPLE.COM"},
		{"WHOIS is stored verbatim", "WHOIS", "https://EXAMPLE.COM", "https://EXAMPLE.COM"},

		// Hostname and address types keep their case; only whitespace is trimmed.
		{"PING keeps host case", "PING", "EXAMPLE.COM", "EXAMPLE.COM"},
		{"PING is trimmed", "PING", "  example.com  ", "example.com"},
		{"PORT keeps host case", "PORT", "EXAMPLE.com", "EXAMPLE.com"},
		{"PORT is trimmed", "PORT", "  example.com  ", "example.com"},
		{"DNS keeps host case", "DNS", "EXAMPLE.COM", "EXAMPLE.COM"},
		{"RBL keeps host case", "RBL", "EXAMPLE.com", "EXAMPLE.com"},
		{"SMTP keeps host case", "SMTP", "EXAMPLE.com", "EXAMPLE.com"},
		{"IMAP4 keeps host case", "IMAP4", "EXAMPLE.com", "EXAMPLE.com"},

		// Boundary and malformed input. A target the provider cannot parse is
		// returned trimmed rather than rejected: validation is the API's job and
		// a parse failure must not make two different strings compare equal.
		{"an empty target stays empty", "HTTP", "", ""},
		{"whitespace only collapses to empty", "HTTP", "   ", ""},
		{"a schemeless target is left alone", "HTTP", "example.com", "example.com"},
		{"a target with no host is left alone", "HTTP", "https://", "https://"},
		{"an unparseable target is left alone", "HTTP", "https://exa mple.com\x7f", "https://exa mple.com\x7f"},
		{"an unknown check type is treated as non normalising", "NOTATYPE", "https://EXAMPLE.COM", "https://EXAMPLE.COM"},
		{"the check type is matched case insensitively", "http", "https://EXAMPLE.COM", "https://example.com/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTarget(tc.checkType, tc.in); got != tc.want {
				t.Errorf("normalizeTarget(%q, %q) = %q, want %q", tc.checkType, tc.in, got, tc.want)
			}
		})
	}
}

// normalizeTarget is only ever used to compare two targets, so the property
// that matters is that it is stable: feeding it its own output must not move
// the value again, or a refresh could oscillate.
func TestNormalizeTarget_IsIdempotent(t *testing.T) {
	inputs := []struct {
		checkType string
		target    string
	}{
		{"HTTP", "https://EXAMPLE.COM"},
		{"HTTP", "https://example.com/sp ace"},
		{"HTTP", "  https://example.com?a=1  "},
		{"SSL", "https://EXAMPLE.COM/x"},
		{"PING", "  EXAMPLE.COM  "},
		{"WEBSOCKET", "wss://EXAMPLE.COM"},
	}

	for _, in := range inputs {
		once := normalizeTarget(in.checkType, in.target)
		twice := normalizeTarget(in.checkType, once)
		if once != twice {
			t.Errorf("normalizeTarget(%q, %q) is not idempotent: %q then %q", in.checkType, in.target, once, twice)
		}
	}
}

// Targets that genuinely differ must never compare equal, whatever the type.
func TestNormalizeTarget_KeepsDistinctTargetsDistinct(t *testing.T) {
	pairs := []struct {
		checkType string
		a, b      string
	}{
		{"HTTP", "https://example.com/a", "https://example.com/b"},
		{"HTTP", "https://example.com/path", "https://example.com/path/"},
		{"HTTP", "https://example.com", "http://example.com"},
		{"HTTP", "https://example.com", "https://example.com:443"},
		{"HTTP", "https://example.com/?a=1", "https://example.com/?a=2"},
		{"HTTP", "https://example.com/a/../b", "https://example.com/b"},
		{"PING", "example.com", "EXAMPLE.COM"},
		{"WEBSOCKET", "wss://example.com", "wss://example.com/"},
	}

	for _, p := range pairs {
		if normalizeTarget(p.checkType, p.a) == normalizeTarget(p.checkType, p.b) {
			t.Errorf("normalizeTarget conflated %s targets %q and %q", p.checkType, p.a, p.b)
		}
	}
}
