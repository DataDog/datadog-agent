//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// logCorpus is a realistic mixed set of log lines covering common log types.
// Designed to stress both tokenizers across a variety of token patterns.
var logCorpus = []struct {
	name string
	log  string
}{
	// HTTP access logs
	{"http_access", `192.168.1.105 - frank [10/Oct/2024:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326`},
	{"http_api", `GET /api/v1/users/123/profile HTTP/1.1 200 OK 145ms`},
	{"http_error", `POST /api/v2/events HTTP/1.1 500 Internal Server Error 2341ms`},

	// Structured app logs
	{"error_stack", `ERROR 2024-10-10T13:55:36.123Z [main-thread] com.example.App - Failed to connect to database: connection refused (attempt 3/5)`},
	{"warn_log", `WARN  2024-10-10T13:55:36.456Z [worker-2] com.example.Cache - Cache miss rate 87.3% exceeds threshold 80.0%`},
	{"info_log", `INFO  2024-10-10T13:55:37.001Z [scheduler] com.example.Job - Job completed successfully in 1234ms, processed 9823 records`},

	// Kubernetes / container logs
	{"k8s_event", `E1010 13:55:36.789012   12345 reflector.go:153] Failed to list *v1.Pod: Get "https://10.96.0.1:443/api/v1/pods": dial tcp 10.96.0.1:443: connect: connection refused`},
	{"k8s_info", `I1010 13:55:37.000000   12345 controller.go:217] Successfully synced "default/my-deployment"`},

	// System / syslog
	{"syslog", `Oct 10 13:55:36 myhost sshd[12345]: Accepted publickey for admin from 10.0.1.50 port 54321 ssh2`},
	{"syslog_error", `Oct 10 13:55:36 myhost kernel: [123456.789] Out of memory: Kill process 9876 (java) score 892 or sacrifice child`},

	// Database logs
	{"db_slow_query", `2024-10-10T13:55:36.123456Z 42 [Warning] InnoDB: page_cleaner: 1000ms intended loop took 4321ms. The settings might not be optimal.`},
	{"db_connection", `2024-10-10 13:55:36 UTC [12345]: user=app,db=production,app=rails,client=10.0.1.20 LOG: connection received: host=10.0.1.20 port=54321`},

	// Simple / short logs
	{"simple_error", `ERROR: null pointer exception at line 42`},
	{"simple_info", `Server started on port 8080`},
	{"uuid_log", `Processing request a1b2c3d4-e5f6-7890-abcd-ef1234567890 for user 99182`},
}

func runBenchmark(b *testing.B, tok token.Tokenizer) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := logCorpus[i%len(logCorpus)]
		_, _ = tok.Tokenize(entry.log)
	}
}

// BenchmarkGoTokenizer benchmarks the Go automaton tokenizer across the mixed log corpus.
func BenchmarkGoTokenizer(b *testing.B) {
	tok := automaton.NewTokenizer()
	runBenchmark(b, tok)
}

// BenchmarkRustTokenizer benchmarks the Rust tokenizer across the mixed log corpus.
func BenchmarkRustTokenizer(b *testing.B) {
	tok := NewRustTokenizer()
	runBenchmark(b, tok)
}

// Sub-benchmarks by log type — useful for identifying which log categories
// drive the biggest difference between tokenizers.
func BenchmarkGoTokenizer_ByType(b *testing.B) {
	tok := automaton.NewTokenizer()
	for _, entry := range logCorpus {
		entry := entry
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = tok.Tokenize(entry.log)
			}
		})
	}
}

func BenchmarkRustTokenizer_ByType(b *testing.B) {
	tok := NewRustTokenizer()
	for _, entry := range logCorpus {
		entry := entry
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = tok.Tokenize(entry.log)
			}
		})
	}
}
