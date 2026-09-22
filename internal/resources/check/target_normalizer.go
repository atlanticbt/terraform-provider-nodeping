package check

import (
	"net/url"
	"strings"
)

// urlNormalizingCheckTypes are the check types whose target NodePing rewrites
// into a canonical URL before storing it. Membership was established by
// creating a check of every type in ValidCheckTypes against the live API and
// comparing the submitted target with the one read back; the types absent from
// this map either store the target verbatim (WEBSOCKET, FTP, RDAP, SIP, WHOIS
// accept a URL and do not touch it) or take a hostname or address rather than a
// URL. Guessing membership either way is a correctness bug: a type wrongly
// included makes the provider treat two distinct targets as one, and a type
// wrongly excluded brings back the phantom diff.
var urlNormalizingCheckTypes = map[string]bool{
	"AUDIO":       true,
	"HTTP":        true,
	"HTTPADV":     true,
	"HTTPCONTENT": true,
	"HTTPPARSE":   true,
	"MONGODB":     true,
	"PGSQL":       true,
	"REDIS":       true,
	"SSL":         true,
}

// normalizeTarget returns the target in the form NodePing stores it, so that a
// configured value and an API response can be compared for equivalence.
//
// Every check type has its target trimmed of surrounding whitespace. For the
// types in urlNormalizingCheckTypes the API additionally folds the scheme and
// host to lower case, gives an empty path a "/", and percent-encodes characters
// that are illegal in a path. It leaves everything else alone: the port is kept
// even when it is the default for the scheme, and duplicate slashes, dot
// segments, userinfo, query and fragment all survive untouched, so this does
// not resolve or canonicalise them either.
//
// Two observed behaviours are deliberately not reproduced here. The API
// punycodes a non-ASCII host ("https://例え.com" becomes
// "https://xn--r8jz45g.com/"), which would need an IDNA implementation; and its
// percent-encoding of non-ASCII path bytes has not been characterised. A target
// using either is normalised only as far as the rules above go, so the provider
// falls back to reporting a difference. That direction is the safe one: it
// shows a diff that settles after one apply rather than hiding a real change.
func normalizeTarget(checkType, target string) string {
	trimmed := strings.TrimSpace(target)

	if !urlNormalizingCheckTypes[strings.ToUpper(checkType)] {
		return trimmed
	}

	parsed, err := url.Parse(trimmed)
	// A relative reference has no scheme or host to fold and no path to root,
	// and a parse failure leaves nothing to work from. In both cases the
	// trimmed string is the most that can be claimed.
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	// Only Host is folded. Userinfo lives in parsed.User and is case sensitive.
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String()
}
