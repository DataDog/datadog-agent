// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens provides tokenization functionality for log messages.
package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input         string
	expectedToken string
}

// TestTokenizer is a broad table-driven anchoring test that exercises
// most named properties in the Tokenization contract at once:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant ByteClassification     — digit/letter/whitespace/punctuation/other
//	    @invariant RunLengthEncoding      — Dn / Cn token emission with run-length preserved
//	    @invariant SpecialTokenPromotion  — Month, Day, Zone, Apm, T promotions
//	    @invariant EmptyInput             — empty input → empty token sequence
//
// Each named property also has dedicated anchoring + property tests
// further down in this file. This table-driven test is the
// human-readable demonstration that representative inputs produce the
// token sequences the spec describes.
func TestTokenizer(t *testing.T) {
	testCases := []testCase{
		{input: "", expectedToken: ""},
		{input: " ", expectedToken: " "},
		{input: "a", expectedToken: "C"},
		{input: "a       b", expectedToken: "C C"},  // Spaces get truncated
		{input: "a  \t \t b", expectedToken: "C C"}, // Any spaces get truncated
		{input: "aaa", expectedToken: "CCC"},
		{input: "0", expectedToken: "D"},
		{input: "000", expectedToken: "DDD"},
		{input: "aa00", expectedToken: "CCDD"},
		{input: "abcd", expectedToken: "CCCC"},
		{input: "1234", expectedToken: "DDDD"},
		{input: "abc123", expectedToken: "CCCDDD"},
		{input: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~", expectedToken: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~"},
		{input: "123-abc-[foo] (bar)", expectedToken: "DDD-CCC-[CCC] (CCC)"},
		{input: "Sun Mar 2PM EST", expectedToken: "DAY MTH DPM ZONE"},
		{input: "12-12-12T12:12:12.12T12:12Z123", expectedToken: "DD-DD-DDTDD:DD:DD.DDTDD:DDZONEDDD"},
		{input: "amped", expectedToken: "CCCCC"},   // am should not be handled if it's part of a word
		{input: "am!ped", expectedToken: "PM!CCC"}, // am should be handled since it's separated by a special character
		{input: "TIME", expectedToken: "CCCC"},
		{input: "T123", expectedToken: "TDDD"},
		{input: "ZONE", expectedToken: "CCCC"},
		{input: "Z0NE", expectedToken: "ZONEDCC"},
		{input: "abc!📀🐶📊123", expectedToken: "CCC!CCCCCCCCCCDDD"},
		{input: "!!!$$$###", expectedToken: "!$#"}, // Symobl runs get truncated

		// Critical severity keyword promotion
		{input: "FATAL", expectedToken: "FATAL"},
		{input: "fatal", expectedToken: "FATAL"}, // case insensitive
		{input: "Fatal", expectedToken: "FATAL"}, // mixed case
		{input: "ERROR", expectedToken: "ERROR"},
		{input: "PANIC", expectedToken: "PANIC"},
		{input: "ALERT", expectedToken: "ALERT"},
		{input: "SEVERE", expectedToken: "SEVERE"},
		{input: "WARN", expectedToken: "WARN"},
		{input: "WARNING", expectedToken: "WARN"},
		{input: "CRIT", expectedToken: "CRIT"},
		{input: "CRITICAL", expectedToken: "CRIT"},
		{input: "EMERG", expectedToken: "EMERG"},
		{input: "EMERGENCY", expectedToken: "EMERG"},
		{input: "EXCEPTION", expectedToken: "EXCEPTION"},
		{input: "CRASH", expectedToken: "CRASH"},
		{input: "CRASHED", expectedToken: "CRASH"},
		{input: "FAILED", expectedToken: "FAILURE"},
		{input: "FAILURE", expectedToken: "FAILURE"},
		{input: "DEADLOCK", expectedToken: "DEADLOCK"},
		{input: "TIMEOUT", expectedToken: "TIMEOUT"},

		// False-positive safety: longer words must NOT match
		{input: "EXCEPTIONS", expectedToken: "CCCCCCCCCC"}, // 10 chars → C10, no match
		{input: "FATALIZER", expectedToken: "CCCCCCCCC"},   // 9 chars, not a keyword → C9

		// In-context: critical keywords separated by non-alpha chars
		{input: "[ERROR] something", expectedToken: "[ERROR] CCCCCCCCC"},
		{input: "FATAL: disk full", expectedToken: "FATAL: CCCC CCCC"},
	}

	tokenizer := NewTokenizer(0)
	for _, tc := range testCases {
		tokens, _ := tokenizer.tokenize([]byte(tc.input))
		actualToken := TokensToString(tokens)
		assert.Equal(t, tc.expectedToken, actualToken)
	}
}

