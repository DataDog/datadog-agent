// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// EtcDefaultPath is the SysV and Upstart procmgr settings file. The init scripts
// source it and stop dd-procmgrd only when DATADOG_ENABLED is exactly "no".
const EtcDefaultPath = "/etc/default/datadog-agent-procmgr"

const etcDefaultVar = "DATADOG_ENABLED"

var (
	// etcDefaultCommentStart matches the whitespace and '#' that start a comment after an unquoted value.
	etcDefaultCommentStart = regexp.MustCompile(`\s+#`)
	// etcDefaultPlainValue is the only value shape accepted: nothing a shell would expand or split.
	etcDefaultPlainValue = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)
)

// etcDefaultBlockWords are the first words of lines that open, close or continue a
// shell block or subshell. An assignment in such a block may not run.
var etcDefaultBlockWords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"case": true, "esac": true, "for": true, "while": true, "until": true,
	"do": true, "done": true, "{": true, "}": true, "(": true, ")": true,
}

// SeedFromEtcDefault reads DATADOG_ENABLED from the SysV and Upstart settings file.
//
// The file is never executed. It is read as a flat list of simple lines. A missing
// file or a file without the variable is not found. A line that mentions the
// variable in a form this parser does not fully understand is a *SeedError, so the
// installer never acts on an opt-out that the init scripts might read differently.
func SeedFromEtcDefault(path string) (SeedResult, error) {
	res := SeedResult{Detail: path}
	data, found, err := readSeedFile(path)
	if err != nil {
		return res, &SeedError{Detail: path, Err: err}
	}
	if !found {
		return res, nil
	}
	enabled, set, err := parseEtcDefault(string(data))
	if err != nil {
		return res, &SeedError{Detail: path, Err: err}
	}
	if set {
		res.Found = true
		res.Enabled = enabled
	}
	return res, nil
}

// parseEtcDefault returns the value of the last valid DATADOG_ENABLED assignment.
// set is false when there is none. Any malformed line makes the whole result an
// error, even if a later line is valid.
func parseEtcDefault(content string) (enabled bool, set bool, err error) {
	lines := strings.Split(content, "\n")
	fileMentions, blockWord := false, false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		mentions := strings.Contains(line, etcDefaultVar)
		fileMentions = fileMentions || mentions
		if etcDefaultBlockWords[strings.Fields(line)[0]] {
			blockWord = true
		}
		if strings.HasSuffix(line, `\`) && i+1 < len(lines) && etcDefaultMentions(lines[i+1]) {
			return false, false, fmt.Errorf("%s after a line continuation is not supported", etcDefaultVar)
		}

		value, assignment := etcDefaultAssignmentValue(line)
		if !assignment {
			if mentions {
				return false, false, fmt.Errorf("%s line is not a simple assignment: %q", etcDefaultVar, line)
			}
			continue
		}
		v, err := etcDefaultValue(value)
		if err != nil {
			return false, false, err
		}
		enabled, set = v != "no", true
	}
	if fileMentions && blockWord {
		return false, false, fmt.Errorf("conditional shell syntax is not supported in a file that sets %s", etcDefaultVar)
	}
	return enabled, set, nil
}

// etcDefaultMentions reports whether a raw line is a non-comment line that mentions the variable.
func etcDefaultMentions(raw string) bool {
	line := strings.TrimSpace(raw)
	return !strings.HasPrefix(line, "#") && strings.Contains(line, etcDefaultVar)
}

// etcDefaultAssignmentValue returns the text after "DATADOG_ENABLED=" when the line is
// a recognized assignment: an optional leading "export" followed by whitespace, then
// the variable name with no whitespace around the '='.
func etcDefaultAssignmentValue(line string) (string, bool) {
	body := line
	if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
		body = strings.TrimLeft(rest, " \t")
	}
	return strings.CutPrefix(body, etcDefaultVar+"=")
}

// etcDefaultValue returns the value of a recognized assignment, as the shell would
// see it: a quoted value ends at the next matching quote, an unquoted value ends at
// a comment. The value must be a plain word.
func etcDefaultValue(raw string) (string, error) {
	var value string
	if raw != "" && (raw[0] == '"' || raw[0] == '\'') {
		quote := raw[0]
		end := strings.IndexByte(raw[1:], quote)
		if end < 0 {
			return "", fmt.Errorf("%s has an unterminated quote: %q", etcDefaultVar, raw)
		}
		value = raw[1 : 1+end]
		// After the closing quote: nothing, or whitespace, optionally followed by a comment.
		rest := raw[end+2:]
		if rest != "" {
			trimmed := strings.TrimLeft(rest, " \t")
			validTail := trimmed != rest && (trimmed == "" || trimmed[0] == '#')
			if !validTail {
				return "", fmt.Errorf("%s has text after the closing quote: %q", etcDefaultVar, raw)
			}
		}
	} else {
		if loc := etcDefaultCommentStart.FindStringIndex(raw); loc != nil {
			raw = raw[:loc[0]]
		}
		value = strings.TrimRightFunc(raw, unicode.IsSpace)
	}
	if !etcDefaultPlainValue.MatchString(value) {
		return "", fmt.Errorf("%s value %q is not a plain word", etcDefaultVar, value)
	}
	return value, nil
}
