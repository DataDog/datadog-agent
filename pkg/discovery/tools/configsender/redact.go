// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import "regexp"

// Sensitive-key redaction is mandatory before bytes leave the host (DSCVR
// roadmap, Phase G: "Configs include passwords, API keys, cert paths.
// Needs to be redacted at the agent before bytes leave the host"). This
// pass is intentionally conservative: it errs on the side of redacting too
// much rather than too little. The implementation is line-based regex,
// which is not bulletproof against deeply nested keys or unusual quoting
// but covers the common shapes seen in redis.conf, yaml, json, and ini.

// keyword group used in both regexes below.
const sensitiveKeyword = `password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|requirepass|masterauth|credentials?|auth[_-]?token`

// redactSeparated matches lines of the form `<key>: <value>` or
// `<key>= <value>` where the key contains one of the sensitive keywords.
// It captures the key+separator (group 1) and the trailing comment region
// (group 3) so they can be re-emitted unchanged around `[REDACTED]`.
var redactSeparated = regexp.MustCompile(
	`(?im)^(\s*"?[\w.\-/]*(?:` + sensitiveKeyword + `)[\w.\-/]*"?\s*[:=]\s*)(\S[^#\n\r]*?)(\s*(?:#[^\n\r]*)?)$`,
)

// redactSpaced matches redis.conf-style `<key> <value>` lines where the
// key is one of the sensitive keywords (no nesting prefix because
// redis.conf is flat).
var redactSpaced = regexp.MustCompile(
	`(?im)^(\s*(?:` + sensitiveKeyword + `)[ \t]+)(\S[^#\n\r]*?)(\s*(?:#[^\n\r]*)?)$`,
)

// redactSensitive replaces values of keys that look like secrets with the
// literal string [REDACTED], preserving the key, separator, and any
// trailing comment.
func redactSensitive(raw []byte) []byte {
	out := redactSeparated.ReplaceAll(raw, []byte("${1}[REDACTED]${3}"))
	out = redactSpaced.ReplaceAll(out, []byte("${1}[REDACTED]${3}"))
	return out
}
