// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides log message tokenization using finite state automaton
// and pattern matching for semantic token classification.
package automaton

import (
	"regexp"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TokenizerState represents the current state of the FSA
type TokenizerState int

const (
	StateStart      TokenizerState = iota
	StateWord                      // Letters, digits, and common separators for structured tokens
	StateNumeric                   // Pure numbers
	StateWhitespace                // Spaces, tabs, newlines
	StateSpecial                   // Operators, punctuation, symbols
)

// Tokenizer implements a finite state automaton for log tokenization
type Tokenizer struct {
	input  string
	pos    int
	length int
	state  TokenizerState
	buffer []rune
	tokens []token.Token
}

// NewTokenizer creates a new tokenizer for the given input
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{
		input:  input,
		pos:    0,
		length: len(input),
		state:  StateStart,
		buffer: make([]rune, 0, 64),        // Pre-allocate buffer
		tokens: make([]token.Token, 0, 32), // Pre-allocate tokens slice
	}
}

// Tokenize processes the input string and returns a TokenList
func (t *Tokenizer) Tokenize() *token.TokenList {
	for t.pos < t.length {
		if !t.processNextToken() {
			break
		}
	}

	t.flushBuffer()
	t.classifyTokens()

	return token.NewTokenListWithTokens(t.tokens)
}

// classifyTokens applies terminal rules for token classification
func (t *Tokenizer) classifyTokens() {
	for i, tok := range t.tokens {
		// Only classify word-like and numeric tokens that might be structured
		if tok.Type != token.TokenWord && tok.Type != token.TokenNumeric {
			continue
		}

		classifiedType := t.classifyToken(tok.Value)
		if classifiedType == token.TokenUnknown {
			continue
		}

		// Update token type
		t.tokens[i].Type = classifiedType

		// Parse date components for date tokens
		if classifiedType == token.TokenDate {
			t.tokens[i].DateInfo = parseDateComponents(tok.Value)
		}
	}
}

// processNextToken advances the automaton by one token
func (t *Tokenizer) processNextToken() bool {
	if t.pos >= t.length {
		return false
	}

	char := rune(t.input[t.pos])

	switch t.state {
	case StateStart:
		return t.handleStartState(char)
	case StateWord:
		return t.handleWordState(char)
	case StateNumeric:
		return t.handleNumericState(char)
	case StateWhitespace:
		return t.handleWhitespaceState(char)
	case StateSpecial:
		return t.handleSpecialState(char)
	default:
		return t.handleStartState(char) // Fallback
	}
}

// handleStartState determines initial state based on character type
func (t *Tokenizer) handleStartState(char rune) bool {
	switch {
	case unicode.IsSpace(char):
		t.setState(StateWhitespace)
	case unicode.IsDigit(char):
		t.setState(StateNumeric)
	case unicode.IsLetter(char) || char == '/':
		t.setState(StateWord)
	default:
		t.setState(StateSpecial)
	}

	t.addToBuffer(char)
	t.pos++
	return true
}

// handleWordState processes word tokens
func (t *Tokenizer) handleWordState(char rune) bool {
	if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-' ||
		char == '.' || char == '@' || char == '/' ||
		(char == ':' && t.isURLScheme()) {
		t.addToBuffer(char)
		t.pos++
		return true
	}

	t.createWordToken()
	t.setState(StateStart)
	return true
}

// handleNumericState processes numeric tokens
// Allows digits and special chars for dates (2024-01-15), times (10:30:45), IPs (192.168.1.1)
func (t *Tokenizer) handleNumericState(char rune) bool {
	switch {
	case unicode.IsDigit(char), char == '.', char == '-', char == '/', char == ':':
		t.addToBuffer(char)
		t.pos++
		return true
	default:
		t.createNumericToken()
		t.setState(StateStart)
		return true
	}
}

// handleWhitespaceState processes whitespace
func (t *Tokenizer) handleWhitespaceState(char rune) bool {
	switch {
	case unicode.IsSpace(char):
		t.addToBuffer(char)
		t.pos++
		return true
	default:
		t.createWhitespaceToken()
		t.setState(StateStart)
		return true
	}
}

// handleSpecialState processes special characters
func (t *Tokenizer) handleSpecialState(char rune) bool {
	// Treat each special char as separate token
	t.addToBuffer(char)
	t.pos++
	t.createSpecialToken()
	t.setState(StateStart)
	return true
}

