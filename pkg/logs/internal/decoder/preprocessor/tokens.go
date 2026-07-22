// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"

// Token aliases the shared log pattern token type for compatibility with the
// decoder preprocessor package.
type Token = logpattern.Token

const (
	Space = logpattern.Space

	Colon        = logpattern.Colon
	Semicolon    = logpattern.Semicolon
	Dash         = logpattern.Dash
	Underscore   = logpattern.Underscore
	Fslash       = logpattern.Fslash
	Bslash       = logpattern.Bslash
	Period       = logpattern.Period
	Comma        = logpattern.Comma
	Singlequote  = logpattern.Singlequote
	Doublequote  = logpattern.Doublequote
	Backtick     = logpattern.Backtick
	Tilda        = logpattern.Tilda
	Star         = logpattern.Star
	Plus         = logpattern.Plus
	Equal        = logpattern.Equal
	Parenopen    = logpattern.Parenopen
	Parenclose   = logpattern.Parenclose
	Braceopen    = logpattern.Braceopen
	Braceclose   = logpattern.Braceclose
	Bracketopen  = logpattern.Bracketopen
	Bracketclose = logpattern.Bracketclose
	Ampersand    = logpattern.Ampersand
	Exclamation  = logpattern.Exclamation
	At           = logpattern.At
	Pound        = logpattern.Pound
	Dollar       = logpattern.Dollar
	Percent      = logpattern.Percent
	Uparrow      = logpattern.Uparrow

	D1  = logpattern.D1
	D2  = logpattern.D2
	D3  = logpattern.D3
	D4  = logpattern.D4
	D5  = logpattern.D5
	D6  = logpattern.D6
	D7  = logpattern.D7
	D8  = logpattern.D8
	D9  = logpattern.D9
	D10 = logpattern.D10

	C1  = logpattern.C1
	C2  = logpattern.C2
	C3  = logpattern.C3
	C4  = logpattern.C4
	C5  = logpattern.C5
	C6  = logpattern.C6
	C7  = logpattern.C7
	C8  = logpattern.C8
	C9  = logpattern.C9
	C10 = logpattern.C10

	Month = logpattern.Month
	Day   = logpattern.Day
	Apm   = logpattern.Apm
	Zone  = logpattern.Zone
	T     = logpattern.T

	Warn      = logpattern.Warn
	Fatal     = logpattern.Fatal
	Error     = logpattern.Error
	Panic     = logpattern.Panic
	Alert     = logpattern.Alert
	Severe    = logpattern.Severe
	Critical  = logpattern.Critical
	Emergency = logpattern.Emergency
	Exception = logpattern.Exception
	Crash     = logpattern.Crash
	Failure   = logpattern.Failure
	Deadlock  = logpattern.Deadlock
	Timeout   = logpattern.Timeout

	End = logpattern.End
)
