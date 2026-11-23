// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLTokenizerPosition(t *testing.T) {
	assert := assert.New(t)
	query := "SELECT username AS         person FROM users WHERE id=4"
	tok := NewSQLTokenizer(query, false, nil)
	tokenCount := 0
	for ; ; tokenCount++ {
		startPos := tok.Position()
		kind, buff := tok.Scan()
		if kind == EndChar {
			break
		}
		if kind == LexError {
			assert.Fail("experienced an unexpected lexer error")
		}
		assert.Equal(string(buff), query[startPos:tok.Position()])
		tok.SkipBlank()
	}
	assert.Equal(10, tokenCount)
}

func TestTokenizeDecimalWithoutIntegerPart(t *testing.T) {
	inputs := []string{
		".001",
		".21341",
		"-.1234",
		"-.0003",
	}

	for _, input := range inputs {
		testTokenizeNumber(t, input)
	}
}

func FuzzTokenizeIntegerStrings(f *testing.F) {
	f.Add(int64(123456789))
	f.Add(int64(0))
	f.Add(int64(-1))
	f.Add(int64(-2018))
	f.Add(int64(math.MinInt64))
	f.Add(int64(math.MaxInt64))
	f.Add(int64(39))
	f.Add(int64(7))
	f.Add(int64(-83))
	f.Add(int64(-9223372036854775807))
	f.Add(int64(9))
	f.Add(int64(-108))
	f.Add(int64(-71))
	f.Add(int64(-9223372036854775675))

	f.Fuzz(func(t *testing.T, i int64) {
		for _, base := range []int{8, 10} {
			value := strconv.FormatInt(i, base)
			testTokenizeNumber(t, value)
		}
	})
}

func FuzzTokenizeFloatStrings(f *testing.F) {
	f.Add(float64(0))
	f.Add(float64(0.123456789))
	f.Add(float64(-0.123456789))
	f.Add(float64(-.0123456789))
	f.Add(float64(12.3456789))
	f.Add(float64(-12.3456789))

	f.Fuzz(func(t *testing.T, f float64) {
		// NaN and Inf are not valid SQL numeric literals; strconv.FormatFloat
		// renders them as "NaN", "+Inf", or "-Inf", which the tokenizer correctly
		// does not classify as Number tokens.  Skip them rather than asserting
		// a property they cannot satisfy.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return
		}
		for _, format := range []byte{'e', 'E', 'f'} {
			value := strconv.FormatFloat(f, format, -1, 64)
			testTokenizeNumber(t, value)
		}
	})
}

func testTokenizeNumber(t *testing.T, input string) {
	tok := NewSQLTokenizer(input, false, nil)
	kind, buf := tok.Scan()
	if kind != Number {
		t.Errorf("the value [%s] was not interpreted as a number", input)
	} else if input != string(buf) {
		t.Errorf("the value [%s] was incorrectly parsed to [%s]", input, string(buf))
	}
}

func TestTokenizePrefixedAndSeparatedNumbers(t *testing.T) {
	inputs := []string{
		// binary and prefixed octal literals (MySQL, PostgreSQL 16+)
		"0b1010",
		"0B1",
		"0o755",
		"0O7",
		"-0b1010",
		"-0o17",
		"-0x1F",
		// digit separators (PostgreSQL 16+)
		"1_500_000",
		"-1_000",
		"0xFF_FF",
		"0x_ff",
		"0b1010_1010",
		"0B_10",
		"0o7_55",
		"0O_7",
		"012_3",
		"1_000.50",
		"1.5_0",
		"1e1_0",
		"1_000.50e-1_2",
		".5_0",
	}
	for _, input := range inputs {
		testTokenizeNumber(t, input)
	}
}

func TestTokenizeLiteralBoundaries(t *testing.T) {
	type token struct {
		kind TokenKind
		buf  string
	}
	for _, tt := range []struct {
		in   string
		want []token
	}{
		// A prefix only counts when a digit of its base follows it.
		{"0b", []token{{Number, "0"}, {ID, "b"}}},
		{"0b2", []token{{Number, "0"}, {ID, "b2"}}},
		{"0b_", []token{{Number, "0"}, {ID, "b_"}}},
		{"0bad", []token{{Number, "0"}, {ID, "bad"}}},
		{"0o8", []token{{Number, "0"}, {ID, "o8"}}},
		{"0o", []token{{Number, "0"}, {ID, "o"}}},
		{"0b1abc", []token{{Number, "0b1"}, {ID, "abc"}}},
		// A separator counts only when a digit follows it.
		{"1_000_", []token{{Number, "1_000"}, {ID, "_"}}},
		{"1__000", []token{{Number, "1"}, {ID, "__000"}}},
		{"1_users", []token{{Number, "1"}, {ID, "_users"}}},
		{"0x_g", []token{{Number, "0x"}, {ID, "_g"}}},
		// identifiers containing the same characters are untouched
		{"ab0b1", []token{{ID, "ab0b1"}}},
		{"t1_2", []token{{ID, "t1_2"}}},
		// Oracle alternative quoting
		{"q'[it's]'", []token{{String, "q'[it's]'"}}},
		{"Q'{a 'b'}'", []token{{String, "Q'{a 'b'}'"}}},
		{"nq'(x)'", []token{{String, "nq'(x)'"}}},
		{"NQ'<x>'", []token{{String, "NQ'<x>'"}}},
		{"q'!it's!'", []token{{String, "q'!it's!'"}}},
		{"q'[]'", []token{{String, "q'[]'"}}},
		{"q'[a]b]'", []token{{String, "q'[a]b]'"}}},
		// only q and nq prefixes open an alternative quoted string
		{"faq'x'", []token{{ID, "faq"}, {String, "x"}}},
		{"N'x'", []token{{ID, "N"}, {String, "x"}}},
		{"q 'x'", []token{{ID, "q"}, {String, "x"}}},
		// whitespace is not a valid delimiter, so q' x' is an ordinary string
		{"q' x'", []token{{ID, "q"}, {String, " x"}}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			tok := NewSQLTokenizer(tt.in, false, nil)
			var got []token
			for {
				kind, buf := tok.Scan()
				if kind == EndChar || kind == LexError {
					break
				}
				got = append(got, token{kind, string(buf)})
			}
			assert.NoError(t, tok.Err())
			assert.Equal(t, tt.want, got)
		})
	}
}
