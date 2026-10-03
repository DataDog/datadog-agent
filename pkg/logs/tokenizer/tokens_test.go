// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTokenOrdinalsArePinned pins the numeric value of every Token. The
// adaptive sampler hashes raw token bytes into the log_hash tag, so moving a
// value changes customer-visible output. New types go immediately before End
// (and bump ProfileVersion); End may move, existing values may not.
func TestTokenOrdinalsArePinned(t *testing.T) {
	pinned := []struct {
		tok  Token
		want int
	}{
		{Space, 0},
		{Colon, 1},
		{Semicolon, 2},
		{Dash, 3},
		{Underscore, 4},
		{Fslash, 5},
		{Bslash, 6},
		{Period, 7},
		{Comma, 8},
		{Singlequote, 9},
		{Doublequote, 10},
		{Backtick, 11},
		{Tilda, 12},
		{Star, 13},
		{Plus, 14},
		{Equal, 15},
		{Parenopen, 16},
		{Parenclose, 17},
		{Braceopen, 18},
		{Braceclose, 19},
		{Bracketopen, 20},
		{Bracketclose, 21},
		{Ampersand, 22},
		{Exclamation, 23},
		{At, 24},
		{Pound, 25},
		{Dollar, 26},
		{Percent, 27},
		{Uparrow, 28},
		{D1, 29},
		{D2, 30},
		{D3, 31},
		{D4, 32},
		{D5, 33},
		{D6, 34},
		{D7, 35},
		{D8, 36},
		{D9, 37},
		{D10, 38},
		{C1, 39},
		{C2, 40},
		{C3, 41},
		{C4, 42},
		{C5, 43},
		{C6, 44},
		{C7, 45},
		{C8, 46},
		{C9, 47},
		{C10, 48},
		{Month, 49},
		{Day, 50},
		{Apm, 51},
		{Zone, 52},
		{T, 53},
		{Warn, 54},
		{Fatal, 55},
		{Error, 56},
		{Panic, 57},
		{Alert, 58},
		{Severe, 59},
		{Critical, 60},
		{Emergency, 61},
		{Exception, 62},
		{Crash, 63},
		{Failure, 64},
		{Deadlock, 65},
		{Timeout, 66},
		{IPv4, 67},
		{End, 68},
	}

	assert.Len(t, pinned, int(End)+1, "every Token, including End, must be pinned here")
	for _, p := range pinned {
		assert.Equal(t, p.want, int(p.tok), "Token %q moved", tokenToString(p.tok))
	}
	// IPv4 keeps its slot. Adding a Token moves End: update this table too.
	assert.Equal(t, 67, int(IPv4))
	assert.Equal(t, 68, int(End))
}
