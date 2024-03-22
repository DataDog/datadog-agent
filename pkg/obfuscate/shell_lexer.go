// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"regexp"
	"strings"
)

// ShellTokenKind is a type of shell token.
type ShellTokenKind int

// ShellTokenKind values
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

// Shared regex patterns
var (
	IFS         = " \n\r\t"
	expressions = map[string]*regexp.Regexp{
		"IFS":                  regexp.MustCompile(`^\s*` + IFS),
		"whiteSpace":           regexp.MustCompile(`^\s*[ \n\r\t]+`),
		"doubleQuoteScanUntil": regexp.MustCompile(`"`),
		"doubleQuote":          regexp.MustCompile(`^"`),
		"singleQuoteScanUntil": regexp.MustCompile(`'`),
		"singleQuote":          regexp.MustCompile(`^'`),
		"parenthesesOpen":      regexp.MustCompile(`^\s*\(`),
		"parenthesesClose":     regexp.MustCompile(`^\s*\)`),
		"equal":                regexp.MustCompile(`^\s*=`),
		"backticks":            regexp.MustCompile("^\\s*`"),
		"dollar":               regexp.MustCompile(`^\s*\$`),
		"redirection":          regexp.MustCompile(`^\s*(?:(&>)|(([0-9])?((>\|)|([<>]&[0-9\-]?)|([<>]?>)|(<(<(<)?(-)?)?))))`),
		"anyToken":             regexp.MustCompile(`[` + IFS + "`" + `"'&|=$()<>;]`),
		"control":              regexp.MustCompile(`^([\n;|]|&[^>])+`),
	} // Regex patterns
)

// ShellToken is a token of a shell command
type ShellToken struct {
	Kind  ShellTokenKind
	Val   string
	Start int
	End   int
}

// State represents the current selected token of the scanner
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
			tokenStr := scanUntil(scanner, expressions["doubleQuoteScanUntil"])
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
		tokenString := scanUntil(scanner, expressions["singleQuoteScanUntil"])
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

	if token = scanner.Scan(expressions["whiteSpace"]); token != nil {
		return &ShellToken{WhiteSpace, token.String(), pos, scanner.Index()}
	}

	tokenString := scanUntil(scanner, expressions["anyToken"])
	if tokenString != nil {
		return &ShellToken{Field, *tokenString, pos, scanner.Index()}
	}

	return nil
}

func tokenize(token *ShellToken, state *State, ret []ShellToken) []ShellToken {
	// Handle double quoted strings
	if token.Kind == DoubleQuote {
		if state.current == DoubleQuote {
			state.current = 0
		} else {
			state.current = DoubleQuote
		}
	}

	// Handle single quoted strings
	if token.Kind == SingleQuote {
		if state.current == SingleQuote {
			state.current = 0
		} else {
			state.current = SingleQuote
		}
	}

	// Handle shell subcommands $() in double quoted strings
	if token.Kind == Field && len(ret) > 0 && ret[len(ret)-1].Kind == DoubleQuote {
		reg := regexp.MustCompile(`\$\(`)
		matches := reg.FindAllStringSubmatchIndex(token.Val, -1)

		endOfLastSubcommand := 0
		for _, match := range matches {
			// Don't take match if it inside another subcommand that has already been tokenized
			if match[0] < endOfLastSubcommand {
				continue
			}

			dollarCharIndex := match[0]
			closingParenthesisIndex := getClosingParenthesisIndex(token.Val, dollarCharIndex+1)
			if closingParenthesisIndex == -1 {
				// Did not find closing parentheses
				continue
			}

			// We now need to split the existing token into a maximum of 3 parts
			// The part before the subcommand (field), the subcommand itself (list of tokens) and the part after the subcommand (field)

			if dollarCharIndex > endOfLastSubcommand {
				// There is a part before the subcommand
				previous := token.Val[endOfLastSubcommand:dollarCharIndex]
				preToken := ShellToken{Field, previous, token.Start + endOfLastSubcommand, token.Start + dollarCharIndex}
				ret = append(ret, preToken)
			}

			// Tokenize shell subcommand (without fully parsing it and change the states of tokens)
			subCmdTokens := tokenizeShell(token.Val[dollarCharIndex:closingParenthesisIndex])

			// Dummy array for applying real changes
			// Update offsets (start, end) of subcommand tokens
			var finalSubCmdTokens []ShellToken

			offset := token.Start + dollarCharIndex
			for _, tok := range subCmdTokens {
				tok.Start += offset
				tok.End += offset

				// append
				finalSubCmdTokens = append(finalSubCmdTokens, tok)
			}

			ret = append(ret, finalSubCmdTokens...)
			endOfLastSubcommand = closingParenthesisIndex
		}

		if endOfLastSubcommand < len(token.Val)-1 {
			// There is a part after the last subcommand
			after := token.Val[endOfLastSubcommand:]
			postToken := ShellToken{Field, after, token.Start + endOfLastSubcommand, token.End}
			ret = append(ret, postToken)
		}
	} else {
		ret = append(ret, *token)
	}

	return ret
}

