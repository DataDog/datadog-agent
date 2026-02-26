//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"testing"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// pipelineCorpus mirrors the tokenizer benchmark corpus so results are directly comparable.
// Covers the log types most likely to stress tokenization + clustering together.
var pipelineCorpus = []string{
	`192.168.1.105 - frank [10/Oct/2024:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326`,
	`GET /api/v1/users/123/profile HTTP/1.1 200 OK 145ms`,
	`POST /api/v2/events HTTP/1.1 500 Internal Server Error 2341ms`,
	`ERROR 2024-10-10T13:55:36.123Z [main-thread] com.example.App - Failed to connect to database: connection refused (attempt 3/5)`,
	`WARN  2024-10-10T13:55:36.456Z [worker-2] com.example.Cache - Cache miss rate 87.3% exceeds threshold 80.0%`,
	`INFO  2024-10-10T13:55:37.001Z [scheduler] com.example.Job - Job completed successfully in 1234ms, processed 9823 records`,
	`E1010 13:55:36.789012   12345 reflector.go:153] Failed to list *v1.Pod: Get "https://10.96.0.1:443/api/v1/pods": dial tcp 10.96.0.1:443: connect: connection refused`,
	`I1010 13:55:37.000000   12345 controller.go:217] Successfully synced "default/my-deployment"`,
	`Oct 10 13:55:36 myhost sshd[12345]: Accepted publickey for admin from 10.0.1.50 port 54321 ssh2`,
	`Oct 10 13:55:36 myhost kernel: [123456.789] Out of memory: Kill process 9876 (java) score 892 or sacrifice child`,
	`2024-10-10T13:55:36.123456Z 42 [Warning] InnoDB: page_cleaner: 1000ms intended loop took 4321ms. The settings might not be optimal.`,
	`2024-10-10 13:55:36 UTC [12345]: user=app,db=production,app=rails,client=10.0.1.20 LOG: connection received: host=10.0.1.20 port=54321`,
	`ERROR: null pointer exception at line 42`,
	`Server started on port 8080`,
	`Processing request a1b2c3d4-e5f6-7890-abcd-ef1234567890 for user 99182`,
}

// benchSource is a minimal LogSource for benchmark messages.
var benchSource = sources.NewLogSource("bench", &logsconfig.LogsConfig{})

// newTestMessage builds a minimal message.Message for pipeline benchmarking.
func newTestMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), message.NewOrigin(benchSource), "", 0)
	msg.MessageMetadata.RawDataLen = len(content)
	return msg
}

// runPipelineBenchmark benchmarks the full processMessage pipeline with the given tokenizer.
// It warms up the cluster manager first (pre-builds patterns) then measures steady-state.
func runPipelineBenchmark(b *testing.B, tok token.Tokenizer) {
	b.Helper()

	msgs := make([]*message.Message, len(pipelineCorpus))
	for i, content := range pipelineCorpus {
		msgs[i] = newTestMessage(content)
	}

	mt := NewMessageTranslator("bench", tok)

	// Drain output channel in background to prevent processMessage from blocking.
	outputChan := make(chan *message.StatefulMessage, 1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range outputChan {
		}
	}()

	// Warm up: run full corpus once so the cluster manager has established patterns.
	// This simulates steady-state (most logs match existing patterns = less clustering work).
	for _, msg := range msgs {
		mt.processMessage(msg, outputChan)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mt.processMessage(msgs[i%len(msgs)], outputChan)
	}

	b.StopTimer()
	close(outputChan)
	<-done
}

// BenchmarkPipeline_GoTokenizer benchmarks the full processMessage pipeline using the Go automaton tokenizer.
func BenchmarkPipeline_GoTokenizer(b *testing.B) {
	runPipelineBenchmark(b, automaton.NewTokenizer())
}

// BenchmarkPipeline_RustTokenizer benchmarks the full processMessage pipeline using the Rust tokenizer.
func BenchmarkPipeline_RustTokenizer(b *testing.B) {
	runPipelineBenchmark(b, rtokenizer.NewRustTokenizer())
}
