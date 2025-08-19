// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"flag"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type compactSpacesTestCase struct {
	before string
	after  string
}

func TestMain(m *testing.M) {
	flag.Parse()

	// prepare JSON obfuscator tests
	suite, err := loadTests()
	if err != nil {
		log.Fatalf("Failed to load JSON obfuscator tests: %s", err.Error())
	}
	if len(suite) == 0 {
		log.Fatal("no tests in suite")
	}
	jsonSuite = suite

	os.Exit(m.Run())
}

func TestNewObfuscator(t *testing.T) {
	assert := assert.New(t)
	o := NewObfuscator(Config{})
	assert.Nil(o.es)
	assert.Nil(o.mongo)

	o = NewObfuscator(Config{})
	assert.Nil(o.es)
	assert.Nil(o.mongo)

	o = NewObfuscator(Config{
		ES:    JSONConfig{Enabled: true},
		Mongo: JSONConfig{Enabled: true},
	})
	defer o.Stop()
	assert.NotNil(o.es)
	assert.NotNil(o.mongo)
}

func TestCompactWhitespaces(t *testing.T) {
	assert := assert.New(t)

	resultsToExpect := []compactSpacesTestCase{
		{"aa",
			"aa"},

		{" aa bb",
			"aa bb"},

		{"aa    bb  cc  dd ",
			"aa bb cc dd"},

		{"    ",
			""},

		{"a b       cde     fg       hi                     j  jk   lk lkjfdsalfd     afsd sfdafsd f",
			"a b cde fg hi j jk lk lkjfdsalfd afsd sfdafsd f"},

		{"   ¡™£¢∞§¶    •ªº–≠œ∑´®†¥¨ˆøπ “‘«åß∂ƒ©˙∆˚¬…æΩ≈ç√ ∫˜µ≤≥÷    ",
			"¡™£¢∞§¶ •ªº–≠œ∑´®†¥¨ˆøπ “‘«åß∂ƒ©˙∆˚¬…æΩ≈ç√ ∫˜µ≤≥÷"},
	}

	for _, testCase := range resultsToExpect {
		assert.Equal(testCase.after, compactWhitespaces(testCase.before))
	}
}

func TestReplaceDigits(t *testing.T) {
	assert := assert.New(t)

	for _, tt := range []struct {
		in       []byte
		expected []byte
	}{
		{
			[]byte("table123"),
			[]byte("table?"),
		},
		{
			[]byte(""),
			[]byte(""),
		},
		{
			[]byte("2020-table"),
			[]byte("?-table"),
		},
		{
			[]byte("sales_2019_07_01"),
			[]byte("sales_?_?_?"),
		},
		{
			[]byte("45"),
			[]byte("?"),
		},
	} {
		assert.Equal(tt.expected, replaceDigits(tt.in))
	}
}

func TestLiteralEscapes(t *testing.T) {
	o := NewObfuscator(Config{})

	t.Run("default", func(t *testing.T) {
		assert.False(t, o.useSQLLiteralEscapes())
	})

	t.Run("true", func(t *testing.T) {
		o.setSQLLiteralEscapes(true)
		assert.True(t, o.useSQLLiteralEscapes())
	})

	t.Run("false", func(t *testing.T) {
		o.setSQLLiteralEscapes(false)
		assert.False(t, o.useSQLLiteralEscapes())
	})
}

func BenchmarkCompactWhitespaces(b *testing.B) {
	str := "a b       cde     fg       hi                     j  jk   lk lkjfdsalfd     afsd sfdafsd f"
	for i := 0; i < b.N; i++ {
		compactWhitespaces(str)
	}
}

func BenchmarkReplaceDigits(b *testing.B) {
	tbl := []byte("sales_2019_07_01_orders")
	for i := 0; i < b.N; i++ {
		replaceDigits(tbl)
	}
}
