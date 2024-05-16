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
	patternElem := newPatternElement(pattern)
	return PatternMatchesWithSegments(patternElem, str, caseInsensitive)
}

// PatternMatchesWithSegments matches a pattern against a string
func PatternMatchesWithSegments(patternElem patternElement, str string, caseInsensitive bool) bool {
	if patternElem.pattern == "*" {
		return true
	}

	if len(patternElem.pattern) == 0 {
		return len(str) == 0
	}

	for _, seg := range patternElem.segments {
		if seg.star {
			// there is no pattern to match after the last star
			if len(seg.segment) == 0 {
				return true
			}

			index := index(str, seg.segment, caseInsensitive)
			if index == -1 {
				return false
			}
			str = str[index+len(seg.segment):]
		} else {
			if !hasPrefix(str, seg.segment, caseInsensitive) {
				return false
			}
			str = str[len(seg.segment):]
		}
	}

	// return false if there is still some str to match
	return len(str) == 0
}

type patternElement struct {
	pattern  string
	segments []patternSegment
}

type patternSegment struct {
	star    bool
	segment string
}

func newPatternElement(element string) patternElement {
	el := patternElement{
		pattern: element,
	}

	for len(element) > 0 {
		star, segment, nextIndex := nextSegment(element)
		element = element[nextIndex:]
		el.segments = append(el.segments, patternSegment{star, segment})
	}

	return el
}
