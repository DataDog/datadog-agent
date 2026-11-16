// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"strconv"
	"strings"
	"sync"
)

// ownerRule is one compiled CODEOWNERS row.
type ownerRule struct {
	// segments is the pattern split on "/"; a "**" segment matches any number
	// of path segments, including none.
	segments []string
	// dirOnly is set by a trailing "/": the pattern only matches the contents
	// of a directory, never a file with that name.
	dirOnly bool
	// team is the primary owner slug, or "" when the row names no team.
	team string
}

// codeOwnersRules compiles the generated table on first use, so agents that
// never log an error do not pay for it.
var codeOwnersRules = sync.OnceValue(func() []ownerRule {
	return parseOwnerRules(ownerTeams, ownerRules)
})

// parseOwnerRules compiles the generated tables into rules in file order.
// Rows that do not parse are dropped.
func parseOwnerRules(teamsTable, rulesTable string) []ownerRule {
	teams := strings.Split(teamsTable, "\n")
	lines := strings.Split(rulesTable, "\n")
	rules := make([]ownerRule, 0, len(lines))
	for _, line := range lines {
		index, pattern, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		team := ""
		if index != "-" {
			i, err := strconv.Atoi(index)
			if err != nil || i < 0 || i >= len(teams) {
				continue
			}
			team = teams[i]
		}
		rules = append(rules, compileOwnerRule(pattern, team))
	}
	return rules
}

// compileOwnerRule follows the CODEOWNERS pattern syntax: a pattern with a
// "/" anywhere but the end is anchored at the repository root, any other
// pattern matches at every depth; "*" and "?" stay within one path segment;
// "**" spans segments; a pattern naming a directory matches everything below
// it. A "**" segment may match zero directories, as on GitHub, while the
// Python parser in tasks/libs/owners needs at least one; the parity test only
// replays paths with one or more, and TestGeneratedTableZeroDirectories pins
// the zero case. Character classes and escapes are not supported; the generator rejects
// them.
func compileOwnerRule(pattern, team string) ownerRule {
	slash := strings.Index(pattern, "/")
	anchored := slash >= 0 && slash != len(pattern)-1
	segments := strings.Split(strings.Trim(pattern, "/"), "/")
	if !anchored {
		segments = append([]string{"**"}, segments...)
	}
	return ownerRule{segments: segments, dirOnly: strings.HasSuffix(pattern, "/"), team: team}
}

// ownerOfPath returns the primary owner slug of a repository-relative path:
// the owner of the last matching rule, as in CODEOWNERS. It returns "" when no
// rule matches or the matching rule names no team.
func ownerOfPath(rules []ownerRule, path string) string {
	segments := strings.Split(path, "/")
	for i := len(rules) - 1; i >= 0; i-- {
		if matchSegments(rules[i].segments, segments, rules[i].dirOnly) {
			return rules[i].team
		}
	}
	return ""
}

// matchSegments reports whether pattern matches path or one of its parent
// directories. With dirOnly, the match must be a parent directory.
func matchSegments(pattern, path []string, dirOnly bool) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(path); i++ {
				if matchSegments(pattern, path[i:], dirOnly) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 || !matchGlob(pattern[0], path[0]) {
			return false
		}
		pattern, path = pattern[1:], path[1:]
	}
	return len(path) > 0 || !dirOnly
}

// matchGlob matches one path segment against a pattern segment where "*"
// matches any run of characters and "?" any single character.
func matchGlob(pattern, name string) bool {
	p, n := 0, 0
	star, mark := -1, 0
	for n < len(name) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, n
			p++
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == name[n]):
			p++
			n++
		case star >= 0:
			mark++
			p, n = star+1, mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
