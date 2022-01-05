// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"regexp"
	"strings"
)

// PatternMatcher defines a pattern matcher
type PatternMatcher interface {
	Compile(pattern string) error
	Matches(value string) bool
}

// RegexpPatternMatcher defines a regular expression pattern matcher
type RegexpPatternMatcher struct {
	re *regexp.Regexp
}

// Compile a regular expression based pattern
func (r *RegexpPatternMatcher) Compile(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	r.re = re

	return nil
}

// Matches returns whether the value matches
func (r *RegexpPatternMatcher) Matches(value string) bool {
	return r.re.MatchString(value)
}

// SimplePatternMatcher defines a simple pattern matcher
type SimplePatternMatcher struct {
	pattern string
}

// Compile a simple pattern
func (s *SimplePatternMatcher) Compile(pattern string) error {
	s.pattern = pattern
	return nil
}

// Matches returns whether the value matches
func (s *SimplePatternMatcher) Matches(value string) bool {
	return patternExprMatches(s.pattern, value)
}

// GlobPatternMatcher defines a glob pattern matcher
type GlobPatternMatcher struct {
	glob *Glob
}

// Compile a simple pattern
func (g *GlobPatternMatcher) Compile(pattern string) error {
	glob, err := NewGlob(pattern)
	if err != nil {
		return err
	}
	g.glob = glob
	return nil
}

// Matches returns whether the value matches
func (g *GlobPatternMatcher) Matches(value string) bool {
	return g.glob.Matches(value)
}

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

func patternExprMatches(pattern string, str string) bool {
	if pattern == "*" {
		return true
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
