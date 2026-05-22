// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "strings"

// cleanValue normalises a raw value captured by the regex or fuzzy tiers
func cleanValue(s string) string {
	s = strings.TrimRight(s, "\r")
	s = strings.TrimSpace(s)
	s = stripAnchor(s)
	return stripQuotes(s)
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// stripAnchor removes a leading YAML anchor (&name) or alias (*name) followed by whitespace.
func stripAnchor(s string) string {
	if len(s) < 2 || (s[0] != '&' && s[0] != '*') {
		return s
	}
	i := 1
	for i < len(s) {
		b := s[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-') {
			break
		}
		i++
	}
	if i == 1 || i >= len(s) || (s[i] != ' ' && s[i] != '\t') {
		return s
	}
	return strings.TrimLeft(s[i:], " \t")
}

// stripSeparators removes underscores and hyphens so "apikey", "api-key" and
// "api_key" collapse to the same shape for fuzzy matching.
func stripSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '_' && c != '-' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// parseLine splits a single config line into key/value. Empty, indented and
// commented lines are rejected so the fuzzy tier never matches inside nested
// mappings or commented-out keys.
func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimRight(line, "\r")
	if len(line) == 0 || line[0] == '#' || line[0] == ' ' || line[0] == '\t' {
		return "", "", false
	}

	sepIdx := strings.IndexAny(line, ":=")
	if sepIdx <= 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:sepIdx])
	value = strings.TrimSpace(line[sepIdx+1:])
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	if value == "" {
		return "", "", false
	}
	return key, value, true
}
