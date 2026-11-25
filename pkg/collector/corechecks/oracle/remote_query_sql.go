// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"strings"
	"unicode"
)

// remoteQueryIsSelect reports whether a query is a plain query (QUERY-02): after any
// leading whitespace, comments, and opening parentheses, its first word is SELECT or
// WITH, and a WITH clause does not start with a PL/SQL FUNCTION or PROCEDURE
// declaration.
//
// Oracle runs DDL even inside a read-only transaction, because DDL commits first, and
// inline PL/SQL can declare an autonomous transaction that writes. This check keeps
// both out; it does not parse the rest of the statement, which Oracle does.
func remoteQueryIsSelect(query string) bool {
	rest := skipSQLNoise(query, true)
	word, rest := sqlWord(rest)
	switch strings.ToUpper(word) {
	case "SELECT":
		return true
	case "WITH":
		after := skipSQLNoise(rest, false)
		if strings.HasPrefix(after, `"`) {
			return true // a quoted query name
		}
		next, _ := sqlWord(after)
		switch strings.ToUpper(next) {
		case "", "FUNCTION", "PROCEDURE":
			return false
		}
		return true
	}
	return false
}

// skipSQLNoise skips whitespace, `--` and `/* */` comments, and, when parens is set,
// opening parentheses.
func skipSQLNoise(s string, parens bool) string {
	for {
		trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
		switch {
		case strings.HasPrefix(trimmed, "--"):
			end := strings.IndexAny(trimmed, "\r\n")
			if end < 0 {
				return ""
			}
			s = trimmed[end:]
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed[2:], "*/")
			if end < 0 {
				return ""
			}
			s = trimmed[2+end+2:]
		case parens && strings.HasPrefix(trimmed, "("):
			s = trimmed[1:]
		default:
			return trimmed
		}
	}
}

// sqlWord returns the leading unquoted identifier of s and what follows it.
func sqlWord(s string) (string, string) {
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (end > 0 && ((c >= '0' && c <= '9') || c == '_' || c == '$' || c == '#')) {
			end++
			continue
		}
		break
	}
	return s[:end], s[end:]
}
