// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"regexp"
	"strings"
)

const procedureIdentifier = `(?:"(?:[^"]|"")+"|[a-zA-Z][a-zA-Z0-9_$#]*)`
const procedureNamePattern = `(` + procedureIdentifier + `)(?:\s*\.\s*(` + procedureIdentifier + `))?`
const procedureScalar = `(?:'(?:[^']|'')*'|:[a-zA-Z0-9_$#]+|[+-]?[0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?|(?i:NULL))`
const procedureArgument = `(?:` + procedureIdentifier + `\s*=>\s*)?` + procedureScalar
const procedureArguments = `\(\s*(?:` + procedureArgument + `(?:\s*,\s*` + procedureArgument + `)*)?\s*\)`

var directCallPattern = regexp.MustCompile(`^\s*(?i:CALL)\s+` + procedureNamePattern + `\s*` + procedureArguments + `\s*;?\s*$`)
var singleBlockCallPattern = regexp.MustCompile(`^\s*(?i:BEGIN)\s+` + procedureNamePattern + `\s*(?:` + procedureArguments + `)?\s*;\s*(?i:END)\s*;?\s*$`)
var unquotedProcedureIdentifier = regexp.MustCompile(`^[A-Z][A-Z0-9_$#]*$`)

// parseDirectProcedureCall accepts exactly one invocation with scalar arguments.
// Expressions, function-valued arguments, loops, declarations, DB links and
// multi-statement blocks are deliberately excluded: cursor executions would
// not establish the number of procedure invocations in those cases.
func parseDirectProcedureCall(statement, parsingSchema string) (owner, name string, qualified, ok bool) {
	statement, ok = removeProcedureComments(statement)
	if !ok {
		return "", "", false, false
	}
	match := directCallPattern.FindStringSubmatch(statement)
	if match == nil {
		match = singleBlockCallPattern.FindStringSubmatch(statement)
	}
	if match == nil {
		return "", "", false, false
	}
	if match[2] == "" {
		return parsingSchema, normalizeProcedureIdentifier(match[1]), false, parsingSchema != ""
	}
	return normalizeProcedureIdentifier(match[1]), normalizeProcedureIdentifier(match[2]), true, true
}

func normalizeProcedureIdentifier(identifier string) string {
	if strings.HasPrefix(identifier, `"`) {
		return strings.ReplaceAll(identifier[1:len(identifier)-1], `""`, `"`)
	}
	return strings.ToUpper(identifier)
}

func displayProcedureIdentifier(identifier string) string {
	if unquotedProcedureIdentifier.MatchString(identifier) {
		return identifier
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func removeProcedureComments(statement string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(statement); {
		switch {
		case statement[i] == '\'' || statement[i] == '"':
			quote := statement[i]
			start := i
			i++
			closed := false
			for i < len(statement) {
				if statement[i] != quote {
					i++
					continue
				}
				i++
				if i < len(statement) && statement[i] == quote {
					i++
					continue
				}
				closed = true
				break
			}
			if !closed {
				return "", false
			}
			out.WriteString(statement[start:i])
		case strings.HasPrefix(statement[i:], "--"):
			end := strings.IndexByte(statement[i:], '\n')
			if end < 0 {
				return out.String(), true
			}
			i += end + 1
			out.WriteByte(' ')
		case strings.HasPrefix(statement[i:], "/*"):
			end := strings.Index(statement[i+2:], "*/")
			if end < 0 {
				return "", false
			}
			i += end + 4
			out.WriteByte(' ')
		default:
			out.WriteByte(statement[i])
			i++
		}
	}
	return out.String(), true
}
