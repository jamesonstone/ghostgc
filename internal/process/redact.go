package process

import (
	"net/url"
	"strings"
)

// Redacted is the placeholder substituted for any value judged sensitive.
const Redacted = "[redacted]"

// sensitiveNameParts are matched against the dash/underscore/dot-separated
// parts of a flag or variable name. Matching on whole parts rather than
// substrings avoids redacting "--design-file" because it contains "sig".
var sensitiveNameParts = map[string]bool{
	"key":            true,
	"keys":           true,
	"apikey":         true,
	"apikeys":        true,
	"token":          true,
	"tokens":         true,
	"secret":         true,
	"secrets":        true,
	"password":       true,
	"passwd":         true,
	"pass":           true,
	"pwd":            true,
	"credential":     true,
	"credentials":    true,
	"creds":          true,
	"auth":           true,
	"authorization":  true,
	"bearer":         true,
	"cookie":         true,
	"cookies":        true,
	"signature":      true,
	"sig":            true,
	"dsn":            true,
	"passphrase":     true,
	"privatekey":     true,
	"jwt":            true,
	"authentication": true,
}

// sensitiveNameSubstrings catch names that do not split cleanly, such as
// "MYAPIKEY" or "dbpassword".
var sensitiveNameSubstrings = []string{
	"apikey", "api_key", "accesskey", "access_key", "secretkey", "secret_key",
	"password", "passwd", "authtoken", "auth_token", "accesstoken",
	"access_token", "refreshtoken", "refresh_token", "privatekey",
	"private_key", "clientsecret", "client_secret", "connectionstring",
	"connection_string", "sessiontoken", "session_token", "sessionkey",
	"session_key", "sessionsecret", "session_secret",
}

// secretValuePrefixes identify values that are self-evidently credentials
// regardless of the flag they were passed with.
var secretValuePrefixes = []string{
	"sk-", "sk_live_", "sk_test_", "rk_live_", "pk_live_",
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_",
	"xoxb-", "xoxp-", "xoxa-", "xoxs-", "xapp-",
	"glpat-", "npm_", "dop_v1_", "doo_v1_", "sq0atp-", "sq0csp-",
	"AKIA", "ASIA", "AIza", "ya29.", "eyJ", "hf_", "sbp_", "shpat_",
	"anthropic-", "Bearer ", "Basic ", "Token ",
}

// sensitiveQueryParams are redacted when they appear in a URL query string,
// which is how presigned URLs leak.
var sensitiveQueryParams = map[string]bool{
	"sig": true, "signature": true, "token": true, "access_token": true,
	"key": true, "apikey": true, "api_key": true, "password": true,
	"x-amz-signature": true, "x-amz-security-token": true, "x-amz-credential": true,
	"se": true, "sp": true, "sv": true, "sr": true, "st": true,
}

// SensitiveName reports whether a flag or environment variable name suggests
// that its value is a credential.
func SensitiveName(name string) bool {
	n := strings.ToLower(strings.TrimLeft(name, "-"))
	if n == "" {
		return false
	}
	for _, sub := range sensitiveNameSubstrings {
		if strings.Contains(n, sub) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(n, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ':'
	}) {
		if sensitiveNameParts[part] {
			return true
		}
	}
	return false
}

// SensitiveValue reports whether a value looks like a credential on its own.
func SensitiveValue(v string) bool {
	for _, p := range secretValuePrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// RedactValue returns a value safe to persist, replacing it wholesale when it
// looks like a credential and rewriting URLs to strip embedded secrets.
func RedactValue(v string) string {
	if v == "" {
		return v
	}
	if SensitiveValue(v) {
		return Redacted
	}
	if looksLikeURL(v) {
		return redactURL(v)
	}
	return v
}

func looksLikeURL(v string) bool {
	i := strings.Index(v, "://")
	return i > 0 && i < 16
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// Unparseable but URL-shaped: refuse to guess, redact the whole thing.
		return Redacted
	}
	changed := false
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), Redacted)
			changed = true
		}
	}
	if q := u.RawQuery; q != "" {
		values, err := url.ParseQuery(q)
		if err != nil {
			return Redacted
		}
		for k, vs := range values {
			if !sensitiveQueryParams[strings.ToLower(k)] && !SensitiveName(k) {
				continue
			}
			for i := range vs {
				vs[i] = Redacted
			}
			values[k] = vs
			changed = true
		}
		if changed {
			u.RawQuery = values.Encode()
		}
	}
	if !changed {
		return raw
	}
	return u.String()
}

// RedactArgs returns a copy of a command line with credential-bearing
// arguments replaced. It is applied before any command line is written to
// storage or to a log line.
//
// The rules are deliberately conservative in the direction of redacting too
// much: an over-redacted command line costs a little explanatory power, an
// under-redacted one writes a live credential to disk.
func RedactArgs(args []string) []string {
	if args == nil {
		return nil
	}
	out := make([]string, len(args))
	redactNext := false
	for i, a := range args {
		switch {
		case redactNext:
			redactNext = false
			if strings.HasPrefix(a, "-") {
				// The sensitive flag took no value; do not eat the next flag.
				out[i] = a
				continue
			}
			out[i] = Redacted
			continue
		case strings.HasPrefix(a, "-"):
			name, value, hasValue := strings.Cut(a, "=")
			if hasValue {
				if SensitiveName(name) {
					out[i] = name + "=" + Redacted
				} else {
					out[i] = name + "=" + RedactValue(value)
				}
				continue
			}
			out[i] = a
			if SensitiveName(a) && i+1 < len(args) {
				redactNext = true
			}
			continue
		}

		// Bare argument. It may still be NAME=VALUE (an inline environment
		// assignment), a URL, or a naked credential.
		if name, value, ok := strings.Cut(a, "="); ok && name != "" && !strings.ContainsAny(name, " /") {
			if SensitiveName(name) {
				out[i] = name + "=" + Redacted
			} else {
				out[i] = name + "=" + RedactValue(value)
			}
			continue
		}
		out[i] = RedactValue(a)
	}
	return out
}

// EnvPolicy controls which environment variables may leave process memory.
type EnvPolicy struct {
	// Allow lists the exact variable names that may be persisted. Every other
	// variable is dropped, not redacted: ghostgc has no reason to keep a
	// record that an unrelated variable existed.
	Allow []string
	// RedactValues drops the values of allowlisted variables whose names look
	// sensitive, keeping only the fact that the variable was set.
	RedactValues bool
}

// RedactEnv reduces an environment to the allowlisted subset that is safe to
// persist. The returned map never aliases the input.
func RedactEnv(env map[string]string, policy EnvPolicy) map[string]string {
	if len(env) == 0 || len(policy.Allow) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, name := range policy.Allow {
		v, ok := env[name]
		if !ok {
			continue
		}
		switch {
		case policy.RedactValues && SensitiveName(name):
			out[name] = Redacted
		case policy.RedactValues:
			out[name] = RedactValue(v)
		default:
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
