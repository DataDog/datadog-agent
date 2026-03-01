// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// verifySedArgs inspects sed arguments to find script expressions and
// validates their content for dangerous subcommands.
func (v *verifier) verifySedArgs(args []*syntax.Word) {
	for i := 0; i < len(args); i++ {
		val, ok := literalWordValue(args[i])
		if !ok {
			// Non-literal sed argument: could be a file path (fine)
			// or a script with variable expansion (can't verify).
			// We need to determine if this is in a script position.
			if isSedScriptPosition(args, i) {
				v.addViolation(args[i].Pos(), "command",
					"sed script contains variable expansion and cannot be verified for safety")
			}
			continue
		}

		// Skip flags — they were already validated in verifyFlags.
		if strings.HasPrefix(val, "-") {
			// Handle -e with a separate argument: -e 'script'
			if val == "-e" && i+1 < len(args) {
				i++
				scriptVal, scriptOk := literalWordValue(args[i])
				if !scriptOk {
					v.addViolation(args[i].Pos(), "command",
						"sed script contains variable expansion and cannot be verified for safety")
					continue
				}
				v.verifySedScript(args[i].Pos(), scriptVal)
			} else if strings.HasPrefix(val, "-e") && len(val) > 2 {
				// Handle -eSCRIPT combined form (e.g., -es/a/b/e)
				script := val[2:]
				v.verifySedScript(args[i].Pos(), script)
			}
			continue
		}

		// This is a non-flag argument. The first non-flag, non-option-arg
		// is the sed script (unless -e was used).
		if isSedScriptPosition(args, i) {
			v.verifySedScript(args[i].Pos(), val)
		}
	}
}

// isSedScriptPosition determines whether args[idx] is in a position where
// it would be interpreted as a sed script (as opposed to a filename).
func isSedScriptPosition(args []*syntax.Word, idx int) bool {
	// If any -e flag was used (separate or combined), all non-flag args are filenames, not scripts.
	for _, arg := range args {
		val, ok := literalWordValue(arg)
		if !ok {
			continue
		}
		if val == "-e" {
			return false
		}
		// Combined form: -eSCRIPT. Must distinguish from -E (extended regex).
		// -e followed by a non-uppercase letter is a combined -e script.
		if len(val) > 2 && val[0] == '-' && val[1] == 'e' && !(val[2] >= 'A' && val[2] <= 'Z') {
			return false
		}
	}

	// Otherwise, the first non-flag argument is the script.
	nonFlagCount := 0
	for i := 0; i < len(args); i++ {
		val, ok := literalWordValue(args[i])
		if !ok {
			if i == idx {
				return nonFlagCount == 0
			}
			nonFlagCount++
			continue
		}

		if strings.HasPrefix(val, "-") {
			// Skip flags and their values.
			if val == "-e" || val == "-f" {
				i++ // skip the next arg (value of -e/-f)
			}
			continue
		}

		if i == idx {
			return nonFlagCount == 0
		}
		nonFlagCount++
	}
	return false
}

// verifySedScript scans a sed script string for dangerous commands.
func (v *verifier) verifySedScript(pos syntax.Pos, script string) {
	// We scan the script to detect dangerous sed commands.
	// Sed scripts can be complex, so we use a conservative approach:
	// scan for known-dangerous patterns.

	scanner := &sedScanner{script: script, pos: 0}
	for scanner.pos < len(scanner.script) {
		scanner.skipWhitespaceAndLabels()
		if scanner.pos >= len(scanner.script) {
			break
		}

		ch := scanner.script[scanner.pos]

		switch ch {
		case 's':
			// Substitution command — check flags after the closing delimiter.
			scanner.pos++
			scanner.skipSubstitution(v, pos)

		case 'e':
			// Standalone 'e' command executes the pattern space as a shell command.
			v.addViolation(pos, "command", "sed 'e' command (execute) is not allowed")
			scanner.pos++

		case 'w', 'W':
			// Write commands — write to files.
			v.addViolation(pos, "command", "sed 'w'/'W' command (write to file) is not allowed")
			scanner.pos++
			scanner.skipToEndOfCommand()

		case 'r', 'R':
			// Read file into output — could leak file contents.
			v.addViolation(pos, "command", "sed 'r'/'R' command (read file) is not allowed")
			scanner.pos++
			scanner.skipToEndOfCommand()

		case 'y':
			// Transliterate command y/source/dest/ — same delimiter structure as 's'.
			scanner.pos++
			if scanner.pos < len(scanner.script) {
				delim := scanner.script[scanner.pos]
				scanner.pos++
				scanner.skipUntilDelim(delim)
				scanner.skipUntilDelim(delim)
			}

		case 'b', 't', 'T':
			// Branch/test commands — may have a label argument.
			scanner.pos++
			scanner.skipToEndOfCommand()

		case 'a', 'i', 'c':
			// Text commands — text follows to end of line.
			scanner.pos++
			scanner.skipToEndOfCommand()

		case '{':
			// Block start — continue scanning inside.
			scanner.pos++

		case '}':
			// Block end.
			scanner.pos++

		case '#':
			// Comment — skip to end of line.
			scanner.skipToNewline()

		default:
			// Other commands (d, p, q, n, x, etc.) are safe single-character
			// commands. Do NOT call skipAddress() here — addresses precede
			// commands, they don't follow them.
			scanner.pos++
		}
	}
}

