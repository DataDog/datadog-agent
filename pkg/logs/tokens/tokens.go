// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens provides token types for log processing rules.
// This is a public re-export of the internal tokens package.
package tokens

import (
	internaltokens "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// Token represents a single token from tokenization
type Token = internaltokens.Token

// TokenKind represents the type of a token
type TokenKind = internaltokens.TokenKind

// Token kinds - re-export from internal package
const (
	Space        = internaltokens.Space
	Colon        = internaltokens.Colon
	Semicolon    = internaltokens.Semicolon
	Dash         = internaltokens.Dash
	Underscore   = internaltokens.Underscore
	Fslash       = internaltokens.Fslash
	Bslash       = internaltokens.Bslash
	Period       = internaltokens.Period
	Comma        = internaltokens.Comma
	Singlequote  = internaltokens.Singlequote
	Doublequote  = internaltokens.Doublequote
	Backtick     = internaltokens.Backtick
	Tilda        = internaltokens.Tilda
	Star         = internaltokens.Star
	Plus         = internaltokens.Plus
	Equal        = internaltokens.Equal
	Parenopen    = internaltokens.Parenopen
	Parenclose   = internaltokens.Parenclose
	Braceopen    = internaltokens.Braceopen
	Braceclose   = internaltokens.Braceclose
	Bracketopen  = internaltokens.Bracketopen
	Bracketclose = internaltokens.Bracketclose
	Ampersand    = internaltokens.Ampersand
	Exclamation  = internaltokens.Exclamation
	At           = internaltokens.At
	Pound        = internaltokens.Pound
	Dollar       = internaltokens.Dollar
	Percent      = internaltokens.Percent
	Uparrow      = internaltokens.Uparrow
	D1           = internaltokens.D1
	D2           = internaltokens.D2
	D3           = internaltokens.D3
	D4           = internaltokens.D4
	D5           = internaltokens.D5
	D6           = internaltokens.D6
	D7           = internaltokens.D7
	D8           = internaltokens.D8
	D9           = internaltokens.D9
	D10          = internaltokens.D10
	C1           = internaltokens.C1
	C2           = internaltokens.C2
	C3           = internaltokens.C3
	C4           = internaltokens.C4
	C5           = internaltokens.C5
	C6           = internaltokens.C6
	C7           = internaltokens.C7
	C8           = internaltokens.C8
	C9           = internaltokens.C9
	C10          = internaltokens.C10
	Month        = internaltokens.Month
	Day          = internaltokens.Day
	Apm          = internaltokens.Apm
	Zone         = internaltokens.Zone
	T            = internaltokens.T
	DAny         = internaltokens.DAny
	CAny         = internaltokens.CAny
	End          = internaltokens.End
)

// NewToken creates a token with the given kind and literal value
func NewToken(kind TokenKind, lit string) Token {
	return internaltokens.NewToken(kind, lit)
}

// NewSimpleToken creates a token with just a kind (no literal)
func NewSimpleToken(kind TokenKind) Token {
	return internaltokens.NewSimpleToken(kind)
}
