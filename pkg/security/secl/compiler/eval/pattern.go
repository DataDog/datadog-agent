// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	for i, c := range str {
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

// PatternMatches matches a pattern against a string
func PatternMatches(pattern string, str string) bool {
	if pattern == "*" {
		return true
	}

	if len(pattern) == 0 {
		return len(str) == 0
	}

	for len(pattern) > 0 {
		star, segment, nextIndex := nextSegment(pattern)
		if star {
			index := strings.Index(str, segment)
			if index == -1 {
				return false
			}
			str = str[index+len(segment):]
		} else {
			if !strings.HasPrefix(str, segment) {
				return false
			}
			str = str[len(segment):]
		}
		pattern = pattern[nextIndex:]
	}
	return true
}
