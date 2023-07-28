// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"regexp"
	"strings"
)

type ShellTokenKind int

const (
	Executable ShellTokenKind = iota
	Field
	DoubleQuote
	SingleQuote
	Control
	VariableDefinition
	WhiteSpace
	Equal
	Backticks
	Dollar
	ShellVariable
	Redirection
	ParentheseOpen
	ParentheseClose
)

var (
	IFS         = " \n\r\t" // FIXME use real $IFS
	expressions = map[string]*regexp.Regexp{
		"IFS":              regexp.MustCompile(` \n\r\t`), // FIXME use real IFS
		"whiteSpace":       regexp.MustCompile(`[ \n\r\t]+`),
		"doubleQuote":      regexp.MustCompile(`"`),
		"singleQuote":      regexp.MustCompile(`'`),
		"parenthesesOpen":  regexp.MustCompile(`\(`),
		"parenthesesClose": regexp.MustCompile(`\)`),
		"equal":            regexp.MustCompile(`=`),
		"backticks":        regexp.MustCompile("`"),
		"dollar":           regexp.MustCompile(`\$`),
		"redirection":      regexp.MustCompile(`(&>)|(([0-9])?((>\|)|([<>]&[0-9\-]?)|([<>]?>)|(<(<(<)?(-)?)?)))`),
		"anyToken":         regexp.MustCompile(`[` + IFS + "`" + `"'&|=$()<>;]`),
		"control":          regexp.MustCompile(`([\n;|]|&[^>])+`),
	} // Regex patterns
)

type ShellToken struct {
	kind  ShellTokenKind
	val   string
	start int
	end   int
}

type State struct {
	current ShellTokenKind
}

// scanUntil searches for the first occurrence of the regex pattern in the input string starting at the current index
func scanUntil(s *ShellScanner, pattern *regexp.Regexp) *string {
	initialPosition := s.Index()
	if initialPosition >= len(s.String()) {
		return nil
	}

	found := pattern.FindStringIndex(s.String()[initialPosition:])
	if len(found) == 0 {
		s.SetIndex(len(s.String()))
	} else {
		s.SetIndex(initialPosition + found[0])
	}

	matched := s.String()[initialPosition:s.Index()]
	return &matched
}

func nextToken(scanner *ShellScanner, state struct{ current ShellTokenKind }) *ShellToken {
	pos := scanner.Index()
	var token *Match

	if state.current != DoubleQuote && state.current != SingleQuote {
		if token = scanner.Scan(expressions["control"]); token != nil {
			return &ShellToken{Control, token.String(), pos, scanner.Index()}
		}
		if token = scanner.Scan(expressions["whiteSpace"]); token != nil {
			return &ShellToken{WhiteSpace, token.String(), pos, scanner.Index()}
		}
	}

	// Handle double quoted strings
	if token = scanner.Scan(expressions["doubleQuote"]); token != nil {
		return &ShellToken{DoubleQuote, token.String(), pos, scanner.Index()}
	}
	if state.current == DoubleQuote {
		var fullToken []string

		for escaped := true; escaped; {
			tokenStr := scanUntil(scanner, expressions["doubleQuote"])
			if tokenStr == nil {
				return nil
			}

			escaped = false
			fullToken = append(fullToken, *tokenStr)
			i := 1

			for ; len(*tokenStr)-i > 0 && (*tokenStr)[len(*tokenStr)-i] == '\\'; i++ {
				escaped = !escaped
			}

			if escaped {
				fullToken = append(fullToken, "\"")
				scanner.SetIndex(scanner.Index() + 1)
			}
		}

		if len(fullToken) > 0 {
			return &ShellToken{Field, strings.Join(fullToken, ""), pos, scanner.Index()}
		}

		return nil
	}

	// Handle single quoted strings
	if token = scanner.Scan(expressions["singleQuote"]); token != nil {
		return &ShellToken{SingleQuote, token.String(), pos, scanner.Index()}
	}
	if state.current == SingleQuote {
		tokenString := scanUntil(scanner, expressions["singleQuote"])
		if tokenString != nil {
			return &ShellToken{Field, *tokenString, pos, scanner.Index()}
		}
		return nil
	}

	// General case
	if token = scanner.Scan(expressions["parenthesesOpen"]); token != nil {
		return &ShellToken{ParentheseOpen, token.String(), pos, scanner.Index()}
	}

	if token = scanner.Scan(expressions["parenthesesClose"]); token != nil {
		return &ShellToken{ParentheseClose, token.String(), pos, scanner.Index()}
	}

	if token = scanner.Scan(expressions["dollar"]); token != nil {
		return &ShellToken{Dollar, token.String(), pos, scanner.Index()}
	}

	if token = scanner.Scan(expressions["redirection"]); token != nil {
		return &ShellToken{Redirection, token.String(), pos, scanner.Index()}
	}

	if token = scanner.Scan(expressions["equal"]); token != nil {
		return &ShellToken{Equal, token.String(), pos, scanner.Index()}
	}

	if token = scanner.Scan(expressions["backticks"]); token != nil {
		return &ShellToken{Backticks, token.String(), pos, scanner.Index()}
	}

	tokenString := scanUntil(scanner, expressions["anyToken"])
	if tokenString != nil {
		return &ShellToken{Field, *tokenString, pos, scanner.Index()}
	}

	return nil
}