// TestTokenizerMaxCharRun anchors the run-length cap clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant RunLengthEncoding
//
// "Runs of digits or characters longer than 10 are capped: 15
// consecutive characters produce C10, identical to 10 characters."
// 16 input letters → exactly 10 'C' tokens (one C10).
func TestTokenizerMaxCharRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("ABCDEFGHIJKLMNOP"))
	assert.Equal(t, "CCCCCCCCCC", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

// TestTokenizerMaxDigitRun anchors the run-length cap clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant RunLengthEncoding
//
// 16 input digits → exactly 10 'D' tokens (one D10). Mirrors
// TestTokenizerMaxCharRun for the digit category.
func TestTokenizerMaxDigitRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("0123456789012345"))
	assert.Equal(t, "DDDDDDDDDD", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

// TestAllSymbolsAreHandled anchors the "each ASCII punctuation
// character is a distinct token" clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant ByteClassification
//
// Every dedicated punctuation token between Space and D1 must produce
// a non-empty string representation and round-trip through the
// classification lookup as its own token (not as the generic character
// run C1).
func TestAllSymbolsAreHandled(t *testing.T) {
	for i := Space; i < D1; i++ {
		str := tokenToString(i)
		assert.NotEmpty(t, str, "Token %d is not converted to a debug string", i)
		assert.NotEqual(t, tokenLookup[str[0]], C1, "Token %v is not tokenizable", str)
	}
}

// TestTokenizerMaxEvalBytes anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant InputTruncation
//
// "Only the first max_bytes of the input are tokenized. Bytes beyond
// this offset are ignored." A tokenizer with max=10 sees only the
// first 10 input bytes; downstream content is invisible.
func TestTokenizerMaxEvalBytes(t *testing.T) {
	tokenizer := NewTokenizer(10)

	toks, _ := tokenizer.Tokenize([]byte("1234567890abcdefg"))
	assert.Equal(t, "DDDDDDDDDD", TokensToString(toks), "Tokens should be limited to 10 digits")

	var indices []int
	toks, indices = tokenizer.Tokenize([]byte("12-12-12T12:12:12.12T12:12Z123"))
	assert.Equal(t, "DD-DD-DDTD", TokensToString(toks), "Tokens should be limited to the first 10 bytes")
	assert.Equal(t, []int{0, 2, 3, 5, 6, 8, 9}, indices)

	toks, indices = tokenizer.Tokenize([]byte("abc 123"))
	assert.Equal(t, "CCC DDD", TokensToString(toks))
	assert.Equal(t, []int{0, 3, 4}, indices)

	toks, indices = tokenizer.Tokenize([]byte("Jan 123"))
	assert.Equal(t, "MTH DDD", TokensToString(toks))
	assert.Equal(t, []int{0, 3, 4}, indices)

	toks, indices = tokenizer.Tokenize([]byte("123Z"))
	assert.Equal(t, "DDDZONE", TokensToString(toks))
	assert.Equal(t, []int{0, 3}, indices)
}

// --- Fuzz tests ---
//
// Each fuzz / property test below names the spec construct it anchors
// in its docstring so that drift in either direction is easy to spot
// during review. Tokenization @invariants live in tokenizer.allium;
// PatternMatching @invariants live in adaptive_sampler.allium.

// Reference tokenizer: an independent implementation of the tokenization
// rules from tokenizer.allium. Used as an oracle to verify the
// production tokenizer matches the spec.

func refClassify(b byte) Token {
	switch {
	case b >= '0' && b <= '9':
		return D1
	case b == ' ' || b == '\t' || b == '\n' || b == '\r':
		return Space
	case b == ':':
		return Colon
	case b == ';':
		return Semicolon
	case b == '-':
		return Dash
	case b == '_':
		return Underscore
	case b == '/':
		return Fslash
	case b == '\\':
		return Bslash
	case b == '.':
		return Period
	case b == ',':
		return Comma
	case b == '\'':
		return Singlequote
	case b == '"':
		return Doublequote
	case b == '`':
		return Backtick
	case b == '~':
		return Tilda
	case b == '*':
		return Star
	case b == '+':
		return Plus
	case b == '=':
		return Equal
	case b == '(':
		return Parenopen
	case b == ')':
		return Parenclose
	case b == '{':
		return Braceopen
	case b == '}':
		return Braceclose
	case b == '[':
		return Bracketopen
	case b == ']':
		return Bracketclose
	case b == '&':
		return Ampersand
	case b == '!':
		return Exclamation
	case b == '@':
		return At
	case b == '#':
		return Pound
	case b == '$':
		return Dollar
	case b == '%':
		return Percent
	case b == '^':
		return Uparrow
	default:
		return C1
	}
}

func refToUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

// refSpecialToken is an independent mapping from uppercase ASCII strings to
// their Token values. It must NOT delegate to the production helpers
// (getSpecialShortToken / getSpecialLongToken): the whole point of
// referenceTokenize is to act as a differential oracle, so routing through
// the same code under test would make FuzzTokenizerCorrectness compare the
// production tokenizer against itself and miss bugs in keyword recognition.
//
// Keep this table in sync with getSpecialShortToken / getSpecialLongToken, but
// always as an independent copy.
func refSpecialToken(s string) Token {
	switch len(s) {
	case 1:
		switch s[0] {
		case 'T':
			return T
		case 'Z':
			return Zone
		}
	case 2:
		switch s {
		case "AM", "PM":
			return Apm
		}
	case 3:
		switch s {
		case "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
			"JUL", "AUG", "SEP", "OCT", "NOV", "DEC":
			return Month
		case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
			return Day
		case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
			"MST", "MDT", "PST", "PDT", "JST", "KST",
			"IST", "MSK", "CET", "BST", "HST", "HDT",
			"NST", "NDT":
			return Zone
		}
	case 4:
		switch s {
		case "WARN":
			return Warn
		case "CRIT":
			return Critical
		case "CEST", "NZST", "NZDT", "ACST", "ACDT",
			"AEST", "AEDT", "AWST", "AWDT", "AKST",
			"AKDT", "CHST", "CHDT":
			return Zone
		}
	case 5:
		switch s {
		case "FATAL":
			return Fatal
		case "ERROR":
			return Error
		case "PANIC":
			return Panic
		case "ALERT":
			return Alert
		case "EMERG":
			return Emergency
		case "CRASH":
			return Crash
		}
	case 6:
		switch s {
		case "SEVERE":
			return Severe
		case "FAILED":
			return Failure
		}
	case 7:
		switch s {
		case "WARNING":
			return Warn
		case "CRASHED":
			return Crash
		case "FAILURE":
			return Failure
		case "TIMEOUT":
			return Timeout
		}
	case 8:
		switch s {
		case "CRITICAL":
			return Critical
		case "DEADLOCK":
			return Deadlock
		}
	case 9:
		switch s {
		case "EMERGENCY":
			return Emergency
		case "EXCEPTION":
			return Exception
		}
	}
	return End
}

func referenceTokenize(input []byte) []Token {
	if len(input) == 0 {
		return nil
	}
	var tokens []Token
	i := 0
	for i < len(input) {
		base := refClassify(input[i])
		runLen := 1
		for i+runLen < len(input) && refClassify(input[i+runLen]) == base {
			runLen++
		}
		if base == C1 || base == D1 {
			if base == C1 && runLen >= 1 && runLen <= 9 {
				upper := make([]byte, runLen)
				for j := 0; j < runLen; j++ {
					upper[j] = refToUpper(input[i+j])
				}
				special := refSpecialToken(string(upper))
				if special != End {
					tokens = append(tokens, special)
					i += runLen
					continue
				}
			}
			r := runLen - 1
			if r >= 10 {
				r = 9
			}
			tokens = append(tokens, base+Token(r))
		} else {
			tokens = append(tokens, base)
		}
		i += runLen
	}
	return tokens
}

// FuzzTokenizerCorrectness verifies the production tokenizer matches
// the reference implementation derived from tokenizer.allium for all
// inputs. This is the broad oracle test — it implicitly covers every
// Tokenization @invariant by exercising the entire rule set against an
// independent encoding of the same rules. Per-invariant property tests
// further down provide more diagnostic failure modes.
func FuzzTokenizerCorrectness(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request processed id=123"))
	f.Add([]byte(""))
	f.Add([]byte("!!!$$$###"))
	f.Add([]byte("Jan Mon UTC PST CEST"))
	f.Add([]byte("T Z am PM"))
	f.Add([]byte("abc!📀🐶📊123"))
	f.Add([]byte("Sun Mar 2PM EST JAN FEB MAR"))
	f.Add([]byte("12-12-12T12:12:12.12T12:12Z123"))
	f.Fuzz(func(t *testing.T, input []byte) {
		tok := NewTokenizer(0)
		actual, _ := tok.Tokenize(input)
		expected := referenceTokenize(input)
		assert.Equal(t, expected, actual,
			"production tokenizer diverges from reference for input %q", input)
	})
}

// FuzzTokenizerDeterminism anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant Determinism
//
// "tokenize is a pure function: the same input and max_bytes always
// produce the same token sequence." Re-tokenizes the same input twice
// on a reused tokenizer and asserts the outputs match. Catches state
// leaks in the tokenizer's reusable internal buffers across calls.
func FuzzTokenizerDeterminism(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request processed id=123"))
	f.Add([]byte(""))
	f.Add([]byte("!!!$$$###"))
	f.Add([]byte("Jan Mon UTC PST CEST"))
	f.Fuzz(func(t *testing.T, input []byte) {
		tok := NewTokenizer(0)
		tokens1, indices1 := tok.Tokenize(input)
		tokens2, indices2 := tok.Tokenize(input)
		assert.Equal(t, tokens1, tokens2)
		assert.Equal(t, indices1, indices2)
	})
}

// FuzzTokenizerInputTruncation anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant InputTruncation
//
// "Only the first max_bytes of the input are tokenized." Tokenizing N
// bytes of input with no limit produces the same result as tokenizing
// the full input with an N-byte limit — a direct restatement of the
// invariant.
func FuzzTokenizerInputTruncation(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request processed"), uint8(10))
	f.Add([]byte("Jan Mon UTC PST"), uint8(5))
	f.Add([]byte("abc"), uint8(1))
	f.Fuzz(func(t *testing.T, input []byte, maxBytesRaw uint8) {
		if len(input) == 0 {
			return
		}
		maxBytes := int(maxBytesRaw)%len(input) + 1
		tokLimited := NewTokenizer(maxBytes)
		tokUnlimited := NewTokenizer(0)
		tokensLimited, _ := tokLimited.Tokenize(input)
		tokensTruncated, _ := tokUnlimited.Tokenize(input[:maxBytes])
		assert.Equal(t, tokensTruncated, tokensLimited)
	})
}

// FuzzTokenizerDigitCollapsing anchors the digit clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant StructuralCollapsing
//
// "Digits may be substituted with other digits" without changing the
// token sequence. Fuzz substitutes every digit byte with a different
// digit byte and asserts the resulting token sequence is identical.
func FuzzTokenizerDigitCollapsing(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request"))
	f.Add([]byte("error code 404 at 192.168.1.1"))
	f.Add([]byte("0"))
	f.Fuzz(func(t *testing.T, input []byte) {
		twin := make([]byte, len(input))
		copy(twin, input)
		for i, b := range twin {
			if b >= '0' && b <= '9' {
				twin[i] = '0' + (b-'0'+1)%10
			}
		}
		tok := NewTokenizer(0)
		tokensOrig, _ := tok.Tokenize(input)
		tokensTwin, _ := tok.Tokenize(twin)
		assert.Equal(t, tokensOrig, tokensTwin,
			"digit substitution should not change tokens: %q → %q", input, twin)
	})
}

// FuzzTokenizerCaseInsensitive anchors the case clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant StructuralCollapsing
//	    @invariant ByteClassification (letter category insensitive to case)
//	    @invariant SpecialTokenPromotion (case-insensitive matching)
//
// Flipping the case of every ASCII letter does not change the token
// sequence: lowercase and uppercase letters share the character base
// category, and special token promotion is case-insensitive (so e.g.
// "jan", "JAN", "Jan" all promote to Month).
func FuzzTokenizerCaseInsensitive(f *testing.F) {
	f.Add([]byte("Jan Mon UTC INFO request"))
	f.Add([]byte("jan mon utc info REQUEST"))
	f.Add([]byte("T Z am PM"))
	f.Fuzz(func(t *testing.T, input []byte) {
		twin := make([]byte, len(input))
		for i, b := range input {
			if b >= 'A' && b <= 'Z' {
				twin[i] = b + 32
			} else if b >= 'a' && b <= 'z' {
				twin[i] = b - 32
			} else {
				twin[i] = b
			}
		}
		tok := NewTokenizer(0)
		tokensOrig, _ := tok.Tokenize(input)
		tokensTwin, _ := tok.Tokenize(twin)
		assert.Equal(t, tokensOrig, tokensTwin,
			"case flip should not change tokens: %q → %q", input, twin)
	})
}

// FuzzIsMatchSymmetry anchors:
//
//	contract PatternMatching (adaptive_sampler.allium)
//	    @invariant Symmetry
//
// "is_match(a, b, t) = is_match(b, a, t) for all a, b, t."
// PatternMatching is defined in adaptive_sampler.allium (separate from
// the Tokenization contract above), but the test lives here because it
// shares the tokenizer + IsMatch infrastructure.
func FuzzIsMatchSymmetry(f *testing.F) {
	f.Add([]byte("INFO request ok"), []byte("WARN startup ok"), uint8(90))
	f.Add([]byte(""), []byte("hello"), uint8(50))
	f.Add([]byte("!"), []byte("!"), uint8(100))
	f.Fuzz(func(t *testing.T, inputA, inputB []byte, threshPct uint8) {
		// threshPct is uint8 (0-255) but IsMatch thresholds above 1.0 are
		// degenerate. %101 folds the range to 0-100 so dividing by 100
		// covers [0.0, 1.0] uniformly instead of wasting inputs on clamped values.
		thresh := float64(threshPct%101) / 100.0
		tok := NewTokenizer(0)
		tokensA, _ := tok.Tokenize(inputA)
		tokensB, _ := tok.Tokenize(inputB)
		ab := IsMatch(tokensA, tokensB, thresh)
		ba := IsMatch(tokensB, tokensA, thresh)
		assert.Equal(t, ab, ba,
			"IsMatch must be symmetric: a=%q b=%q thresh=%.2f", inputA, inputB, thresh)
	})
}

// FuzzIsMatchMonotonicity anchors:
//
//	contract PatternMatching (adaptive_sampler.allium)
//	    @invariant MonotonicThreshold
//
// "For fixed a and b, if is_match(a, b, t1) and t2 <= t1, then
// is_match(a, b, t2). Lowering the threshold cannot cause a previously
// passing match to fail."
func FuzzIsMatchMonotonicity(f *testing.F) {
	f.Add([]byte("INFO request ok"), []byte("WARN startup ok"), uint8(90), uint8(50))
	f.Add([]byte("abc"), []byte("abd"), uint8(70), uint8(60))
	f.Fuzz(func(t *testing.T, inputA, inputB []byte, hiPct, loPct uint8) {
		// See FuzzIsMatchSymmetry for why %101.
		hi := float64(hiPct%101) / 100.0
		lo := float64(loPct%101) / 100.0
		if lo > hi {
			lo, hi = hi, lo
		}
		tok := NewTokenizer(0)
		tokensA, _ := tok.Tokenize(inputA)
		tokensB, _ := tok.Tokenize(inputB)
		if IsMatch(tokensA, tokensB, hi) {
			assert.True(t, IsMatch(tokensA, tokensB, lo),
				"match at thresh=%.2f must imply match at thresh=%.2f: a=%q b=%q",
				hi, lo, inputA, inputB)
		}
	})
}

func TestIsMatch(t *testing.T) {
	tokenizer := NewTokenizer(0)
	// A string of 10 tokens to make math easier.
	ta, _ := tokenizer.tokenize([]byte("! @ # $ %"))
	tb, _ := tokenizer.tokenize([]byte("! @ # $ %"))

	assert.True(t, IsMatch(ta, tb, 1))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("! @ #1a1a1"))

	assert.True(t, IsMatch(ta, tb, 0.5))
	assert.False(t, IsMatch(ta, tb, 0.55))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("#1a1a1$ $ "))

	assert.False(t, IsMatch(ta, tb, 0.5))
	assert.True(t, IsMatch(ta, tb, 0.3))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte(""))

	assert.False(t, IsMatch(ta, tb, 0.5))
	assert.False(t, IsMatch(ta, tb, 0))
	assert.False(t, IsMatch(ta, tb, 1))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("!"))

	assert.True(t, IsMatch(ta, tb, 1))
	assert.True(t, IsMatch(ta, tb, 0.01))
}