// classifyToken attempts to classify a single token's type using terminal rules
// Takes a token value and returns a more specific type if a rule matches, or TokenUnknown
func (t *Tokenizer) classifyToken(value string) token.TokenType {
	return globalTrie.Match(value)
}

// parseDateComponents extracts structural information from date strings
// Uses the same comprehensive patterns as the multiline aggregation package
func parseDateComponents(dateStr string) *token.DateComponents {
	// Comprehensive date patterns from multiline aggregation package
	patterns := []struct {
		regex  *regexp.Regexp
		format string
		parser func([]string) *token.DateComponents
	}{
		// RFC3339: 2006-01-02T15:04:05Z07:00
		{
			regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[\+\-]\d{2}:?\d{2})?`),
			"RFC3339",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Year: matches[1], Month: matches[2], Day: matches[3],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "RFC3339",
				}
			},
		},
		// Standard timestamp: 2021-07-08 05:08:19,214
		{
			regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})(,\d+)?`),
			"YYYY-MM-DD HH:mm:ss",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Year: matches[1], Month: matches[2], Day: matches[3],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "YYYY-MM-DD HH:mm:ss",
				}
			},
		},
		// Date only: 2021-01-31 (with strict month/day validation)
		{
			regexp.MustCompile(`^(\d{4})-(1[012]|0?[1-9])-([12][0-9]|3[01]|0?[1-9])`),
			"YYYY-MM-DD",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Year: matches[1], Month: matches[2], Day: matches[3],
					Format: "YYYY-MM-DD",
				}
			},
		},
		// Slash format: 2023/02/20 14:33:24
		{
			regexp.MustCompile(`^(\d{4})/(\d{2})/(\d{2}) (\d{2}):(\d{2}):(\d{2})`),
			"YYYY/MM/DD HH:mm:ss",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Year: matches[1], Month: matches[2], Day: matches[3],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "YYYY/MM/DD HH:mm:ss",
				}
			},
		},
		// Java SimpleFormatter: January 31, 2021 2:30:45 PM
		{
			regexp.MustCompile(`^([A-Za-z_]+) (\d+), (\d{4}) (\d+):(\d+):(\d+) (AM|PM)`),
			"Month DD, YYYY HH:mm:ss AM/PM",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Month: matches[1], Day: matches[2], Year: matches[3],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "Month DD, YYYY HH:mm:ss AM/PM",
				}
			},
		},
		// ANSIC: Mon Jan _2 15:04:05 2006
		{
			regexp.MustCompile(`^([A-Za-z_]+) ([A-Za-z_]+) +(\d+) (\d+):(\d+):(\d+) (\d+)`),
			"ANSIC",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Month: matches[2], Day: matches[3], Year: matches[7],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "ANSIC",
				}
			},
		},
		// UnixDate: Mon Jan _2 15:04:05 MST 2006
		{
			regexp.MustCompile(`^([A-Za-z_]+) ([A-Za-z_]+) +(\d+) (\d+):(\d+):(\d+)( [A-Za-z_]+ (\d+))?`),
			"UnixDate",
			func(matches []string) *token.DateComponents {
				year := matches[7]
				if year == "" && len(matches) > 8 {
					year = matches[8]
				}
				return &token.DateComponents{
					Month: matches[2], Day: matches[3], Year: year,
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "UnixDate",
				}
			},
		},
		// RubyDate: Mon Jan 02 15:04:05 -0700 2006
		{
			regexp.MustCompile(`^([A-Za-z_]+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) ([\-\+]\d+) (\d+)`),
			"RubyDate",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Month: matches[2], Day: matches[3], Year: matches[8],
					Hour: matches[4], Minute: matches[5], Second: matches[6],
					Format: "RubyDate",
				}
			},
		},
		// RFC822: 02 Jan 06 15:04 MST
		{
			regexp.MustCompile(`^(\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+) ([A-Za-z_]+)`),
			"RFC822",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Day: matches[1], Month: matches[2], Year: matches[3],
					Hour: matches[4], Minute: matches[5],
					Format: "RFC822",
				}
			},
		},
		// RFC822Z: 02 Jan 06 15:04 -0700
		{
			regexp.MustCompile(`^(\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+) (-\d+)`),
			"RFC822Z",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Day: matches[1], Month: matches[2], Year: matches[3],
					Hour: matches[4], Minute: matches[5],
					Format: "RFC822Z",
				}
			},
		},
		// RFC850: Monday, 02-Jan-06 15:04:05 MST
		{
			regexp.MustCompile(`^([A-Za-z_]+), (\d+)-([A-Za-z_]+)-(\d+) (\d+):(\d+):(\d+) ([A-Za-z_]+)`),
			"RFC850",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Day: matches[2], Month: matches[3], Year: matches[4],
					Hour: matches[5], Minute: matches[6], Second: matches[7],
					Format: "RFC850",
				}
			},
		},
		// RFC1123: Mon, 02 Jan 2006 15:04:05 MST
		{
			regexp.MustCompile(`^([A-Za-z_]+), (\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) ([A-Za-z_]+)`),
			"RFC1123",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Day: matches[2], Month: matches[3], Year: matches[4],
					Hour: matches[5], Minute: matches[6], Second: matches[7],
					Format: "RFC1123",
				}
			},
		},
		// RFC1123Z: Mon, 02 Jan 2006 15:04:05 -0700
		{
			regexp.MustCompile(`^([A-Za-z_]+), (\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) (-\d+)`),
			"RFC1123Z",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Day: matches[2], Month: matches[3], Year: matches[4],
					Hour: matches[5], Minute: matches[6], Second: matches[7],
					Format: "RFC1123Z",
				}
			},
		},
		// RFC3339Nano: 2006-01-02T15:04:05.999999999Z07:00
		{
			regexp.MustCompile(`^(\d+)-(\d+)-(\d+)([A-Za-z_]+)(\d+):(\d+):(\d+)\.(\d+)([A-Za-z_]+)(\d+):(\d+)`),
			"RFC3339Nano",
			func(matches []string) *token.DateComponents {
				return &token.DateComponents{
					Year: matches[1], Month: matches[2], Day: matches[3],
					Hour: matches[5], Minute: matches[6], Second: matches[7],
					Format: "RFC3339Nano",
				}
			},
		},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(dateStr); matches != nil {
			return pattern.parser(matches)
		}
	}

	return nil // Couldn't parse
}

