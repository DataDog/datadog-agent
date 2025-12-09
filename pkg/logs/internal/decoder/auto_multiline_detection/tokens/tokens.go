// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens contains the token definitions for the tokenizer.
package tokens

// TokenKind represents the type/category of a token
type TokenKind byte

// Token represents a single token with its kind and optional literal value
type Token struct {
	Kind TokenKind
	Lit  string // Literal string value (for character/digit runs and special recognition)
}

// Disable linter since the token list is self explanatory, or documented where needed.
//
//revive:disable
const (
	Space TokenKind = iota

	// Special Characters
	Colon        // :
	Semicolon    // ;
	Dash         // -
	Underscore   // _
	Fslash       // /
	Bslash       // \
	Period       // .
	Comma        // ,
	Singlequote  // '
	Doublequote  // "
	Backtick     // `
	Tilda        // ~
	Star         // *
	Plus         // +
	Equal        // =
	Parenopen    // (
	Parenclose   // )
	Braceopen    // {
	Braceclose   // }
	Bracketopen  // [
	Bracketclose // ]
	Ampersand    // &
	Exclamation  // !
	At           // @
	Pound        // #
	Dollar       // $
	Percent      // %
	Uparrow      // ^

	// Digit runs
	D1
	D2
	D3
	D4
	D5
	D6
	D7
	D8
	D9
	D10

	// Char runs
	C1
	C2
	C3
	C4
	C5
	C6
	C7
	C8
	C9
	C10

	// Special tokens
	Month
	Day
	Apm  // am or pm
	Zone // Represents a timezone
	T    // t (often `T`) denotes a time separator in many timestamp formats

	// Wildcard tokens for matching variable-length patterns
	DAny // Matches any digit run (D1-D10) - used with length constraints
	CAny // Matches any character run (C1-C10) - used with length constraints

	End // Not a valid token. Used to mark the end of the token list or as a terminator.
)

//revive:enable

// NewToken creates a token with the given kind and literal value
func NewToken(kind TokenKind, lit string) Token {
	return Token{Kind: kind, Lit: lit}
}

// NewSimpleToken creates a token with just a kind (no literal value)
func NewSimpleToken(kind TokenKind) Token {
	return Token{Kind: kind, Lit: ""}
}

// Equals compares two tokens for equality
// If both tokens have literals, compares both kind and literal
// If pattern token has no literal, only compares kind
// Supports wildcard matching for DAny and CAny
func (t Token) Equals(pattern Token) bool {
	// Handle wildcard digit matching (DAny matches any D1-D10)
	if pattern.Kind == DAny && t.Kind >= D1 && t.Kind <= D10 {
		return true
	}

	// Handle wildcard character matching (CAny matches any C1-C10)
	if pattern.Kind == CAny && t.Kind >= C1 && t.Kind <= C10 {
		return true
	}

	// Normal kind matching
	if t.Kind != pattern.Kind {
		return false
	}

	// If pattern has a literal value, it must match exactly
	if pattern.Lit != "" {
		return t.Lit == pattern.Lit
	}

	// Pattern has no literal, so kind match is sufficient
	return true
}

// EqualsKind checks if token has the given kind (ignoring literal value)
func (t Token) EqualsKind(kind TokenKind) bool {
	return t.Kind == kind
}
