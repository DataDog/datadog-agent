// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens provides tokenization functionality for log messages.
package preprocessor

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input         string
	expectedToken string
}

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

func TestTokenizerMaxCharRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("ABCDEFGHIJKLMNOP"))
	assert.Equal(t, "CCCCCCCCCC", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestTokenizerMaxDigitRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("0123456789012345"))
	assert.Equal(t, "DDDDDDDDDD", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestAllSymbolsAreHandled(t *testing.T) {
	for i := Space; i < D1; i++ {
		str := tokenToString(i)
		assert.NotEmpty(t, str, "Token %d is not converted to a debug string", i)
		assert.NotEqual(t, tokenLookup[str[0]], C1, "Token %v is not tokenizable", str)
	}
}

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
// Each fuzz test below maps to an @invariant in adaptive_sampler.allium.

// Reference tokenizer: an independent implementation of the tokenization
// rules from adaptive_sampler.allium. Used as an oracle to verify the
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

func refSpecialToken(s string) Token {
	if len(s) == 1 {
		switch s[0] {
		case 'T':
			return T
		case 'Z':
			return Zone
		}
		return End
	}
	switch s {
	case "AM", "PM":
		return Apm
	case "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
		"JUL", "AUG", "SEP", "OCT", "NOV", "DEC":
		return Month
	case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
		return Day
	case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
		"MST", "MDT", "PST", "PDT", "JST", "KST",
		"IST", "MSK", "CET", "BST", "HST", "HDT",
		"NST", "NDT",
		"CEST", "NZST", "NZDT", "ACST", "ACDT",
		"AEST", "AEDT", "AWST", "AWDT", "AKST",
		"AKDT", "CHST", "CHDT":
		return Zone
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
			if base == C1 && runLen >= 1 && runLen <= 4 {
				upper := make([]byte, runLen)
				for j := 0; j < runLen; j++ {
					upper[j] = refToUpper(input[i+j])
				}
				if special := refSpecialToken(string(upper)); special != End {
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

// Verify the production tokenizer matches the reference implementation
// derived from adaptive_sampler.allium for all inputs.
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

// Tokenization.Determinism: same input always produces the same tokens.
// Catches state leaks in the Tokenizer's reusable buffers.
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

// Tokenization.InputTruncation: tokenizing N bytes of input with no limit
// produces the same result as tokenizing the full input with an N-byte limit.
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

// Tokenization.StructuralCollapsing (digits): substituting any digit with
// a different digit does not change the token sequence.
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

// Tokenization.StructuralCollapsing + ByteClassification: flipping the case
// of every ASCII letter does not change the token sequence. Lowercase and
// uppercase letters are the same base category (character), and special
// token promotion is case-insensitive.
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

// PatternMatching.Symmetry: is_match(a, b, t) = is_match(b, a, t).
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

// PatternMatching.MonotonicThreshold: if two sequences match at threshold t1,
// they must also match at any t2 <= t1.
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

// PatternMatching.Isolation: two sequences differing at more than
// tolerance = len - round(threshold * len) positions are guaranteed
// not to match. Contrapositive of ThresholdComparison.
func FuzzIsMatchIsolation(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request"), uint8(90), uint8(0))
	f.Add([]byte("error at line 42 in module"), uint8(75), uint8(3))
	f.Add([]byte("GET /api/v2/users 200 42ms"), uint8(50), uint8(1))
	f.Fuzz(func(t *testing.T, input []byte, threshPct, startPos uint8) {
		thresh := float64(threshPct%101) / 100.0
		tok := NewTokenizer(0)
		tokens, _ := tok.Tokenize(input)
		n := len(tokens)
		if n < 2 {
			return
		}

		required := int(math.Round(thresh * float64(n)))
		tolerance := n - required

		// Construct a mutated sequence differing at tolerance+1 positions.
		// This should guarantee no match.
		diffs := tolerance + 1
		if diffs > n {
			return // threshold so low that everything matches
		}

		mutated := make([]Token, n)
		copy(mutated, tokens)
		start := int(startPos) % n
		for i := range diffs {
			pos := (start + i) % n
			mutated[pos] = (tokens[pos] + 1) % End
		}

		assert.False(t, IsMatch(tokens, mutated, thresh),
			"sequences differing at %d positions (tolerance=%d) must not match: thresh=%.2f len=%d",
			diffs, tolerance, thresh, n)
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
