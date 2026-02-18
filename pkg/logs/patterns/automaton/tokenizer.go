// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides log message tokenization using finite state automaton
// and pattern matching for semantic token classification.
package automaton

import (
	"fmt"
	"sync"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TokenizerState represents the current state of the FSA
type TokenizerState int

const (
	StateStart      TokenizerState = iota // StateStart is the initial state
	StateWord                             // StateWord is letters, digits, and common separators for structured tokens
	StateNumeric                          // StateNumeric is pure numbers
	StateWhitespace                       // StateWhitespace is spaces, tabs, newlines
	StateSpecial                          // StateSpecial is operators, punctuation, symbols
)

// NOTE + TODO: These numbers could be ran with some more testing on more log samples to optimize these values. Potentially add telemetry in staging to track usage and tune these values.
const (
	// tokenizerBufferCapacity is the initial capacity for the byte buffer.
	tokenizerBufferCapacity = 128

	// tokenizerTokensCapacity is the initial capacity for the tokens slice.
	tokenizerTokensCapacity = 24

	// Limits prevent retaining extremely large buffers in the pool, resetting to initial capacity.
	tokenizerMaxBufferCapacity = 4096
	tokenizerMaxTokensCapacity = 512
)

var tokenizerPool = sync.Pool{
	New: func() any {
		return &Tokenizer{
			buffer: make([]byte, 0, tokenizerBufferCapacity),
			tokens: make([]token.Token, 0, tokenizerTokensCapacity),
		}
	},
}

// Tokenizer implements a finite state automaton for log tokenization.
// It implements the token.Tokenizer interface.
type Tokenizer struct {
	input  string
	pos    int
	length int
	state  TokenizerState
	buffer []byte
	tokens []token.Token
}

// AutomatonTokenizer is a stateless wrapper that implements token.Tokenizer interface.
// It uses the internal sync.Pool for efficiency.
type AutomatonTokenizer struct{}

// NewTokenizer creates a new automaton tokenizer that implements token.Tokenizer interface.
// The returned tokenizer is stateless and safe to reuse.
func NewTokenizer() token.Tokenizer {
	return &AutomatonTokenizer{}
}

// Tokenize implements token.Tokenizer.Tokenize.
// It gets a tokenizer from the pool, processes the log, and returns it to the pool.
func (at *AutomatonTokenizer) Tokenize(log string) (*token.TokenList, error) {
	return newTokenizerInternal(log).tokenize(), nil
}

// TokenizeBatch implements token.Tokenizer.TokenizeBatch.
// It processes multiple logs sequentially, reusing pool instances for each.
func (at *AutomatonTokenizer) TokenizeBatch(logs []string) ([]token.TokenizeResult, error) {
	results := make([]token.TokenizeResult, len(logs))
	for i, log := range logs {
		t := newTokenizerInternal(log)
		tokenList := t.tokenize()
		t.Release()
		results[i] = token.TokenizeResult{
			TokenList: tokenList,
			Err:       nil,
		}
	}
	return results, nil
}

// newTokenizerInternal creates a new tokenizer for the given input from the pool (internal use)
func newTokenizerInternal(input string) *Tokenizer {
	tokenizer := tokenizerPool.Get().(*Tokenizer)
	tokenizer.reset(input)
	return tokenizer
}

// tokenize processes the input string and returns a TokenList (internal method)
func (t *Tokenizer) tokenize() *token.TokenList {
	for t.pos < t.length {
		if !t.processNextToken() {
			break
		}
	}

	t.handleLastToken()
	t.classifyTokens()

	tokens := t.tokens
	t.tokens = make([]token.Token, 0, tokenizerTokensCapacity)
	return token.NewTokenListWithTokens(tokens)
}

// classifyTokens upgrades generic tokens to specific types.
// The FSA first creates generic tokens (TokenWord, TokenNumeric), then this function uses
// pattern matching to identify structured types:
//   - "192.168.1.1" → TokenNumeric upgraded to TokenIPv4
//   - "user@example.com" → TokenWord upgraded to TokenEmail
//   - "GET" → TokenWord upgraded to TokenHTTPMethod
func (t *Tokenizer) classifyTokens() {
	for i, tok := range t.tokens {
		// Skip if not eligible for classification
		if !t.shouldClassify(&tok) {
			continue
		}

		// identify specific structured types (IP, Email, Date, HTTP, etc.)
		// fallback to word token if can't upgrade to specific type
		classifiedType, err := t.classifyToken(tok.Value)
		if err != nil {
			log.Warnf("Failed to classify token '%s': %v. Falling back to word token type", tok.Value, err)
			continue
		}

		// fallback to word token if can't upgrade to specific type
		if classifiedType == token.TokenWord {
			continue
		}

		// Upgrade token to the more specific type
		t.tokens[i].Type = classifiedType
		t.tokens[i].Wildcard = getWildcardPotential(classifiedType)
	}
}

// shouldClassify determines if a token is eligible for pattern-based classification.
// Returns true only for generic Word/Numeric tokens that are PotentialWildcard.
// Excludes: whitespace, punctuation (NotWildcard)
func (t *Tokenizer) shouldClassify(tok *token.Token) bool {
	isGenericType := tok.Type == token.TokenWord || tok.Type == token.TokenNumeric
	canVary := tok.Wildcard != token.NotWildcard

	return isGenericType && canVary
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
// Allows digits and special chars for dates (2024-01-15), times (10:30:45) or IPs (192.168.1.1)
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
func (t *Tokenizer) handleSpecialState(_ rune) bool {
	// The special character is already in buffer from handleStartState
	// Just create the token and reset state
	t.createSpecialToken()
	t.setState(StateStart)
	return true
}

// classifyToken attempts to classify a single token's type using trie and terminal rules.
func (t *Tokenizer) classifyToken(value string) (token.TokenType, error) {
	if len(value) == 0 {
		return token.TokenUnknown, fmt.Errorf("cannot classify empty string token value")
	}
	return globalTrie.Match(value), nil
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
	t.buffer = append(t.buffer, byte(char))
}

func (t *Tokenizer) clearBuffer() {
	t.buffer = t.buffer[:0] // Keep capacity, reset length
}

func (t *Tokenizer) bufferToString() string {
	return string(t.buffer)
}

// reset resets the tokenizer to the initial state.
func (t *Tokenizer) reset(input string) {
	t.input = input
	t.pos = 0
	t.length = len(input)
	t.state = StateStart
	t.resetBuffer()
	t.resetTokens()
}

// resetBuffer resets the buffer slice to the initial capacity.
func (t *Tokenizer) resetBuffer() {
	if cap(t.buffer) > tokenizerMaxBufferCapacity {
		t.buffer = make([]byte, 0, tokenizerBufferCapacity)
	} else {
		t.buffer = t.buffer[:0]
	}
}

// resetTokens resets the tokens slice to the initial capacity.
func (t *Tokenizer) resetTokens() {
	if cap(t.tokens) > tokenizerMaxTokensCapacity {
		t.tokens = make([]token.Token, 0, tokenizerTokensCapacity)
	} else {
		t.tokens = t.tokens[:0]
	}
}

// Release returns the tokenizer to the pool. The returned TokenList must not
// reference t.tokens after this is called.
func (t *Tokenizer) Release() {
	t.input = ""
	t.pos = 0
	t.length = 0
	t.state = StateStart
	t.resetBuffer()
	t.resetTokens()
	tokenizerPool.Put(t)
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
	// Normalize all whitespace (tabs, spaces, newlines, multiple spaces) to single space
	value := " "
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
