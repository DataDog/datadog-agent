// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import "strings"

// tokenDebug and importantToken are lookup tables built once from the generated
// data (tokenMeta, specialChars in token_tables_gen.go). They back tokenToString
// and isImportant with a plain array index instead of a switch.
var (
	tokenDebug     [256]string
	importantToken [256]bool
)

func init() {
	// Digit and character runs render as repeated D/C, one entry per run length.
	for i := range maxRun {
		tokenDebug[D1+Token(i)] = strings.Repeat("D", i+1)
		tokenDebug[C1+Token(i)] = strings.Repeat("C", i+1)
	}
	tokenDebug[IPv4] = "IPv4"
	for _, m := range tokenMeta {
		tokenDebug[m.tok] = m.debug
		importantToken[m.tok] = m.critical
	}
	for _, c := range specialChars {
		tokenDebug[c.tok] = string(c.ch)
	}
}

// tokenToString returns the debug string for a single token ("" if it has none).
func tokenToString(token Token) string {
	return tokenDebug[token]
}

// isImportant reports whether any token is a critical-severity keyword; such
// logs bypass adaptive sampling.
func isImportant(tokens []Token) bool {
	for _, t := range tokens {
		if importantToken[t] {
			return true
		}
	}
	return false
}
