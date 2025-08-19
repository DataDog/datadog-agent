// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens contains the token definitions for the tokenizer.
package tokens

// Token is the type that represents a single token.
type Token byte

// Disable linter since the token list is self explanatory, or documented where needed.
//
//revive:disable
const (
	Space Token = iota

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

	End // Not a valid token. Used to mark the end of the token list or as a terminator.
)

//revive:enable
