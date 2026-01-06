// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens contains the token definitions for the tokenizer.
package tokens

import patterntokens "github.com/DataDog/datadog-agent/pkg/logs/pattern/tokens"

// Token is the type that represents a single token.
// It is an alias to the non-internal token type so other packages (e.g. observer) can share signature logic.
type Token = patterntokens.Token

// Disable linter since the token list is self explanatory, or documented where needed.
//
//revive:disable
const (
	Space        = patterntokens.Space
	Colon        = patterntokens.Colon
	Semicolon    = patterntokens.Semicolon
	Dash         = patterntokens.Dash
	Underscore   = patterntokens.Underscore
	Fslash       = patterntokens.Fslash
	Bslash       = patterntokens.Bslash
	Period       = patterntokens.Period
	Comma        = patterntokens.Comma
	Singlequote  = patterntokens.Singlequote
	Doublequote  = patterntokens.Doublequote
	Backtick     = patterntokens.Backtick
	Tilda        = patterntokens.Tilda
	Star         = patterntokens.Star
	Plus         = patterntokens.Plus
	Equal        = patterntokens.Equal
	Parenopen    = patterntokens.Parenopen
	Parenclose   = patterntokens.Parenclose
	Braceopen    = patterntokens.Braceopen
	Braceclose   = patterntokens.Braceclose
	Bracketopen  = patterntokens.Bracketopen
	Bracketclose = patterntokens.Bracketclose
	Ampersand    = patterntokens.Ampersand
	Exclamation  = patterntokens.Exclamation
	At           = patterntokens.At
	Pound        = patterntokens.Pound
	Dollar       = patterntokens.Dollar
	Percent      = patterntokens.Percent
	Uparrow      = patterntokens.Uparrow

	D1  = patterntokens.D1
	D2  = patterntokens.D2
	D3  = patterntokens.D3
	D4  = patterntokens.D4
	D5  = patterntokens.D5
	D6  = patterntokens.D6
	D7  = patterntokens.D7
	D8  = patterntokens.D8
	D9  = patterntokens.D9
	D10 = patterntokens.D10

	C1  = patterntokens.C1
	C2  = patterntokens.C2
	C3  = patterntokens.C3
	C4  = patterntokens.C4
	C5  = patterntokens.C5
	C6  = patterntokens.C6
	C7  = patterntokens.C7
	C8  = patterntokens.C8
	C9  = patterntokens.C9
	C10 = patterntokens.C10

	Month = patterntokens.Month
	Day   = patterntokens.Day
	Apm   = patterntokens.Apm
	Zone  = patterntokens.Zone
	T     = patterntokens.T

	End = patterntokens.End
)

//revive:enable
