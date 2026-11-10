// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import "regexp"

// scrubReplacement is the fixed placeholder substituted for a redacted value.
// It matches the convention already used by the Datadog Agent's process
// command-line scrubber (pkg/process/procutil/data_scrubber.go).
const scrubReplacement = "********"

// sensitiveKeyWords lists identifier fragments that, when found in a flag
// name or environment variable name, mark the value that follows as
// sensitive. Matching is substring-based (not whole-word) so that compound
// identifiers such as AWS_SECRET_ACCESS_KEY or DB_PASSWORD are still caught.
const sensitiveKeyWords = `(?:pass(?:word)?|pwd|secret|token|api[_-]?key|apikey|access[_-]?key|credential|private[_-]?key|privatekey|client[_-]?secret|session[_-]?id|sessionid)`

// valueAlt matches the value half of a key/value pair: a double-quoted
// string, a single-quoted string, or a bare token. The bare alternative
// stops before whitespace and before '@' so it cannot swallow the rest of a
// URL (e.g. the "@host/path" following a credential embedded in a URL like
// https://x-access-token:TOKEN@github.com/...). It also stops before '"',
// backslash, and single quote so it cannot swallow a shell-escaped quote (e.g. \" in a
// double-quoted argument) that immediately follows the value with no
// intervening whitespace — doing so would silently delete that quote
// character from the scrubbed output instead of merely redacting the value.
const valueAlt = `("[^"]*"|'[^']*'|[^\s@"'\\]+)`

var (
	// reURLCreds matches credentials embedded in a URL
	// (scheme://user:pass@host), redacting only the password half so the
	// username and host remain visible for debugging. This runs first so
	// that keyword-based regexes below don't mistake a URL username (e.g.
	// x-access-token) for a flag/env key and consume the rest of the URL.
	reURLCreds = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@]+):([^\s/@]+)@`)

	// reBasicAuthFlag matches curl/wget-style `-u user:pass` basic-auth
	// credentials. This is intentionally broader than the keyword list above
	// since "-u" carries no sensitive substring itself.
	reBasicAuthFlag = regexp.MustCompile(`(-u\s+)(\S+:\S+)`)

	// reAuthHeader matches an Authorization header value that names its
	// scheme (Bearer/Basic/Token), e.g. `Authorization: Bearer <jwt>`. The
	// value excludes quote characters so a trailing quote that closes the
	// enclosing shell argument (e.g. -H "Authorization: Bearer xyz") is left
	// in place rather than being swallowed into the redacted value.
	reAuthHeader = regexp.MustCompile(`(?i)(authorization['"]?\s*:\s*(?:bearer|basic|token)\s+)([^\s"']+)`)

	// reKeyEquals matches `key=value` or `key:value` where key contains a
	// sensitive keyword, covering env assignments (API_KEY=x), JSON-ish
	// fragments, and glued long-flag values (--password=x).
	reKeyEquals = regexp.MustCompile(`(?i)([\w-]*` + sensitiveKeyWords + `[\w-]*\s*[:=]\s*)` + valueAlt)

	// reFlagSpace matches a `-flag value` / `--flag value` pair where flag
	// contains a sensitive keyword and the value is a separate, space
	// delimited argument (--password hunter2). Restricted to arguments that
	// start with a dash so plain-English text is not mistaken for a flag.
	reFlagSpace = regexp.MustCompile(`(?i)(-{1,2}[\w-]*` + sensitiveKeyWords + `[\w-]*\s+)` + valueAlt)

	// reAWSAccessKey matches a bare AWS access key ID literal.
	reAWSAccessKey = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)

	// reJWT matches a bare JWT-shaped token: three dot-separated
	// base64url segments. This is the fallback for opaque bearer tokens that
	// appear without a preceding keyword or scheme.
	reJWT = regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
)

// redactQuotedValue replaces every match of re (which must have a prefix
// group at index 1 and a value group at index 2, matching valueAlt) with the
// prefix followed by the scrub placeholder, preserving the value's
// surrounding quote characters (if any) so the redacted output stays
// syntactically shaped like the input.
func redactQuotedValue(re *regexp.Regexp, text string) string {
	return re.ReplaceAllStringFunc(text, func(m string) string {
		sub := re.FindStringSubmatch(m)
		prefix, value := sub[1], sub[2]
		switch {
		case len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"':
			return prefix + `"` + scrubReplacement + `"`
		case len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'':
			return prefix + `'` + scrubReplacement + `'`
		default:
			return prefix + scrubReplacement
		}
	})
}

// scrubCommandText redacts common secret-shaped substrings from a raw shell
// command before it is attached to a telemetry tag. It is a best-effort,
// regex-based defense-in-depth measure, not a guarantee: it cannot catch
// every way a secret can appear (e.g. single-letter flags like mysql's
// glued `-pSECRET`, or secrets embedded in file contents the command
// references). Operators whose commands are too sensitive for any residual
// risk should use [DisableDetailedTelemetry] instead.
func scrubCommandText(text string) string {
	text = reURLCreds.ReplaceAllString(text, "${1}:"+scrubReplacement+"@")
	text = reBasicAuthFlag.ReplaceAllString(text, "${1}"+scrubReplacement)
	text = reAuthHeader.ReplaceAllString(text, "${1}"+scrubReplacement)
	text = redactQuotedValue(reKeyEquals, text)
	text = redactQuotedValue(reFlagSpace, text)
	text = reAWSAccessKey.ReplaceAllString(text, scrubReplacement)
	text = reJWT.ReplaceAllString(text, scrubReplacement)
	return text
}