func getClosingParenthesisIndex(str string, start int) int {
	open := 1
	for i := start + 1; i < len(str); i++ {
		if str[i] == '(' {
			open++
		} else if str[i] == ')' {
			open--
		}

		if open == 0 {
			return i + 1
		}
	}

	return -1
}

// changeStates changes tokens kind based on their context and state
// remove whitespaces and set executable tokens
//
//revive:disable-line:empty-block
func changeStates(ret []ShellToken) []ShellToken {
	var withoutWhitespaces []ShellToken
	stateList := []ShellTokenKind{VariableDefinition}
	codeExecutionBackticks := false

	for i := 0; i < len(ret); i++ {
		t := ret[i]
		if t.Kind == DoubleQuote {
			if stateList[len(stateList)-1] == DoubleQuote {
				stateList = stateList[:len(stateList)-1]
				if stateList[len(stateList)-1] == VariableDefinition {
					withoutWhitespaces[len(withoutWhitespaces)-1].Kind = Executable
					stateList = stateList[:len(stateList)-1]
				}
			} else {
				stateList = append(stateList, t.Kind)
			}
		}
		if t.Kind == SingleQuote {
			if stateList[len(stateList)-1] == SingleQuote {
				stateList = stateList[:len(stateList)-1]
			} else {
				stateList = append(stateList, t.Kind)
			}
		}

		if t.Kind == Backticks {
			if codeExecutionBackticks {
				codeExecutionBackticks = false
			} else {
				codeExecutionBackticks = true
				stateList[len(stateList)-1] = VariableDefinition
			}
		}

		if t.Kind == Dollar {
			if i < len(ret)-1 && ret[i+1].Kind == ParentheseOpen {
				stateList[len(stateList)-1] = VariableDefinition
			} else if i < len(ret)-1 && ret[i+1].Kind == Field {
				if len(ret[i+1].Val) > 0 && ret[i+1].Val[0] == '{' {
					// Special case if the field is a shell expansion (with the '{' character)
					ret[i+1].Kind = Executable
				} else {
					// Otherwise that's a shell variable
					ret[i+1].Kind = ShellVariable
				}
			}
		}

		if t.Kind == Control {
			stateList[len(stateList)-1] = VariableDefinition
		} else if len(stateList) > 0 && stateList[len(stateList)-1] == VariableDefinition {
			if t.Kind == Field {
				if len(ret) > i+1 && ret[i+1].Kind == Equal {
					t.Kind = VariableDefinition
					withoutWhitespaces = append(withoutWhitespaces, t)
					i++
					withoutWhitespaces = append(withoutWhitespaces, ret[i])
					stateList[len(stateList)-1] = Equal
					continue
				} else if len(withoutWhitespaces) == 0 || withoutWhitespaces[len(withoutWhitespaces)-1].Kind != Redirection {
					// we skip the case of redirection - token will be added in the default behaviour

					t.Kind = Executable
					stateList[len(stateList)-1] = 0
				}
			}
		} else if len(stateList) > 0 && stateList[len(stateList)-1] == Equal {
			stateList[len(stateList)-1] = VariableDefinition
		}

		if t.Kind != WhiteSpace {
			withoutWhitespaces = append(withoutWhitespaces, t)
		}
	}

	return withoutWhitespaces
}

func tokenizeShell(cmd string) []ShellToken {
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

	return ret
}

// ParseShell parses a shell command and returns a list of tokens
func ParseShell(cmd string) []ShellToken {

	ret := tokenizeShell(cmd)
	// Stage 2.2 find what is executable and remove whitespace
	return changeStates(ret)
}
