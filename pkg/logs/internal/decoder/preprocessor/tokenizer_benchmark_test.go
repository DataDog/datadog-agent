// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import "testing"

// BenchmarkTokenizerOwnership compares the stable caller-owned API with the
// borrowed-buffer path used by the synchronous preprocessing pipeline.
func BenchmarkTokenizerOwnership(b *testing.B) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"Short", []byte("abc123")},
		{"Medium", []byte("2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully")},
		{"JSON", []byte(`{"timestamp":"2024-01-15T10:30:45.123Z","level":"INFO","service":"api","message":"Request completed","duration_ms":125,"user_id":12345}`)},
		{"StackTrace", []byte("at com.example.Service.handleRequest(Service.java:123) at com.example.Controller.process(Controller.java:456) at javax.servlet.http.HttpServlet.service(HttpServlet.java:789)")},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.input)))

			b.Run("Owned", func(b *testing.B) {
				tok := NewTokenizer(0)
				for n := 0; n < b.N; n++ {
					tok.Tokenize(tc.input)
				}
			})

			b.Run("Borrowed", func(b *testing.B) {
				tok := NewTokenizer(0)
				for n := 0; n < b.N; n++ {
					tok.tokenizeBorrowed(tc.input)
				}
			})
		})
	}
}

// BenchmarkTokenizerThroughput reports tokenizer throughput in MiB/s over
// representative, ~1 KB production log lines (structured JSON, a Java stack
// trace, and a key=value app log). It measures both the caller-owned public API
// and the borrowed-buffer path used by the synchronous preprocessing pipeline.
// The reported MiB/s uses binary mebibytes (1024*1024 bytes).
func BenchmarkTokenizerThroughput(b *testing.B) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"JSON", []byte(`{"timestamp":"2024-01-15T10:30:45.123456Z","level":"ERROR","logger":"com.example.service.PaymentProcessor","thread":"http-nio-8080-exec-42","service":"payment-api","env":"production","region":"us-east-1","host":"ip-10-0-12-34.ec2.internal","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","user_id":1234567,"account_id":"acct_9f8e7d6c5b4a3210","request_id":"req-2024-01-15-10-30-45-abcdef123456","method":"POST","path":"/api/v1/payments/charge","query":"retry=true&source=mobile","status":500,"duration_ms":1234,"bytes_out":4096,"client_ip":"203.0.113.42","user_agent":"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X)","message":"Failed to process payment: gateway timeout after 3 retries to upstream processor gw-03","error":{"type":"GatewayTimeoutException","code":"GTW_504","detail":"upstream did not respond within 3000ms","retryable":true},"metadata":{"amount":9999,"currency":"USD","merchant_id":"mrc_abc123def456","idempotency_key":"idem_xyz789uvw012","circuit_breaker":"half_open"}}`)},
		{"StackTrace", []byte(`javax.servlet.ServletException: Request processing failed; nested exception is java.lang.NullPointerException: Cannot invoke "com.example.model.User.getId()" because "user" is null at org.springframework.web.servlet.FrameworkServlet.processRequest(FrameworkServlet.java:1014) at org.springframework.web.servlet.FrameworkServlet.doPost(FrameworkServlet.java:914) at javax.servlet.http.HttpServlet.service(HttpServlet.java:681) at org.springframework.web.servlet.FrameworkServlet.service(FrameworkServlet.java:889) at javax.servlet.http.HttpServlet.service(HttpServlet.java:764) at org.apache.catalina.core.ApplicationFilterChain.internalDoFilter(ApplicationFilterChain.java:227) at org.apache.catalina.core.ApplicationFilterChain.doFilter(ApplicationFilterChain.java:162) at com.example.security.JwtAuthenticationFilter.doFilterInternal(JwtAuthenticationFilter.java:88) at org.springframework.web.filter.OncePerRequestFilter.doFilter(OncePerRequestFilter.java:117) at org.apache.catalina.core.StandardWrapperValve.invoke(StandardWrapperValve.java:197)`)},
		{"AppLog", []byte(`2024-01-15 10:30:45.123 [http-nio-8080-exec-42] ERROR c.e.s.PaymentProcessor - Failed to process payment transaction: gateway timeout after 3 retries user_id=1234567 account_id=acct_9f8e7d6c5b4a3210 request_id=req-2024-01-15-10-30-45-abcdef123456 trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7 method=POST path=/api/v1/payments/charge status=500 duration_ms=1234 amount=9999 currency=USD merchant_id=mrc_abc123def456 idempotency_key=idem_xyz789uvw012 client_ip=203.0.113.42 region=us-east-1 host=ip-10-0-12-34.ec2.internal env=production upstream=processor-gw-03 error_type=GatewayTimeoutException error_code=GTW_504 detail="upstream did not respond within 3000ms" retry_count=3 circuit_breaker=half_open pool_active=42 pool_idle=8 gc_pause_ms=12 heap_used_mb=3840 thread_count=210 open_files=1024 correlation_id=corr-abc-123-xyz-789-def-456 db_query_ms=45 cache_hit=false shard=07 replica=primary datacenter=iad tenant=acme-corp version=2.14.3`)},
	}

	mibPerSec := func(b *testing.B, byteLen int) float64 {
		return float64(byteLen) * float64(b.N) / (1024 * 1024) / b.Elapsed().Seconds()
	}

	for _, tc := range cases {
		b.Run(tc.name+"/Owned", func(b *testing.B) {
			tok := NewTokenizer(0)
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				tok.Tokenize(tc.input)
			}
			b.ReportMetric(mibPerSec(b, len(tc.input)), "MiB/s")
		})

		b.Run(tc.name+"/Borrowed", func(b *testing.B) {
			tok := NewTokenizer(0)
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				tok.tokenizeBorrowed(tc.input)
			}
			b.ReportMetric(mibPerSec(b, len(tc.input)), "MiB/s")
		})
	}
}