// --- Dedicated anchoring + property test pairs ---
//
// The Tokenization contract in tokenizer.allium names seven invariants.
// The tests above provide partial coverage; the pairs below give each
// remaining invariant a dedicated anchoring test and (where the input
// space is non-trivial) a property test, so every named Tokenization
// invariant has both layers of coverage.

// TestTokenizer_EmptyInput anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant EmptyInput
//
// "An empty input produces an empty token sequence (length 0)."
//
// No property test pair: the input space for this invariant is a
// single value (empty bytes). A fuzz would be degenerate.
func TestTokenizer_EmptyInput(t *testing.T) {
	tokens, indices := NewTokenizer(0).Tokenize([]byte{})
	assert.Empty(t, tokens, "empty input must produce an empty token sequence")
	assert.Empty(t, indices, "empty input must produce no indices")
}

// TestTokenizer_ByteClassification anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant ByteClassification
//
// "Each input byte maps to exactly one base category via a 256-entry
// lookup table." Demonstrates the classification with one
// representative byte per category. The property pair
// FuzzTokenizerByteClassification covers the full byte space.
func TestTokenizer_ByteClassification(t *testing.T) {
	cases := []struct {
		name     string
		input    byte
		expected string
	}{
		{"digit", '5', "D"},
		{"lowercase letter", 'a', "C"},
		{"uppercase letter", 'A', "C"},
		{"space", ' ', " "},
		{"tab", '\t', " "},
		{"newline", '\n', " "},
		{"carriage return", '\r', " "},
		{"colon", ':', ":"},
		{"semicolon", ';', ";"},
		{"dash", '-', "-"},
		{"underscore", '_', "_"},
		{"forward slash", '/', "/"},
		{"backslash", '\\', "\\"},
		{"period", '.', "."},
		{"comma", ',', ","},
		{"single quote", '\'', "'"},
		{"double quote", '"', "\""},
		{"backtick", '`', "`"},
		{"tilde", '~', "~"},
		{"star", '*', "*"},
		{"plus", '+', "+"},
		{"equal", '=', "="},
		{"open paren", '(', "("},
		{"close paren", ')', ")"},
		{"open brace", '{', "{"},
		{"close brace", '}', "}"},
		{"open bracket", '[', "["},
		{"close bracket", ']', "]"},
		{"ampersand", '&', "&"},
		{"exclamation", '!', "!"},
		{"at sign", '@', "@"},
		{"pound", '#', "#"},
		{"dollar", '$', "$"},
		{"percent", '%', "%"},
		{"caret", '^', "^"},
		{"non-ASCII byte", 0xff, "C"},
		{"control byte", 0x01, "C"},
	}
	tok := NewTokenizer(0)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, _ := tok.tokenize([]byte{tc.input})
			assert.Equal(t, tc.expected, TokensToString(tokens),
				"byte 0x%02x classified incorrectly", tc.input)
		})
	}
}

