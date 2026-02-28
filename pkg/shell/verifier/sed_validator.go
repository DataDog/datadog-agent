// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"fmt"
	"strings"
)

// ValidateSedArgs validates sed arguments (as plain strings) for dangerous subcommands.
// This is used by the interpreter after word expansion.
func ValidateSedArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		val := args[i]

		// Skip flags.
		if strings.HasPrefix(val, "-") {
			// Handle -e with a separate argument: -e 'script'
			if val == "-e" && i+1 < len(args) {
				i++
				if err := validateSedScript(args[i]); err != nil {
					return err
				}
			} else if strings.HasPrefix(val, "-e") && len(val) > 2 {
				// Handle -eSCRIPT combined form.
				if err := validateSedScript(val[2:]); err != nil {
					return err
				}
			}
			continue
		}

		// The first non-flag argument is the sed script (unless -e was used).
		if isSedScriptPositionStr(args, i) {
			if err := validateSedScript(val); err != nil {
				return err
			}
		}
	}
	return nil
}

// isSedScriptPositionStr determines whether args[idx] is in a position where
// it would be interpreted as a sed script (as opposed to a filename).
func isSedScriptPositionStr(args []string, idx int) bool {
	for _, arg := range args {
		if arg == "-e" {
			return false
		}
		if len(arg) > 2 && arg[0] == '-' && arg[1] == 'e' && !(arg[2] >= 'A' && arg[2] <= 'Z') {
			return false
		}
	}

	nonFlagCount := 0
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			if args[i] == "-e" || args[i] == "-f" {
				i++
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

// validateSedScript scans a sed script string for dangerous commands.
// Returns an error if a dangerous command is found.
func validateSedScript(script string) error {
	scanner := &sedScanner{script: script, pos: 0}
	for scanner.pos < len(scanner.script) {
		scanner.skipWhitespaceAndLabels()
		if scanner.pos >= len(scanner.script) {
			break
		}

		ch := scanner.script[scanner.pos]

		switch ch {
		case 's':
			scanner.pos++
			if err := scanner.skipSubstitutionValidate(); err != nil {
				return err
			}

		case 'e':
			return fmt.Errorf("sed 'e' command (execute) is not allowed")

		case 'w', 'W':
			return fmt.Errorf("sed 'w'/'W' command (write to file) is not allowed")

		case 'r', 'R':
			return fmt.Errorf("sed 'r'/'R' command (read file) is not allowed")

		case 'y':
			scanner.pos++
			if scanner.pos < len(scanner.script) {
				delim := scanner.script[scanner.pos]
				scanner.pos++
				scanner.skipUntilDelim(delim)
				scanner.skipUntilDelim(delim)
			}

		case 'b', 't', 'T':
			scanner.pos++
			scanner.skipToEndOfCommand()

		case 'a', 'i', 'c':
			scanner.pos++
			scanner.skipToEndOfCommand()

		case '{':
			scanner.pos++

		case '}':
			scanner.pos++

		case '#':
			scanner.skipToNewline()

		default:
			scanner.pos++
		}
	}
	return nil
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
			s.skipToEndOfCommand()
		} else {
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

	if ch >= '0' && ch <= '9' {
		for s.pos < len(s.script) && s.script[s.pos] >= '0' && s.script[s.pos] <= '9' {
			s.pos++
		}
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	if ch == '$' {
		s.pos++
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	if ch == '/' {
		s.skipDelimitedPattern('/')
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}

	if ch == '\\' && s.pos+1 < len(s.script) {
		s.pos++
		delim := s.script[s.pos]
		s.skipDelimitedPattern(delim)
		if s.pos < len(s.script) && s.script[s.pos] == ',' {
			s.pos++
			s.skipAddress()
		}
		return
	}
}

// skipSubstitutionValidate skips past a s/pattern/replacement/ command and returns
// an error if dangerous flags (e, w) are found.
func (s *sedScanner) skipSubstitutionValidate() error {
	if s.pos >= len(s.script) {
		return nil
	}

	delim := s.script[s.pos]
	s.pos++

	s.skipUntilDelim(delim)
	s.skipUntilDelim(delim)

	for s.pos < len(s.script) {
		ch := s.script[s.pos]
		if ch == ';' || ch == '\n' || ch == '}' {
			break
		}
		if ch == 'e' {
			return fmt.Errorf("sed 's///e' flag (execute replacement) is not allowed")
		}
		if ch == 'w' {
			return fmt.Errorf("sed 's///w' flag (write to file) is not allowed")
		}
		s.pos++
	}
	return nil
}

func (s *sedScanner) skipUntilDelim(delim byte) {
	for s.pos < len(s.script) && s.script[s.pos] != delim {
		if s.script[s.pos] == '\\' && s.pos+1 < len(s.script) {
			s.pos++
		}
		s.pos++
	}
	if s.pos < len(s.script) {
		s.pos++
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

func (s *sedScanner) skipDelimitedPattern(delim byte) {
	s.pos++
	for s.pos < len(s.script) && s.script[s.pos] != delim {
		if s.script[s.pos] == '\\' && s.pos+1 < len(s.script) {
			s.pos++
		}
		s.pos++
	}
	if s.pos < len(s.script) {
		s.pos++
	}
}
