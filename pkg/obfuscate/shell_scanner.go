// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"regexp"
)

// ShellScanner is a simple scanner for shell commands.
type ShellScanner struct {
	s     string
	index int
	match *match
}

type match struct {
	s string
}

// NewShellScanner creates a new ShellScanner for the given input.
func NewShellScanner(input string) *ShellScanner {
	return &ShellScanner{s: input, index: 0, match: nil}
}

// String returns the string that the scanner is scanning.
func (s *ShellScanner) String() string {
	return s.s
}

// SetIndex sets the index of the scanner.
func (s *ShellScanner) SetIndex(newIndex int) {
	s.index = newIndex
}

// Index returns the index of the scanner.
func (s *ShellScanner) Index() int {
	return s.index
}

// Match returns the current match.
func (m *match) String() string {
	return m.s
}

// Scan scans the string for the given regexp.
func (s *ShellScanner) Scan(re *regexp.Regexp) *match {
	loc := re.FindStringIndex(s.s[s.index:])
	if loc == nil {
		return nil
	}
	s.match = &match{s: s.s[s.index+loc[0] : s.index+loc[1]]}

	s.index += loc[1]
	return s.match
}