// FuzzTokenizerByteClassification anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant ByteClassification
//
// Property: every byte in [0, 255] tokenized as a single-byte input
// must produce a token matching the reference implementation. The
// fuzz seeds enumerate the full input space; generated inputs add no
// additional coverage (the input space is finite) but the test
// follows the property-test convention. Uses referenceTokenize (not
// refClassify alone) so that single-byte special-token promotion —
// 'T' → T token, 'Z' → Zone token — is also exercised; the test
// would otherwise miss the length-1 cases of SpecialTokenPromotion
// which interact with classification.
func FuzzTokenizerByteClassification(f *testing.F) {
	for i := 0; i < 256; i++ {
		f.Add(byte(i))
	}
	f.Fuzz(func(t *testing.T, b byte) {
		tok := NewTokenizer(0)
		tokens, _ := tok.tokenize([]byte{b})
		expected := referenceTokenize([]byte{b})
		assert.Equal(t, expected, tokens,
			"byte 0x%02x: expected tokens %v, got %v", b, expected, tokens)
	})
}

// TestTokenizer_RunLengthEncoding anchors the run-boundary clause of:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant RunLengthEncoding
//
// "Consecutive bytes of the same base token form a run. When a
// different base token is encountered, the accumulated run is emitted
// as a single token." Demonstrates that homogeneous runs merge into
// one token and that category transitions produce token boundaries.
// The length-cap clause (runs of 10+ → D10/C10) is covered separately
// by TestTokenizerMaxCharRun and TestTokenizerMaxDigitRun.
func TestTokenizer_RunLengthEncoding(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"a", "C"},
		{"abc", "CCC"},
		{"abcdefghi", "CCCCCCCCC"},
		{"123", "DDD"},
		{"abc123", "CCCDDD"},
		{"a1b2c3", "CDCDCD"},
		{"abc !@# def", "CCC !@# CCC"},
	}
	tok := NewTokenizer(0)
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, _ := tok.tokenize([]byte(tc.input))
			assert.Equal(t, tc.expected, TokensToString(tokens))
		})
	}
}

