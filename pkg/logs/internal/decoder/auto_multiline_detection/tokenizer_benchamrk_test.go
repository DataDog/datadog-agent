// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tokens"
	"testing"
)

// BenchmarkTokenizerShort tests tokenization of very short messages
func BenchmarkTokenizerShort(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("abc123"))
	}
}

// BenchmarkTokenizerMedium tests tokenization of medium-length log lines
func BenchmarkTokenizerMedium(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully"))
	}
}

// BenchmarkTokenizerLong tests tokenization with many different token types
func BenchmarkTokenizerLong(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	}
}

// BenchmarkTokenizerVeryLong tests tokenization with very long log messages
func BenchmarkTokenizerVeryLong(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	msg := []byte("2024-01-15T10:30:45.123456Z ERROR [microservice-backend] exception=NullPointerException user_id=12345678 session_id=abcdef1234567890 " +
		"request_id=req-2024-01-15-10-30-45-123456 path=/api/v1/users/profile method=GET status=500 duration_ms=1234 " +
		"{'error': 'Internal server error', 'stack_trace': 'at com.example.Service.method(Service.java:123)', " +
		"'details': {'query_params': {'filter': 'active', 'sort': 'created_at'}, 'headers': {'User-Agent': 'Mozilla/5.0'}}} " +
		"remote_addr=192.168.1.100 forwarded_for=10.0.0.1,172.16.0.1 correlation_id=corr-abc-123-xyz-789")
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize(msg)
	}
}

// BenchmarkTokenizerJSON tests JSON-heavy log messages
func BenchmarkTokenizerJSON(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte(`{"timestamp":"2024-01-15T10:30:45.123Z","level":"INFO","service":"api","message":"Request completed","duration_ms":125,"user_id":12345,"metadata":{"version":"1.2.3","env":"prod"}}`))
	}
}

// BenchmarkTokenizerTimestampHeavy tests messages with multiple timestamp formats
func BenchmarkTokenizerTimestampHeavy(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("Mon Jan 15 10:30:45 PST 2024 | 2024-01-15T10:30:45.123Z | 15/Jan/2024:10:30:45 -0800 | Jan 15, 2024 10:30 AM EDT"))
	}
}

// BenchmarkTokenizerNumberHeavy tests messages with many numbers
func BenchmarkTokenizerNumberHeavy(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("cpu=45.67 memory=2048 disk=512000 network_tx=1234567890 network_rx=9876543210 connections=100 threads=32 load_avg=1.23,2.34,3.45"))
	}
}

// BenchmarkTokenizerSpecialCharsHeavy tests messages with many special characters
func BenchmarkTokenizerSpecialCharsHeavy(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("!@#$%^&*()_+-=[]{}|\\:;\"'<>,.?/~`!@#$%^&*()_+-=[]{}|\\:;\"'<>,.?/~`"))
	}
}

// BenchmarkTokenizerStackTrace tests log messages with stack traces
func BenchmarkTokenizerStackTrace(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("at com.example.Service.handleRequest(Service.java:123) at com.example.Controller.process(Controller.java:456) at javax.servlet.http.HttpServlet.service(HttpServlet.java:789)"))
	}
}

// BenchmarkTokenizerMixedTimezones tests messages with various timezone abbreviations
func BenchmarkTokenizerMixedTimezones(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("UTC GMT EST EDT CST CDT MST MDT PST PDT JST KST IST MSK CEST CET BST NZST NZDT ACST ACDT AEST AEDT AWST AKST HST CHST"))
	}
}

// BenchmarkTokenizerAllMonthsDays tests messages with all month and day names
func BenchmarkTokenizerAllMonthsDays(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("JAN FEB MAR APR MAY JUN JUL AUG SEP OCT NOV DEC MON TUE WED THU FRI SAT SUN AM PM"))
	}
}

// BenchmarkTokenizerLongWords tests messages with very long character runs
func BenchmarkTokenizerLongWords(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("thisIsAVeryLongWordWithoutAnySpacesOrSpecialCharactersJustToTestHowTheTokenizerHandlesLongCharacterRunsInTheInput"))
	}
}

// BenchmarkTokenizerLongNumbers tests messages with very long number sequences
func BenchmarkTokenizerLongNumbers(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("12345678901234567890123456789012345678901234567890123456789012345678901234567890"))
	}
}

// BenchmarkTokenizerPathsAndURLs tests file paths and URLs
func BenchmarkTokenizerPathsAndURLs(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("/var/log/application/app.log /usr/local/bin/service C:\\Windows\\System32\\drivers\\etc\\hosts https://api.example.com:8443/v1/users?id=123&filter=active"))
	}
}

// BenchmarkTokenizerRealisticApacheLog tests realistic Apache access log format
func BenchmarkTokenizerRealisticApacheLog(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte(`192.168.1.100 - user123 [15/Jan/2024:10:30:45 -0800] "GET /api/v1/users/profile?id=12345 HTTP/1.1" 200 4567 "https://example.com" "Mozilla/5.0 (X11; Linux x86_64)"`))
	}
}

// BenchmarkTokenizerRealisticAppLog tests realistic application log format
func BenchmarkTokenizerRealisticAppLog(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tokenizer.Tokenize([]byte("2024-01-15 10:30:45.123 [pool-1-thread-15] INFO  c.e.s.UserService - User authentication successful for user_id=12345, session=abc123, ip=192.168.1.100"))
	}
}

func BenchmarkTokenizerIsMatchNoMatchStart(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	ta, _ := tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.Tokenize([]byte("$ abc foo bar thie beginning is different !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		tokens.IsMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchNoMatchEnd(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	ta, _ := tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST But this one is different near the end of the sequence"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		tokens.IsMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchFullMatch(b *testing.B) {
	tokenizer := tokens.NewTokenizer(0)
	ta, _ := tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tokenizer.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		tokens.IsMatch(ta, tb, 0.75)
	}
}