func tokenize(token *ShellToken, state *State, ret []ShellToken) []ShellToken {
	// Handle double quoted strings
	if token.kind == DoubleQuote {
		if state.current == DoubleQuote {
			state.current = 0
		} else {
			state.current = DoubleQuote
		}
	}

	// Handle single quoted strings
	if token.kind == SingleQuote {
		if state.current == SingleQuote {
			state.current = 0
		} else {
			state.current = SingleQuote
		}
	}

	// Handle shell subcommands
	if token.kind == Field && len(ret) > 0 && ret[len(ret)-1].kind == DoubleQuote && strings.Contains(token.val, "$(") {
		reg := regexp.MustCompile(`\$\(([^)]*)\)`)
		matches := reg.FindAllStringSubmatchIndex(token.val, -1)

		index := 0
		for _, match := range matches {
			previous := token.val[index:match[0]]
			preToken := ShellToken{Field, previous, token.start + index, token.start + len(previous) + index}

			// Parse shell subcommand of the match of index 0
			subCmdTokens := parseShell(token.val[match[0]:match[1]]) // TODO NOT SURE ABOUT THAT

			// Dummy array for applying real changes
			var finalSubCmdTokens []ShellToken

			offset := index + len(previous)
			for _, tok := range subCmdTokens {
				tok.start += token.start + offset
				tok.end += token.start + offset

				// append
				finalSubCmdTokens = append(finalSubCmdTokens, tok)
			}

			ret = append(ret, preToken)
			ret = append(ret, finalSubCmdTokens...)

			index = finalSubCmdTokens[len(finalSubCmdTokens)-1].end
		}
	} else {
		ret = append(ret, *token)
	}

	return ret
}

// changeStates changes tokens kind based on their context and state
// remove whitespaces and set executable tokens
func changeStates(ret []ShellToken) []ShellToken {
	var withoutWhitespaces []ShellToken
	stateList := []ShellTokenKind{VariableDefinition}
	codeExecutionBackticks := false
	codeExecutionDollar := false

	for i := 0; i < len(ret); i++ {
		t := ret[i]
		if t.kind == DoubleQuote {
			if stateList[len(stateList)-1] == DoubleQuote {
				stateList = stateList[:len(stateList)-1]
				if stateList[len(stateList)-1] == VariableDefinition {
					withoutWhitespaces[len(withoutWhitespaces)-1].kind = Executable
					stateList = stateList[:len(stateList)-1]
				}
			} else {
				stateList = append(stateList, t.kind)
			}
		}
		if t.kind == SingleQuote {
			if stateList[len(stateList)-1] == SingleQuote {
				stateList = stateList[:len(stateList)-1]
			} else {
				stateList = append(stateList, t.kind)
			}
		}
		if t.kind == Backticks {
			if codeExecutionBackticks {
				codeExecutionBackticks = false
			} else {
				codeExecutionBackticks = true
				stateList[len(stateList)-1] = VariableDefinition
			}
		}
		if t.kind == Dollar {
			if i < len(ret)-1 && ret[i+1].kind == ParentheseOpen {
				if codeExecutionDollar {
					codeExecutionDollar = false
				} else {
					codeExecutionDollar = true
					stateList[len(stateList)-1] = VariableDefinition
				}
			} else if i < len(ret)-1 && ret[i+1].kind == Field {
				ret[i+1].kind = ShellVariable
			}
		}

		if t.kind == Control {
			stateList[len(stateList)-1] = VariableDefinition
		} else if len(stateList) > 0 && stateList[len(stateList)-1] == VariableDefinition {
			if t.kind == Field || t.kind == ShellVariable {
				if len(ret) > i+1 && ret[i+1].kind == Equal {
					t.kind = VariableDefinition
					withoutWhitespaces = append(withoutWhitespaces, t)
					i++
					withoutWhitespaces = append(withoutWhitespaces, ret[i])
					stateList[len(stateList)-1] = Equal
					continue

				} else if len(withoutWhitespaces) > 0 && withoutWhitespaces[len(withoutWhitespaces)-1].kind == Redirection {
					// do nothing - this is a redirection - token will be added in the default behaviour
				} else {
					t.kind = Executable
					stateList[len(stateList)-1] = 0
				}
			}
		} else if len(stateList) > 0 && stateList[len(stateList)-1] == Equal {
			stateList[len(stateList)-1] = VariableDefinition
		}

		if t.kind != WhiteSpace {
			withoutWhitespaces = append(withoutWhitespaces, t)
		}
	}

	return withoutWhitespaces
}

// parseShell parses a shell command and returns a list of tokens
func parseShell(cmd string) []ShellToken {
	state := State{current: 0}

	scanner := NewShellScanner(cmd)
	var ret []ShellToken
	var token *ShellToken

	// Stage 2.1 Tokenize
	for {
		token = nextToken(scanner, state)
		if token == nil {
			break
		}

		ret = tokenize(token, &state, ret)
	}

	// Stage 2.2 find what is executable and remove whitespace
	return changeStates(ret)
}

// matchingParams returns a list of params that are found in the shell command
func matchingParams(shCmd string, params []string) []string {
	var found []string
	for _, paramVal := range params {
		if strings.Contains(shCmd, paramVal) {
			found = append(found, paramVal)
		}
	}
	return found
}

// findTokenWithIndex finds a token's index given a target position.
// Using a binary search
func findTokenWithIndex(parsedTokens []ShellToken, targetPos int) int {
	min, max := 0, len(parsedTokens)
	for min <= max {
		currentIndex := min + (max-min)/2
		if parsedTokens[currentIndex].start <= targetPos && targetPos < parsedTokens[currentIndex].end {
			return currentIndex
		} else if parsedTokens[currentIndex].start > targetPos {
			max = currentIndex - 1
		} else {
			min = currentIndex + 1
		}
	}

	return -1
}
