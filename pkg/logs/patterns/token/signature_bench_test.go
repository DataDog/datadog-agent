// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

import (
	"strings"
	"testing"
)

// benchTokenPattern is a realistic log line token sequence covering all token types.
// Pattern: timestamp severity word word uri kvseq word ipv4 word numeric word httpmethod httpstatus word path word email
var benchTokenPattern = []struct {
	typ   TokenType
	value string
}{
	{TokenDate, "2024-01-15"},
	{TokenWhitespace, " "},
	{TokenLocalTime, "12:34:56"},
	{TokenSpecialChar, "."},
	{TokenNumeric, "789"},
	{TokenWhitespace, " "},
	{TokenOffsetDateTime, "2024-01-15T12:34:56+00:00"},
	{TokenWhitespace, " "},
	{TokenSeverityLevel, "ERROR"},
	{TokenWhitespace, " "},
	{TokenWord, "connection"},
	{TokenWhitespace, " "},
	{TokenWord, "failed"},
	{TokenWhitespace, " "},
	{TokenHTTPMethod, "POST"},
	{TokenWhitespace, " "},
	{TokenURI, "https://api.example.com/v2/users?id=42"},
	{TokenWhitespace, " "},
	{TokenHTTPStatus, "503"},
	{TokenWhitespace, " "},
	{TokenIPv4, "192.168.1.100"},
	{TokenSpecialChar, ":"},
	{TokenNumeric, "8080"},
	{TokenWhitespace, " "},
	{TokenIPv6, "2001:0db8:85a3::8a2e:0370:7334"},
	{TokenWhitespace, " "},
	{TokenKeyValueSequence, "env=prod region=us-east-1"},
	{TokenWhitespace, " "},
	{TokenEmail, "alert@example.com"},
	{TokenWhitespace, " "},
	{TokenAbsolutePath, "/var/log/app/server.log"},
	{TokenWhitespace, " "},
	{TokenAuthority, "db-primary.internal:5432"},
	{TokenWhitespace, " "},
	{TokenPathWithQueryAndFragment, "/api/v2/search?q=test#results"},
	{TokenWhitespace, " "},
	{TokenRegularName, "app-server-01"},
	{TokenWhitespace, " "},
	{TokenLocalDate, "2024-01-15"},
	{TokenWhitespace, " "},
	{TokenLocalDateTime, "2024-01-15T12:34:56"},
	{TokenWhitespace, " "},
	{TokenCollapsedToken, "*** 3 similar ***"},
	{TokenWhitespace, " "},
	{TokenWord, "done"},
}

// makeBenchTokenList creates a TokenList with n tokens cycling through all token types.
func makeBenchTokenList(n int) *TokenList {
	tl := NewTokenList()
	for i := 0; i < n; i++ {
		entry := benchTokenPattern[i%len(benchTokenPattern)]
		tl.Add(NewToken(entry.typ, entry.value, PotentialWildcard))
	}
	return tl
}

// BenchmarkComputeHash_Short benchmarks computeHash with a short signature (~20 chars)
func BenchmarkComputeHash_Short(b *testing.B) {
	input := "Word|Whitespace|Word"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeHash(input)
	}
}

// BenchmarkComputeHash_Medium benchmarks computeHash with a medium signature (~80 chars)
func BenchmarkComputeHash_Medium(b *testing.B) {
	input := "Word|Whitespace|Word|Numeric|Whitespace|Word|SpecialChar|Date|Whitespace|LocalTime"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeHash(input)
	}
}

// BenchmarkComputeHash_Long benchmarks computeHash with a long signature (~200 chars)
func BenchmarkComputeHash_Long(b *testing.B) {
	parts := make([]string, 25)
	for i := range parts {
		parts[i] = benchTokenPattern[i%len(benchTokenPattern)].typ.String()
	}
	input := strings.Join(parts, "|")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeHash(input)
	}
}

// BenchmarkPositionSignature_5 benchmarks positionSignature with 5 tokens
func BenchmarkPositionSignature_5(b *testing.B) {
	tl := makeBenchTokenList(5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = positionSignature(tl)
	}
}

// BenchmarkPositionSignature_10 benchmarks positionSignature with 10 tokens
func BenchmarkPositionSignature_10(b *testing.B) {
	tl := makeBenchTokenList(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = positionSignature(tl)
	}
}

// BenchmarkPositionSignature_20 benchmarks positionSignature with 20 tokens
func BenchmarkPositionSignature_20(b *testing.B) {
	tl := makeBenchTokenList(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = positionSignature(tl)
	}
}

// BenchmarkPositionSignature_50 benchmarks positionSignature with 50 tokens
func BenchmarkPositionSignature_50(b *testing.B) {
	tl := makeBenchTokenList(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = positionSignature(tl)
	}
}

// BenchmarkNewSignature_5 benchmarks NewSignature with 5 tokens
func BenchmarkNewSignature_5(b *testing.B) {
	tl := makeBenchTokenList(5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewSignature(tl)
	}
}

// BenchmarkNewSignature_10 benchmarks NewSignature with 10 tokens
func BenchmarkNewSignature_10(b *testing.B) {
	tl := makeBenchTokenList(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewSignature(tl)
	}
}

// BenchmarkNewSignature_20 benchmarks NewSignature with 20 tokens
func BenchmarkNewSignature_20(b *testing.B) {
	tl := makeBenchTokenList(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewSignature(tl)
	}
}

// BenchmarkNewSignature_50 benchmarks NewSignature with 50 tokens
func BenchmarkNewSignature_50(b *testing.B) {
	tl := makeBenchTokenList(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewSignature(tl)
	}
}
