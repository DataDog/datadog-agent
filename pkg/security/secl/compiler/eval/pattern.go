// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"strings"
)

func nextSegment(str string) (bool, string, int) {
	var inSegment bool
	var start, end int

	var star bool
	if str[0] == '*' {
		star = true
	}

	for i, c := range []byte(str) {
		if c != '*' {
			if !inSegment {
				start = i
				inSegment = true
			}
			end = i
		} else if inSegment {
			break
		}
	}

	if star && start == 0 {
		return star, "", 1
	}

	end++

	return star, str[start:end], end
}

func index(s, subtr string, caseInsensitive bool) int {
	if caseInsensitive {
		s = strings.ToLower(s)
		subtr = strings.ToLower(subtr)
	}
	return strings.Index(s, subtr)
}

func hasPrefix(s, prefix string, caseInsensitive bool) bool {
	if caseInsensitive {
		s = strings.ToLower(s)
		prefix = strings.ToLower(prefix)
	}
	return strings.HasPrefix(s, prefix)
}

// PatternMatches matches a pattern against a string
func PatternMatches(pattern string, str string, caseInsensitive bool) bool {
	if pattern == "*" {
		return true
	}

	if len(pattern) == 0 {
		return len(str) == 0
	}

	for len(pattern) > 0 {
		star, segment, nextIndex := nextSegment(pattern)
		if star {
			// there is no pattern to match after the last star
			if len(segment) == 0 {
				return true
			}

			index := index(str, segment, caseInsensitive)
			if index == -1 {
				return false
			}
			str = str[index+len(segment):]
		} else {
			if !hasPrefix(str, segment, caseInsensitive) {
				return false
			}
			str = str[len(segment):]
		}
		pattern = pattern[nextIndex:]
	}

	// return false if there is still some str to match
	return len(str) == 0
}