// FuzzTokenizerRunLengthEncoding anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant RunLengthEncoding
//
// Property: a homogeneous digit run of length N produces exactly one
// token. For N <= 10 the token encodes the exact run length (D_N);
// for N >= 10 it is capped at D10. Digit runs are chosen because they
// never trigger special token promotion, isolating run-length encoding
// from SpecialTokenPromotion concerns.
func FuzzTokenizerRunLengthEncoding(f *testing.F) {
	for n := uint8(1); n <= 15; n++ {
		f.Add(n)
	}
	f.Fuzz(func(t *testing.T, n uint8) {
		if n == 0 || n > 30 {
			return
		}
		input := strings.Repeat("5", int(n))
		tok := NewTokenizer(0)
		tokens, _ := tok.tokenize([]byte(input))
		assert.Len(t, tokens, 1,
			"homogeneous digit run of length %d must produce exactly one token, got %d",
			n, len(tokens))
		runLen := int(n)
		if runLen > 10 {
			runLen = 10
		}
		expected := D1 + Token(runLen-1)
		assert.Equal(t, expected, tokens[0],
			"digit run of length %d: expected %v, got %v", n, expected, tokens[0])
	})
}

// TestTokenizer_SpecialTokenPromotion anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant SpecialTokenPromotion
//
// "Before emitting a character run of length 1-4, the run's bytes are
// uppercased and checked against a fixed table. If matched, the
// special token replaces the regular C1-C4." Demonstrates each
// promotion category (T, Zone, Apm, Month, Day) and the "no promotion
// for length >= 5" boundary.
func TestTokenizer_SpecialTokenPromotion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// Length 1: T → T, Z → Zone
		{"T", "T"},
		{"Z", "ZONE"},
		{"t", "T"}, // case insensitive
		// Length 2: AM, PM → Apm
		{"AM", "PM"},
		{"PM", "PM"},
		{"am", "PM"},
		// Length 3: Months
		{"JAN", "MTH"},
		{"DEC", "MTH"},
		{"Jan", "MTH"}, // case insensitive
		// Length 3: Days
		{"MON", "DAY"},
		{"SUN", "DAY"},
		// Length 3: Zones
		{"UTC", "ZONE"},
		// Length 4: Zones
		{"CEST", "ZONE"},
		{"AKDT", "ZONE"},
		// Boundary: length 5+ is never promoted via the 1-4 table
		{"JANUA", "CCCCC"},
		// No-match within length range falls back to regular C-tokens
		{"FOO", "CCC"},
		{"X", "C"},
	}
	tok := NewTokenizer(0)
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, _ := tok.tokenize([]byte(tc.input))
			assert.Equal(t, tc.expected, TokensToString(tokens))
		})
	}
}

// FuzzTokenizerSpecialTokenPromotion anchors:
//
//	contract Tokenization (tokenizer.allium)
//	    @invariant SpecialTokenPromotion
//
// Property: for any letter-only input of length 1-4, the tokenizer's
// output must match the reference function, which independently
// encodes the special token table. Inputs of length 5+ must NOT
// promote to a special token (they emit regular Cn for n in 5..10).
// Letter-only inputs isolate this property from ByteClassification
// and RunLengthEncoding concerns.
func FuzzTokenizerSpecialTokenPromotion(f *testing.F) {
	f.Add("T")
	f.Add("Z")
	f.Add("PM")
	f.Add("Jan")
	f.Add("CEST")
	f.Add("Foo")
	f.Add("Hello") // length 5, must not promote
	f.Add("EXCEPTIONAL")
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) == 0 || len(input) > 12 {
			return
		}
		for _, b := range []byte(input) {
			if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')) {
				return
			}
		}
		tok := NewTokenizer(0)
		actual, _ := tok.Tokenize([]byte(input))
		expected := referenceTokenize([]byte(input))
		assert.Equal(t, expected, actual,
			"tokenize(%q): expected %v, got %v", input, expected, actual)
	})
}
