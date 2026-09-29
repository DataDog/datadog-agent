// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import "github.com/DataDog/datadog-agent/pkg/logs/tokenizer"

// The tokenizer lives in pkg/logs/tokenizer. The aliases and wrappers in this
// file keep existing callers compiling unchanged while they migrate one at a
// time. New code should import pkg/logs/tokenizer directly; this file goes
// away once no caller uses it.

// Token is an alias of tokenizer.Token.
type Token = tokenizer.Token

// Tokenizer is an alias of tokenizer.Tokenizer.
type Tokenizer = tokenizer.Tokenizer

// BorrowedTokens is an alias of tokenizer.BorrowedTokens.
type BorrowedTokens = tokenizer.BorrowedTokens

// NewTokenizer calls tokenizer.NewTokenizer.
func NewTokenizer(maxEvalBytes int) *Tokenizer { return tokenizer.NewTokenizer(maxEvalBytes) }

// NewBorrowedTokens calls tokenizer.NewBorrowedTokens.
func NewBorrowedTokens(tokens []Token, indices []int) BorrowedTokens {
	return tokenizer.NewBorrowedTokens(tokens, indices)
}

// TokensToString calls tokenizer.TokensToString.
func TokensToString(tokens []Token) string { return tokenizer.TokensToString(tokens) }

// IsMatch calls tokenizer.IsMatch.
func IsMatch(seqA []Token, seqB []Token, thresh float64) bool {
	return tokenizer.IsMatch(seqA, seqB, thresh)
}

// IsImportant calls tokenizer.IsImportant.
func IsImportant(tokens []Token) bool { return tokenizer.IsImportant(tokens) }

// Token constants, re-exported one for one from pkg/logs/tokenizer.
//
//revive:disable
const (
	Space        = tokenizer.Space
	Colon        = tokenizer.Colon
	Semicolon    = tokenizer.Semicolon
	Dash         = tokenizer.Dash
	Underscore   = tokenizer.Underscore
	Fslash       = tokenizer.Fslash
	Bslash       = tokenizer.Bslash
	Period       = tokenizer.Period
	Comma        = tokenizer.Comma
	Singlequote  = tokenizer.Singlequote
	Doublequote  = tokenizer.Doublequote
	Backtick     = tokenizer.Backtick
	Tilda        = tokenizer.Tilda
	Star         = tokenizer.Star
	Plus         = tokenizer.Plus
	Equal        = tokenizer.Equal
	Parenopen    = tokenizer.Parenopen
	Parenclose   = tokenizer.Parenclose
	Braceopen    = tokenizer.Braceopen
	Braceclose   = tokenizer.Braceclose
	Bracketopen  = tokenizer.Bracketopen
	Bracketclose = tokenizer.Bracketclose
	Ampersand    = tokenizer.Ampersand
	Exclamation  = tokenizer.Exclamation
	At           = tokenizer.At
	Pound        = tokenizer.Pound
	Dollar       = tokenizer.Dollar
	Percent      = tokenizer.Percent
	Uparrow      = tokenizer.Uparrow
	D1           = tokenizer.D1
	D2           = tokenizer.D2
	D3           = tokenizer.D3
	D4           = tokenizer.D4
	D5           = tokenizer.D5
	D6           = tokenizer.D6
	D7           = tokenizer.D7
	D8           = tokenizer.D8
	D9           = tokenizer.D9
	D10          = tokenizer.D10
	C1           = tokenizer.C1
	C2           = tokenizer.C2
	C3           = tokenizer.C3
	C4           = tokenizer.C4
	C5           = tokenizer.C5
	C6           = tokenizer.C6
	C7           = tokenizer.C7
	C8           = tokenizer.C8
	C9           = tokenizer.C9
	C10          = tokenizer.C10
	Month        = tokenizer.Month
	Day          = tokenizer.Day
	Apm          = tokenizer.Apm
	Zone         = tokenizer.Zone
	T            = tokenizer.T
	Warn         = tokenizer.Warn
	Fatal        = tokenizer.Fatal
	Error        = tokenizer.Error
	Panic        = tokenizer.Panic
	Alert        = tokenizer.Alert
	Severe       = tokenizer.Severe
	Critical     = tokenizer.Critical
	Emergency    = tokenizer.Emergency
	Exception    = tokenizer.Exception
	Crash        = tokenizer.Crash
	Failure      = tokenizer.Failure
	Deadlock     = tokenizer.Deadlock
	Timeout      = tokenizer.Timeout
	IPv4         = tokenizer.IPv4
	End          = tokenizer.End
)

//revive:enable
