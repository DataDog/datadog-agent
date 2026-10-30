// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"bytes"
	"slices"
	"testing"
)

// This file is the benchmark half of the tokenizer's performance contract (see
// doc.go). It uses only the exported API and nothing from other test files, so
// perf_compare.sh can run it unchanged against an older revision.

// consumer is one way the Agent tokenizes every log line it handles, at the
// window the consumer uses in production. Add a consumer here when a new
// package starts tokenizing logs, so the gates and benchmarks cover its calls.
type consumer struct {
	name         string
	maxEvalBytes int
	// perLine is what the consumer does with each line after NewTokenizer.
	perLine func(tok *Tokenizer, line []byte)
}

var consumers = []consumer{
	// Logs preprocessor, default config: logs_config.auto_multi_line.tokenizer_max_input_bytes.
	{"logs_auto_multiline", 60, func(tok *Tokenizer, line []byte) {
		benchSink += tok.TokenizeBorrowed(line).Limit(60).Len()
	}},
	// Logs preprocessor with adaptive sampling or noisy log detection on:
	// logs_config.experimental_adaptive_sampling.tokenizer_max_input_bytes. The
	// labeler still reads only the auto multiline window.
	{"logs_adaptive_sampler", 2048, func(tok *Tokenizer, line []byte) {
		tokens := tok.TokenizeBorrowed(line)
		benchSink += tokens.Limit(60).Len()
		if IsImportant(tokens.Borrow()) {
			benchSink++
		}
	}},
	// AAD log signatures (comp/anomalydetection/observer/impl/logsignature), at
	// the MaxEvalBytes of DefaultLogMetricsExtractorConfig.
	{"aad_log_signature", 4096, func(tok *Tokenizer, line []byte) {
		benchSink += len(TokensToString(tok.TokenizeBorrowed(line).Borrow()))
	}},
}

// perfLines are the line shapes every consumer is measured on: common log
// formats plus the inputs that hit the tokenizer's slower paths (keyword
// promotion, IPv4 collapsing, one token per byte, non-ASCII bytes). Every line
// but short is longer than the smallest window, and dense is longer than every
// window.
var perfLines = []struct {
	name string
	line []byte
}{
	{"short", []byte("abc123 ok")},
	{"applog", appLogLine},
	{"json", []byte(`{"timestamp":"2024-01-15T10:30:45.123456Z","level":"ERROR","logger":"com.example.service.PaymentProcessor","thread":"http-nio-8080-exec-42","service":"payment-api","host":"ip-10-0-12-34.ec2.internal","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","user_id":1234567,"method":"POST","path":"/api/v1/payments/charge","status":500,"duration_ms":1234,"client_ip":"203.0.113.42","message":"Failed to process payment: gateway timeout after 3 retries"}`)},
	{"stacktrace", []byte(`javax.servlet.ServletException: Request processing failed; nested exception is java.lang.NullPointerException at org.springframework.web.servlet.FrameworkServlet.processRequest(FrameworkServlet.java:1014) at org.springframework.web.servlet.FrameworkServlet.doPost(FrameworkServlet.java:914) at javax.servlet.http.HttpServlet.service(HttpServlet.java:681) at org.apache.catalina.core.ApplicationFilterChain.internalDoFilter(ApplicationFilterChain.java:227)`)},
	{"syslog_ipv4", []byte(`Jan 15 10:30:45 gw-03 sshd[4242]: Failed password for root from 203.0.113.42 port 52113 ssh2; forwarded 10.0.12.34 -> 192.168.100.200 via 172.16.0.1`)},
	{"keywords", []byte(`WARN WARNING FATAL ERROR PANIC ALERT SEVERE CRIT CRITICAL EMERG EMERGENCY EXCEPTION FAILED FAILURE TIMEOUT warn error failed timeout info debug trace notice`)},
	{"unicode", []byte(`2024-01-15 10:30:45 用户 登录 失败: 密码 错误 — voilà, l'opération a échoué après 3 tentatives (Ошибка: превышено время ожидания)`)},
	{"dense", bytes.Repeat([]byte("a1."), 2048)},
}

var appLogLine = []byte(`2024-01-15 10:30:45.123 [http-nio-8080-exec-42] ERROR c.e.s.PaymentProcessor - Failed to process payment transaction: gateway timeout after 3 retries user_id=1234567 account_id=acct_9f8e7d6c5b4a3210 request_id=req-2024-01-15-10-30-45-abcdef123456 trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7 method=POST path=/api/v1/payments/charge status=500 duration_ms=1234 amount=9999 currency=USD merchant_id=mrc_abc123def456 client_ip=203.0.113.42 region=us-east-1 host=ip-10-0-12-34.ec2.internal env=production`)

// benchSink keeps the compiler from discarding the work being measured.
var benchSink int

// BenchmarkConsumer measures each consumer's per-line call at its production
// window, on every line shape. perf_compare.sh fails a change that makes any
// of them significantly slower.
func BenchmarkConsumer(b *testing.B) {
	for _, c := range consumers {
		for _, l := range perfLines {
			b.Run(c.name+"/"+l.name, func(b *testing.B) {
				tok := NewTokenizer(c.maxEvalBytes)
				b.SetBytes(int64(min(len(l.line), c.maxEvalBytes)))
				b.ReportAllocs()
				for b.Loop() {
					c.perLine(tok, l.line)
				}
			})
		}
	}
}

// BenchmarkConsumerMatch measures the pattern scans that run on every line:
// the auto multiline pattern table (up to 20 rows, threshold 0.75, 60-byte
// window) and the adaptive sampler (up to 1000 rows, threshold 0.9, 2048-byte
// window). Every row but the last shares the line's first 70% of tokens and
// then differs, so each scan reads most of the line before it fails.
func BenchmarkConsumerMatch(b *testing.B) {
	for _, tc := range []struct {
		name         string
		maxEvalBytes int
		rows         int
		thresh       float64
	}{
		{"logs_pattern_table", 60, 20, 0.75},
		{"logs_adaptive_sampler", 2048, 1000, 0.9},
	} {
		b.Run(tc.name, func(b *testing.B) {
			line, _ := NewTokenizer(tc.maxEvalBytes).Tokenize(appLogLine)
			miss := slices.Clone(line)
			for i := len(miss) * 7 / 10; i < len(miss); i++ {
				miss[i] = End // a token no line produces
			}
			rows := make([][]Token, tc.rows)
			for i := range rows {
				rows[i] = miss
			}
			rows[len(rows)-1] = line
			b.ReportAllocs()
			for b.Loop() {
				for _, row := range rows {
					if IsMatch(row, line, tc.thresh) {
						break
					}
				}
			}
		})
	}
}
