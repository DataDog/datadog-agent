// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"regexp"
)

type ShellScanner struct {
	s     string
	index int
	match *Match
}

type Match struct {
	s string
}

func NewShellScanner(input string) *ShellScanner {
	return &ShellScanner{s: input, index: 0, match: nil}
}

func (s *ShellScanner) String() string {
	return s.s
}

func (s *ShellScanner) SetIndex(newIndex int) {
	s.index = newIndex
}

func (s *ShellScanner) Index() int {
	return s.index
}

func (m *Match) String() string {
	return m.s
}

func (s *ShellScanner) Scan(re *regexp.Regexp) *Match {
	loc := re.FindStringIndex(s.s[s.index:])
	if loc == nil {
		return nil
	}
	s.match = &Match{s: s.s[s.index+loc[0] : s.index+loc[1]]}

	s.index += loc[1]
	return s.match
}
