// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lite

import "strings"

// cleanValue strips surrounding quotes and carriage returns from a value.
func cleanValue(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}

// stripSeparators removes underscores and hyphens from a string.
func stripSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		if c != '_' && c != '-' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// parseLine extracts a key-value pair from a config line
func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimRight(line, "\r")
	if len(line) == 0 || line[0] == '#' || line[0] == ' ' || line[0] == '\t' {
		return "", "", false
	}

	sepIdx := strings.IndexAny(line, ":;=")
	if sepIdx <= 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:sepIdx])
	value = strings.TrimSpace(line[sepIdx+1:])
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if value == "" {
		return "", "", false
	}
	return key, value, true
}
