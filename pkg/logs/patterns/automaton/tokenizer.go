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

	t.flushBuffer()
	t.classifyTokens()

	return token.NewTokenList(t.tokens)
}

// classifyTokens applies terminal rules for token classification
func (t *Tokenizer) classifyTokens() {
	for i, tok := range t.tokens {
		// Only classify word-like and numeric tokens that might be structured
		if tok.Type == token.TokenWord || tok.Type == token.TokenNumeric {
			classifiedType := t.classifyToken(tok.Value)
			if classifiedType != token.TokenUnknown {
				t.tokens[i].Type = classifiedType
			}
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
	if unicode.IsSpace(char) {
		t.setState(StateWhitespace)
		t.addToBuffer(char)
	} else if unicode.IsDigit(char) {
		t.setState(StateNumeric)
		t.addToBuffer(char)
	} else if unicode.IsLetter(char) || char == '/' {
		t.setState(StateWord)
		t.addToBuffer(char)
	} else {
		t.setState(StateSpecial)
		t.addToBuffer(char)
	}

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
func (t *Tokenizer) handleNumericState(char rune) bool {
	if unicode.IsDigit(char) {
		t.addToBuffer(char)
		t.pos++
		return true
	} else if char == '.' {
		t.addToBuffer(char)
		t.pos++
		return true
	} else if char == '-' || char == '/' || char == ':' {
		t.addToBuffer(char)
		t.pos++
		return true
	} else {
		t.createNumericToken()
		t.setState(StateStart)
		return true
	}
}

// handleWhitespaceState processes whitespace
func (t *Tokenizer) handleWhitespaceState(char rune) bool {
	if unicode.IsSpace(char) {
		t.addToBuffer(char)
		t.pos++
		return true
	} else {
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

// classifyToken uses globalTrie.Match() for token classification
func (t *Tokenizer) classifyToken(value string) token.TokenType {
	return globalTrie.Match(value)
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

	t.tokens = append(t.tokens, token.Token{
		Type:  tokenType,
		Value: value,
	})
	t.clearBuffer()
}

func (t *Tokenizer) createNumericToken() {
	value := t.bufferToString()
	t.tokens = append(t.tokens, token.Token{
		Type:  token.TokenNumeric,
		Value: value,
	})
	t.clearBuffer()
}

func (t *Tokenizer) createWhitespaceToken() {
	value := t.bufferToString()
	t.tokens = append(t.tokens, token.Token{
		Type:  token.TokenWhitespace,
		Value: value,
	})
	t.clearBuffer()
}

func (t *Tokenizer) createSpecialToken() {
	value := t.bufferToString()
	tokenType := t.classifyToken(value)

	t.tokens = append(t.tokens, token.Token{
		Type:  tokenType,
		Value: value,
	})
	t.clearBuffer()
}