// sedScanner is a simple scanner for sed script content.
type sedScanner struct {
	script string
	pos    int
}

func (s *sedScanner) skipWhitespaceAndLabels() {
	for s.pos < len(s.script) {
		ch := s.script[s.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ';' {
			s.pos++
		} else if ch == ':' {
			// Label — skip to end of line or semicolon.
			s.skipToEndOfCommand()
		} else {
			// Skip address prefix (e.g., /pattern/, line numbers)
			s.skipAddress()
			return
		}
	}
}

func (s *sedScanner) skipAddress() {
	if s.pos >= len(s.script) {
		return
	}
	ch := s.script[s.pos]

	// Line number address
	if ch >= '0' && ch <= '9' {
		for s.pos < len(s.script) && s.script[s.pos] >= '0' && s.script[s.pos] <= '9' {
			s.pos++
		}
		// Check for comma (range)
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	// $ address (last line)
	if ch == '$' {
		s.pos++
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	// /regex/ address
	if ch == '/' {
		s.skipDelimitedPattern('/')
		// Check for comma (range)
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	// \Xregex\X address (alternate delimiter syntax)
	if ch == '\\' && s.pos+1 < len(s.script) {
		s.pos++ // skip backslash
		delim := s.script[s.pos]
		s.skipDelimitedPattern(delim)
		// Check for comma (range)
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}
}

// skipSubstitution skips past a s/pattern/replacement/ command and checks
// the flags for dangerous 'e' and 'w'.
func (s *sedScanner) skipSubstitution(v *verifier, pos syntax.Pos) {
	if s.pos >= len(s.script) {
		return
	}

	delim := s.script[s.pos]
	s.pos++ // skip delimiter

	// Skip pattern
	s.skipUntilDelim(delim)
	// Skip replacement
	s.skipUntilDelim(delim)

	// Now we're at the flags position. Scan for dangerous flags.
	for s.pos < len(s.script) {
		ch := s.script[s.pos]
		if ch == ';' || ch == '\n' || ch == '}' {
			break
		}
		if ch == 'e' {
			v.addViolation(pos, "command", "sed 's///e' flag (execute replacement) is not allowed")
		}
		if ch == 'w' {
			v.addViolation(pos, "command", "sed 's///w' flag (write to file) is not allowed")
			// Skip the filename that follows 'w'
			s.pos++
			s.skipToEndOfCommand()
			return
		}
		s.pos++
	}
}

func (s *sedScanner) skipUntilDelim(delim byte) {
	for s.pos < len(s.script) && s.script[s.pos] != delim {
		if s.script[s.pos] == '\\' && s.pos+1 < len(s.script) {
			s.pos++ // skip escaped char
		}
		s.pos++
	}
	if s.pos < len(s.script) {
		s.pos++ // skip closing delimiter
	}
}

func (s *sedScanner) skipToEndOfCommand() {
	for s.pos < len(s.script) && s.script[s.pos] != ';' && s.script[s.pos] != '\n' {
		s.pos++
	}
}

func (s *sedScanner) skipToNewline() {
	for s.pos < len(s.script) && s.script[s.pos] != '\n' {
		s.pos++
	}
}

// skipDelimitedPattern skips a /pattern/ or Xpattern\X section.
func (s *sedScanner) skipDelimitedPattern(delim byte) {
	s.pos++ // skip opening delimiter
	for s.pos < len(s.script) && s.script[s.pos] != delim {
		if s.script[s.pos] == '\\' && s.pos+1 < len(s.script) {
			s.pos++ // skip escaped char
		}
		s.pos++
	}
	if s.pos < len(s.script) {
		s.pos++ // skip closing delimiter
	}
}