// BenchmarkTokenizerShort tests tokenization of very short messages
func BenchmarkTokenizerShort(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("abc123"))
	}
}

// BenchmarkTokenizerMedium tests tokenization of medium-length log lines
func BenchmarkTokenizerMedium(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully"))
	}
}

// BenchmarkTokenizerLong tests tokenization with many different token types
func BenchmarkTokenizerLong(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	}
}

// BenchmarkTokenizerVeryLong tests tokenization with very long log messages
func BenchmarkTokenizerVeryLong(b *testing.B) {
	tok := NewTokenizer(0)
	msg := []byte("2024-01-15T10:30:45.123456Z ERROR [microservice-backend] exception=NullPointerException user_id=12345678 session_id=abcdef1234567890 " +
		"request_id=req-2024-01-15-10-30-45-123456 path=/api/v1/users/profile method=GET status=500 duration_ms=1234 " +
		"{'error': 'Internal server error', 'stack_trace': 'at com.example.Service.method(Service.java:123)', " +
		"'details': {'query_params': {'filter': 'active', 'sort': 'created_at'}, 'headers': {'User-Agent': 'Mozilla/5.0'}}} " +
		"remote_addr=192.168.1.100 forwarded_for=10.0.0.1,172.16.0.1 correlation_id=corr-abc-123-xyz-789")
	for n := 0; n < b.N; n++ {
		tok.Tokenize(msg)
	}
}

// BenchmarkTokenizerJSON tests JSON-heavy log messages
func BenchmarkTokenizerJSON(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte(`{"timestamp":"2024-01-15T10:30:45.123Z","level":"INFO","service":"api","message":"Request completed","duration_ms":125,"user_id":12345,"metadata":{"version":"1.2.3","env":"prod"}}`))
	}
}

// BenchmarkTokenizerTimestampHeavy tests messages with multiple timestamp formats
func BenchmarkTokenizerTimestampHeavy(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("Mon Jan 15 10:30:45 PST 2024 | 2024-01-15T10:30:45.123Z | 15/Jan/2024:10:30:45 -0800 | Jan 15, 2024 10:30 AM EDT"))
	}
}

// BenchmarkTokenizerNumberHeavy tests messages with many numbers
func BenchmarkTokenizerNumberHeavy(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("cpu=45.67 memory=2048 disk=512000 network_tx=1234567890 network_rx=9876543210 connections=100 threads=32 load_avg=1.23,2.34,3.45"))
	}
}

