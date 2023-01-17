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
