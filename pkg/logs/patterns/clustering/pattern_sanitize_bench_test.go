// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// sanitizeForTemplateIntoOptimized is an ASCII-fast-path variant for benchmarking.
// For bytes < 0x80, skip utf8.DecodeRuneInString and use direct byte comparison.
// Only decode runes when non-ASCII bytes are encountered.
func sanitizeForTemplateIntoOptimized(builder *strings.Builder, s string) int {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < utf8.RuneSelf {
			// ASCII fast path: keep if space (0x20) through 0x7E, exclude DEL (0x7F)
			if b >= ' ' && b != 0x7F {
				continue
			}
			// Control char or DEL: need to filter
			if builder != nil {
				builder.WriteString(s[:i])
			}
			written := i
			i++
			for i < len(s) {
				b = s[i]
				if b < utf8.RuneSelf {
					if b >= ' ' && b != 0x7F {
						if builder != nil {
							builder.WriteByte(b)
						}
						written++
					}
					i++
					continue
				}
				r, size := utf8.DecodeRuneInString(s[i:])
				keep := r >= ' ' && r != 0x7F && r != utf8.RuneError && r < 0xFFFD
				if keep {
					if builder != nil {
						builder.WriteString(s[i : i+size])
					}
					written += size
				}
				i += size
			}
			return written
		}
		// Non-ASCII: decode rune and check
		r, size := utf8.DecodeRuneInString(s[i:])
		keep := r >= ' ' && r != 0x7F && r != utf8.RuneError && r < 0xFFFD
		if !keep {
			if builder != nil {
				builder.WriteString(s[:i])
			}
			written := i
			i += size
			for i < len(s) {
				r, size = utf8.DecodeRuneInString(s[i:])
				keep = r >= ' ' && r != 0x7F && r != utf8.RuneError && r < 0xFFFD
				if keep {
					if builder != nil {
						builder.WriteString(s[i : i+size])
					}
					written += size
				}
				i += size
			}
			return written
		}
		i += size - 1 // loop will i++, so net advance is size
	}
	if builder != nil {
		builder.WriteString(s)
	}
	return len(s)
}

// --- Benchmark input fixtures ---

// Pure ASCII, typical log formats
var (
	// Short (~50B): JSON-style
	benchASCIIShort = `{"level":"info","msg":"User login","ts":1234567890}`

	// Medium (~200B): syslog-style
	benchASCIIMedium = `2024-01-15T10:30:45.123Z INFO  [http-server] Request completed method=GET path=/api/users status=200 duration_ms=42 client_ip=192.168.1.1`

	// Long (~1KB): structured key=value
	benchASCIILong = strings.Repeat(`service=api env=prod region=us-east-1 instance=i-abc123 `+
		`level=info msg="request processed" method=GET path=/health status=200 `+
		`duration_ns=1234 user_id=usr_xyz tags="a,b,c" `, 5) // ~1KB
)

// Mixed ASCII + UTF-8
var (
	benchMixedShort  = `{"msg":"User 世界 logged in","level":"info"}`
	benchMixedMedium = `2024-01-15 INFO [svc] ユーザー 世界 user=123 action=login 日本語`
	benchMixedLong   = strings.Repeat(`level=info msg=処理中 世界 日本語 emoji=🎉 `, 18) // ~1KB
)

// With control characters that need filtering
var (
	benchControlShort  = "ERROR\x00\x07\x08: connection failed\x7F"
	benchControlMedium = "INFO\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f " +
		"request_id=abc123\x7F\x1b status=200"
	benchControlLong = strings.Repeat("normal_text\x00\x07\x08\x7F\x0a\x0d\x1b", 50) // ~1KB
)

// --- Benchmarks: sanitizeForTemplateInto with builder (typical GetPatternString path) ---

func BenchmarkSanitize_ASCII_Short(b *testing.B) {
	s := benchASCIIShort
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_ASCII_Medium(b *testing.B) {
	s := benchASCIIMedium
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_ASCII_Long(b *testing.B) {
	s := benchASCIILong
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Mixed_Short(b *testing.B) {
	s := benchMixedShort
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Mixed_Medium(b *testing.B) {
	s := benchMixedMedium
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Mixed_Long(b *testing.B) {
	s := benchMixedLong
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Control_Short(b *testing.B) {
	s := benchControlShort
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Control_Medium(b *testing.B) {
	s := benchControlMedium
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

func BenchmarkSanitize_Control_Long(b *testing.B) {
	s := benchControlLong
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateInto(&builder, s)
	}
}

// --- Benchmarks: sanitizeForTemplateLen (builder=nil, GetWildcardCharPositions path) ---

func BenchmarkSanitizeLen_ASCII_Short(b *testing.B) {
	s := benchASCIIShort
	for i := 0; i < b.N; i++ {
		sanitizeForTemplateLen(s)
	}
}

func BenchmarkSanitizeLen_ASCII_Medium(b *testing.B) {
	s := benchASCIIMedium
	for i := 0; i < b.N; i++ {
		sanitizeForTemplateLen(s)
	}
}

func BenchmarkSanitizeLen_ASCII_Long(b *testing.B) {
	s := benchASCIILong
	for i := 0; i < b.N; i++ {
		sanitizeForTemplateLen(s)
	}
}

func BenchmarkSanitizeLen_Mixed_Short(b *testing.B) {
	s := benchMixedShort
	for i := 0; i < b.N; i++ {
		sanitizeForTemplateLen(s)
	}
}

func BenchmarkSanitizeLen_Control_Short(b *testing.B) {
	s := benchControlShort
	for i := 0; i < b.N; i++ {
		sanitizeForTemplateLen(s)
	}
}

// --- Correctness test: optimized matches current implementation ---

func TestSanitizeOptimized_MatchesCurrent(t *testing.T) {
	inputs := []string{
		"",
		"Hello World 123",
		"Hello\x00\x07\x08World",
		"Hello\x7FWorld",
		"Service: Error! @user #tag",
		"Hello 世界 🌍",
		benchASCIIShort,
		benchASCIIMedium,
		benchMixedShort,
		benchControlShort,
		benchControlMedium,
		"mixed\x00ascii\x7Fand 世界 unicode",
	}
	for _, s := range inputs {
		var b1, b2 strings.Builder
		b1.Grow(len(s))
		b2.Grow(len(s))
		got1 := sanitizeForTemplateInto(&b1, s)
		got2 := sanitizeForTemplateIntoOptimized(&b2, s)
		out1 := b1.String()
		out2 := b2.String()
		if got1 != got2 || out1 != out2 {
			t.Errorf("mismatch for %q:\n  current: len=%d out=%q\n  optimized: len=%d out=%q",
				s, got1, out1, got2, out2)
		}
	}
}

// --- Benchmarks: optimized vs current (ASCII hot path) ---

func BenchmarkSanitize_ASCII_Short_Optimized(b *testing.B) {
	s := benchASCIIShort
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateIntoOptimized(&builder, s)
	}
}

func BenchmarkSanitize_ASCII_Medium_Optimized(b *testing.B) {
	s := benchASCIIMedium
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateIntoOptimized(&builder, s)
	}
}

func BenchmarkSanitize_ASCII_Long_Optimized(b *testing.B) {
	s := benchASCIILong
	var builder strings.Builder
	builder.Grow(len(s))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		sanitizeForTemplateIntoOptimized(&builder, s)
	}
}

func BenchmarkSanitizeLen_ASCII_Short_Optimized(b *testing.B) {
	s := benchASCIIShort
	var sink int
	for i := 0; i < b.N; i++ {
		sink = sanitizeForTemplateIntoOptimized(nil, s)
	}
	_ = sink
}
