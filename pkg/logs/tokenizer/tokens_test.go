// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"encoding/binary"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestProfileVersionPinnedWithEnd ties ProfileVersion to the token set: adding
// a Token moves End, and the same change must bump ProfileVersion. Update both
// values here together.
func TestProfileVersionPinnedWithEnd(t *testing.T) {
	assert.Equal(t, [2]int{68, 1}, [2]int{int(End), ProfileVersion},
		"End moved or ProfileVersion changed: a new Token needs a ProfileVersion bump, and both values move together")
}

// profileCorpus exercises every part of the tokenizer: every Token, every byte
// value, every run length up to and past the cap, every keyword, IPv4 edge
// cases, punctuation, whitespace variants and non-ASCII input.
var profileCorpus = []string{
	"",
	"2026-09-11 14:03:27 ERROR checkout failed for request 48192",
	"2026-09-11T14:03:27.123Z INFO started pid=4821",
	"Mon Jan  2 15:04:05 PM UTC 2006",
	"Tue Feb 03 03:04:05 am PST",
	"jan FEB mar apr may jun jul aug sep oct nov dec",
	"sun mon tue wed thu fri sat",
	"warn warning fatal error err panic alert severe crit critical emerg emergency",
	"exception crash failure failed deadlock timeout",
	"GMT EST EDT CST CDT MST MDT PDT JST KST IST MSK CET BST HST HDT NST NDT CEST NZST NZDT ACST ACDT AEST AEDT AWST AWDT AKST AKDT CHST CHDT crashed",
	"from 10.0.0.1 to 192.168.100.200",
	"1.2.3.4.5 1234.1.2.3.4 1..2.3.4 999.1.1.1",
	"12345678901234567890 abcdefghijklmnopqrstuvwxyz",
	"!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
	"tab\there\nnewline\rreturn\vvtab\fform feed",
	"caf\u00e9 \u00a0nbsp \u0085nel \U0001F600 emoji",
	`{"level":"error","msg":"connection refused","port":8080}`,
	"goroutine 1 [running]:\n\tmain.main()\n\t\t/app/main.go:42 +0x1d",
	"GET /api/v1/users/3f2a9c4e-1b2d-4c5e-8f9a-0b1c2d3e4f5a?page=2 HTTP/1.1 200",
	"1 12 123 1234 12345 123456 1234567 12345678 123456789 1234567890",
	everyByte(),
}

// everyByte returns one string holding each byte value 0x00-0xff once, so a
// change to any byte's class changes the digest.
func everyByte() string {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return string(b)
}

// profileGolden maps each ProfileVersion to a digest of Tokenize's output
// (tokens, start offsets and their TokensToString form) over profileCorpus.
// Version 1 is the output of the tokenizer before it moved out of
// pkg/logs/internal/decoder/preprocessor (same digest at 84cb28203e1).
var profileGolden = map[int]uint64{
	1: 0xd7e22cc1190a86eb,
}

// TestProfileVersionMatchesTokenizerOutput fails when Tokenize's output, or its
// TokensToString form, changes without a ProfileVersion bump. If you changed
// the output on purpose: bump ProfileVersion, add the new digest for the new
// version to profileGolden, and keep the old entries.
func TestProfileVersionMatchesTokenizerOutput(t *testing.T) {
	h := fnv.New64a()
	tok := NewTokenizer(0)
	var buf [4]byte
	for _, line := range profileCorpus {
		tokens, indices := tok.Tokenize([]byte(line))
		for i, tk := range tokens {
			binary.LittleEndian.PutUint32(buf[:], uint32(indices[i]))
			_, _ = h.Write([]byte{byte(tk)})
			_, _ = h.Write(buf[:])
		}
		_, _ = h.Write([]byte(TokensToString(tokens))) // consumers may hash this form
		_, _ = h.Write([]byte{0xff})                   // line separator; not a Token value
	}

	want, ok := profileGolden[ProfileVersion]
	require.True(t, ok, "no golden digest for ProfileVersion %d: add one to profileGolden", ProfileVersion)
	assert.Equal(t, want, h.Sum64(),
		"Tokenize output changed: bump ProfileVersion and add the new digest to profileGolden (got %#x)", h.Sum64())
}

// TestProfileCorpusCoversEveryToken keeps the golden test meaningful: a Token
// that profileCorpus never produces could change without moving the digest.
func TestProfileCorpusCoversEveryToken(t *testing.T) {
	seen := make([]bool, End)
	tok := NewTokenizer(0)
	for _, line := range profileCorpus {
		tokens, _ := tok.Tokenize([]byte(line))
		for _, tk := range tokens {
			seen[tk] = true
		}
	}
	for v := Token(0); v < End; v++ {
		assert.True(t, seen[v], "profileCorpus never produces %s: add a line that does", tokenToString(v))
	}
}