// hasNumericPattern checks if a word contains numbers
func hasNumericPattern(word string) bool {
	return regexp.MustCompile(`\d`).MatchString(word)
}

// shouldSetPossiblyWildcard determines if a token should have the possiblyWildcard flag
// Words with numeric patterns (user123, admin456) can be wildcarded during merging
func shouldSetPossiblyWildcard(tokenType token.TokenType, value string) bool {
	return tokenType == token.TokenWord && hasNumericPattern(value)
}

// Helper functions

// isURLScheme checks if current buffer looks like a URL scheme
func (t *Tokenizer) isURLScheme() bool {
	buffer := string(t.buffer)
	return buffer == "http" || buffer == "https"
}

// State management helpers

func (t *Tokenizer) setState(newState TokenizerState) {
	t.state = newState
}

func (t *Tokenizer) addToBuffer(char rune) {
	t.buffer = append(t.buffer, char)
}

func (t *Tokenizer) clearBuffer() {
	t.buffer = t.buffer[:0] // Keep capacity, reset length
}

func (t *Tokenizer) bufferToString() string {
	return string(t.buffer)
}

func (t *Tokenizer) flushBuffer() {
	if len(t.buffer) > 0 {
		// Create remaining content as word token
		t.createWordToken()
	}
}

// Token creation methods

func (t *Tokenizer) createWordToken() {
	value := t.bufferToString()
	tokenType := t.classifyToken(value)

	tok := token.NewTokenWithFlags(tokenType, value, false, shouldSetPossiblyWildcard(tokenType, value))
	t.tokens = append(t.tokens, tok)
	t.clearBuffer()
}

func (t *Tokenizer) createNumericToken() {
	value := t.bufferToString()
	t.tokens = append(t.tokens, token.Token{
		Type:             token.TokenNumeric,
		Value:            value,
		PossiblyWildcard: true, // Numeric tokens can be merged (25 vs 62 → *)
	})
	t.clearBuffer()
}

func (t *Tokenizer) createWhitespaceToken() {
	value := t.bufferToString()
	t.tokens = append(t.tokens, token.Token{
		Type:             token.TokenWhitespace,
		Value:            value,
		PossiblyWildcard: false, // Whitespace tokens are not mergeable
	})
	t.clearBuffer()
}

func (t *Tokenizer) createSpecialToken() {
	value := t.bufferToString()
	tokenType := t.classifyToken(value)

	t.tokens = append(t.tokens, token.Token{
		Type:             tokenType,
		Value:            value,
		PossiblyWildcard: shouldSetPossiblyWildcard(tokenType, value),
	})
	t.clearBuffer()
}
