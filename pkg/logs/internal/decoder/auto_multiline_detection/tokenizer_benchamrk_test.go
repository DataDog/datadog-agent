// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"
)

func BenchmarkTokenizerLong(b *testing.B) {
	tokenizer := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	}
}

func BenchmarkTokenizerShort(b *testing.B) {
	tokenizer := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.tokenize([]byte("abc123"))
	}
}

func BenchmarkTokenizerIsMatchNoMatchStart(b *testing.B) {
	tokenizer := NewTokenizer(0)
	ta, _ := tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.tokenize([]byte("$ abc foo bar thie beginning is different !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		isMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchNoMatchEnd(b *testing.B) {
	tokenizer := NewTokenizer(0)
	ta, _ := tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST But this one is different near the end of the sequence"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		isMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchFullMatch(b *testing.B) {
	tokenizer := NewTokenizer(0)
	ta, _ := tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		isMatch(ta, tb, 0.75)
	}
}
