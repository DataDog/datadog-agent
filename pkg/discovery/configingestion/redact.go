// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import "regexp"

const sensitiveKeyword = `(?i)(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|requirepass|masterauth|credentials?|auth[_-]?token)`

// sensitiveRe matches sensitive key-value pairs in a single pass, covering:
//   - YAML/ini style:    key: value   or   key=value
//   - redis.conf style:  key value
//
// Using a single regex eliminates double-redaction. The word boundary (\b)
// prevents false positives on keys that merely contain a sensitive word
// (e.g. "notapassword"). The separator group ($2) is preserved so that the
// original formatting (colon, equals, space) is unchanged.
var sensitiveRe = regexp.MustCompile(
	`(?m)\b(` + sensitiveKeyword + `)(\s*[:=]\s*|\s+)(\S+)`,
)

func redactSensitive(content string) string {
	return sensitiveRe.ReplaceAllString(content, `$1$2[REDACTED]`)
}
