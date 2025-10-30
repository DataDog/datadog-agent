// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides log message tokenization using finite state automaton
// and pattern matching for semantic token classification.
package automaton

import (
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

	t.handleLastToken()
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

		// Skip classification for punctuation (already marked as NotWildcard in createSpecialToken)
		if tok.Wildcard == token.NotWildcard {
			continue
		}

		classifiedType := t.classifyToken(tok.Value)

		// If classification returns TokenWord or TokenUnknown, keep current state
		// TokenWord = "generic word, no specific classification"
		// TokenUnknown = "should not happen, but keep current state"
		if classifiedType == token.TokenWord || classifiedType == token.TokenUnknown {
			continue
		}

		// Update token type to the more specific classification
		t.tokens[i].Type = classifiedType

		// Set wildcard potential based on classified type
		t.tokens[i].Wildcard = getWildcardPotential(classifiedType)

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

// getWildcardPotential determines if a token type can potentially become a wildcard
// Returns either NotWildcard (0%) or PotentialWildcard (50%)
// Note: IsWildcard (100%) is only set during pattern merging, never during tokenization
func getWildcardPotential(tokenType token.TokenType) token.WildcardStatus {
	// Only whitespace cannot become a wildcard
	if tokenType == token.TokenWhitespace {
		return token.NotWildcard
	}

	// Everything else can potentially become wildcards
	// Dates wildcard if they have the same format (both TokenDate means same structure)
	return token.PotentialWildcard
}

// ================================================
// Helper functions
// ================================================

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

func (t *Tokenizer) handleLastToken() {
	if len(t.buffer) > 0 {
		// Create token from remaining buffer content based on current state
		switch t.state {
		case StateNumeric:
			t.createNumericToken()
		case StateWhitespace:
			t.createWhitespaceToken()
		case StateSpecial:
			t.createSpecialToken()
		default:
			t.createWordToken()
		}
	}
}

// ================================================
// Token creation methods
// ================================================

func (t *Tokenizer) createWordToken() {
	value := t.bufferToString()
	// Create as basic Word type - classification happens later in classifyTokens()
	tok := token.NewToken(token.TokenWord, value, token.PotentialWildcard)
	t.tokens = append(t.tokens, tok)
	t.clearBuffer()
}

func (t *Tokenizer) createNumericToken() {
	value := t.bufferToString()
	// Numeric tokens are potential wildcards - will be classified later
	tok := token.NewToken(token.TokenNumeric, value, token.PotentialWildcard)
	t.tokens = append(t.tokens, tok)
	t.clearBuffer()
}

func (t *Tokenizer) createWhitespaceToken() {
	value := t.bufferToString()
	// Whitespace never becomes wildcard
	tok := token.NewToken(token.TokenWhitespace, value, token.NotWildcard)
	t.tokens = append(t.tokens, tok)
	t.clearBuffer()
}

func (t *Tokenizer) createSpecialToken() {
	value := t.bufferToString()
	// Special characters (punctuation, symbols) should not wildcard - only merge if identical
	// Examples: ":", "[", "@" - structural markers that must stay consistent
	tok := token.NewToken(token.TokenWord, value, token.NotWildcard)
	t.tokens = append(t.tokens, tok)
	t.clearBuffer()
}