// BenchmarkTokenizerSpecialCharsHeavy tests messages with many special characters
func BenchmarkTokenizerSpecialCharsHeavy(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("!@#$%^&*()_+-=[]{}|\\:;\"'<>,.?/~`!@#$%^&*()_+-=[]{}|\\:;\"'<>,.?/~`"))
	}
}

// BenchmarkTokenizerStackTrace tests log messages with stack traces
func BenchmarkTokenizerStackTrace(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("at com.example.Service.handleRequest(Service.java:123) at com.example.Controller.process(Controller.java:456) at javax.servlet.http.HttpServlet.service(HttpServlet.java:789)"))
	}
}

// BenchmarkTokenizerMixedTimezones tests messages with various timezone abbreviations
func BenchmarkTokenizerMixedTimezones(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("UTC GMT EST EDT CST CDT MST MDT PST PDT JST KST IST MSK CEST CET BST NZST NZDT ACST ACDT AEST AEDT AWST AKST HST CHST"))
	}
}

// BenchmarkTokenizerAllMonthsDays tests messages with all month and day names
func BenchmarkTokenizerAllMonthsDays(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("JAN FEB MAR APR MAY JUN JUL AUG SEP OCT NOV DEC MON TUE WED THU FRI SAT SUN AM PM"))
	}
}

// BenchmarkTokenizerLongWords tests messages with very long character runs
func BenchmarkTokenizerLongWords(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("thisIsAVeryLongWordWithoutAnySpacesOrSpecialCharactersJustToTestHowTheTokenizerHandlesLongCharacterRunsInTheInput"))
	}
}

// BenchmarkTokenizerLongNumbers tests messages with very long number sequences
func BenchmarkTokenizerLongNumbers(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("12345678901234567890123456789012345678901234567890123456789012345678901234567890"))
	}
}

// BenchmarkTokenizerPathsAndURLs tests file paths and URLs
func BenchmarkTokenizerPathsAndURLs(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("/var/log/application/app.log /usr/local/bin/service C:\\Windows\\System32\\drivers\\etc\\hosts https://api.example.com:8443/v1/users?id=123&filter=active"))
	}
}

// BenchmarkTokenizerRealisticApacheLog tests realistic Apache access log format
func BenchmarkTokenizerRealisticApacheLog(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte(`192.168.1.100 - user123 [15/Jan/2024:10:30:45 -0800] "GET /api/v1/users/profile?id=12345 HTTP/1.1" 200 4567 "https://example.com" "Mozilla/5.0 (X11; Linux x86_64)"`))
	}
}

// BenchmarkTokenizerRealisticAppLog tests realistic application log format
func BenchmarkTokenizerRealisticAppLog(b *testing.B) {
	tok := NewTokenizer(0)
	for n := 0; n < b.N; n++ {
		tok.Tokenize([]byte("2024-01-15 10:30:45.123 [pool-1-thread-15] INFO  c.e.s.UserService - User authentication successful for user_id=12345, session=abc123, ip=192.168.1.100"))
	}
}

func BenchmarkTokenizerIsMatchNoMatchStart(b *testing.B) {
	tok := NewTokenizer(0)
	ta, _ := tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tok.Tokenize([]byte("$ abc foo bar thie beginning is different !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IsMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchNoMatchEnd(b *testing.B) {
	tok := NewTokenizer(0)
	ta, _ := tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST But this one is different near the end of the sequence"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IsMatch(ta, tb, 0.75)
	}
}

func BenchmarkTokenizerIsMatchFullMatch(b *testing.B) {
	tok := NewTokenizer(0)
	ta, _ := tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))
	tb, _ := tok.Tokenize([]byte("Sun Mar 2PM EST JAN FEB MAR !@#$%^&*()_+[]:-/\\.,\\'{}\"`~ 0123456789 NZST ACDT aaaaaaaaaaaaaaaa CHST T!Z(T)Z#AM 123-abc-[foo] (bar) 12-12-12T12:12:12.12T12:12Z123"))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IsMatch(ta, tb, 0.75)
	}
}
