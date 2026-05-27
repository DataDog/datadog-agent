// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Realistic log corpus representing production workloads.
var loadBenchCorpus = [][]byte{
	[]byte("2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully user_id=12345 duration_ms=42 path=/api/v1/users"),
	[]byte("2024-01-15 10:30:46,789 INFO c.e.s.UserService - Authentication successful session=abc123def ip=192.168.1.100"),
	[]byte("Jan 15 10:30:47 web-01 nginx: 192.168.1.100 - - [15/Jan/2024:10:30:47 -0800] \"GET /api/v1/health HTTP/1.1\" 200 15"),
	[]byte(`{"timestamp":"2024-01-15T10:30:48.000Z","level":"INFO","service":"payment","message":"Transaction completed","amount":99.99,"currency":"USD"}`),
	[]byte("2024-01-15 10:30:49.111 [pool-1-thread-15] DEBUG c.e.s.CacheManager - Cache hit key=user:5678 ttl=300 store=redis"),
	[]byte("2024-01-15 10:30:50.222 WARN [service-name] Connection pool low available=2 max=50 wait_time_ms=150"),
	[]byte("Mon Jan 15 10:30:51 PST 2024 | audit | user=admin action=delete resource=record id=9999 result=ok"),
	[]byte("2024-01-15T10:30:52.333Z DEBUG [grpc-server] method=GetUser status=OK latency=5ms peer=10.0.0.1:52341"),
	[]byte("2024-01-15T10:30:53.444Z ERROR [service-name] exception=NullPointerException user_id=12345 request_id=req-abc-123"),
	[]byte("at com.example.Service.handleRequest(Service.java:123) at com.example.Controller.process(Controller.java:456)"),
	[]byte("cpu=45.67 memory=2048 disk=512000 network_tx=1234567890 network_rx=9876543210 connections=100 threads=32"),
	[]byte("2024-01-15 10:30:54 health status=ok latency_p99=12ms connections=128 workers=16 queue_depth=0"),
	[]byte("2024-01-15T10:30:55.555Z stdout F {\"log\":\"Starting container\",\"stream\":\"stdout\",\"time\":\"2024-01-15T10:30:55.555Z\"}"),
	[]byte("/var/log/containers/web-deployment-abc123_default_web-abc123.log 2024-01-15T10:30:56Z INFO ready"),
	[]byte(`192.168.1.100 - user123 [15/Jan/2024:10:30:57 -0800] "GET /api/v1/users/profile?id=12345&filter=active HTTP/1.1" 200 4567 "https://example.com" "Mozilla/5.0"`),
	[]byte(`10.0.0.1 - - [15/Jan/2024:10:30:58 +0000] "POST /api/v2/events HTTP/2.0" 202 0 "-" "datadog-agent/7.50.0"`),
}

// ── Helpers ──

func loadBenchMakeFillPatterns(n int) [][]Token {
	tok := NewTokenizer(0)
	templates := []string{
		"metric cpu_usage=45.67 host=web-01 env=prod ts=1234567890",
		"audit user=admin action=delete resource=record id=9999 result=ok",
		"health status=ok latency_p99=12ms connections=128 workers=16",
		"cache hit key=user:5678 ttl=300 size=1024 store=redis",
		"http 192.168.1.100 GET /api/v2/items?page=2 200 512 12ms",
		"deploy version=2.3.1 env=staging region=us-east-1 ok=true",
		"queue enqueue job=email_send priority=5 delay=0 id=abc123",
		"storage read path=/data/shard-4/chunk-17 bytes=65536 ms=3",
		"auth token=eyJhbGciOiJSUzI1NiJ9 user=42 scope=read exp=3600",
		"grpc method=GetUser status=OK latency=5ms peer=10.0.0.1",
	}
	patterns := make([][]Token, n)
	for i := range n {
		tokens, _ := tok.Tokenize([]byte(templates[i%len(templates)]))
		cp := make([]Token, len(tokens))
		copy(cp, tokens)
		patterns[i] = cp
	}
	return patterns
}

func loadBenchPrefillSampler(s *AdaptiveSampler, patterns [][]Token, count int) {
	now := time.Now()
	for i := range count {
		s.entries = append(s.entries, samplerEntry{
			tokens:     patterns[i%len(patterns)],
			credits:    s.config.BurstSize,
			lastSeen:   now,
			matchCount: 1,
		})
	}
}

func loadBenchNewSampler(maxPatterns int) *AdaptiveSampler {
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    maxPatterns,
		RateLimit:      1e9,
		BurstSize:      math.MaxFloat64 / 2,
		MatchThreshold: 0.75,
	}, "bench")
}

func loadBenchMsg() *message.Message {
	return message.NewMessage([]byte("2024-01-15 10:30:45.123 INFO [service-a] Request processed user_id=12345"), nil, message.StatusInfo, 0)
}

// ──────────────────────────────────────────────────────
// 1. Tokenizer under production maxEvalBytes settings
// ──────────────────────────────────────────────────────

func BenchmarkTokenizer_ProductionLabeler_60bytes(b *testing.B) {
	tok := NewTokenizer(60)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
	}
}

func BenchmarkTokenizer_ProductionSampler_2048bytes(b *testing.B) {
	tok := NewTokenizer(2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
	}
}

func BenchmarkTokenizer_Unlimited(b *testing.B) {
	tok := NewTokenizer(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
	}
}

// ──────────────────────────────────────────────────────
// 2. Tokenize + TimestampDetector (labeler hot path)
// ──────────────────────────────────────────────────────

func BenchmarkTokenizeAndTimestampDetect(b *testing.B) {
	tok := NewTokenizer(60)
	td := NewTimestampDetector(0.5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := loadBenchCorpus[i%len(loadBenchCorpus)]
		tokens, indices := tok.Tokenize(msg)
		ctx := &messageContext{
			rawMessage:      msg,
			tokens:          tokens,
			tokenIndicies:   indices,
			label:           aggregate,
			labelAssignedBy: defaultLabelSource,
		}
		td.ProcessAndContinue(ctx)
	}
}

// ──────────────────────────────────────────────────────
// 3. Tokenize + IsMatch scan (adaptive sampler hot path)
// ──────────────────────────────────────────────────────

func BenchmarkTokenizeAndSamplerScan(b *testing.B) {
	for _, patternCount := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("SteadyState_P%d", patternCount), func(b *testing.B) {
			tok := NewTokenizer(2048)
			s := loadBenchNewSampler(patternCount + 100)
			fillPatterns := loadBenchMakeFillPatterns(patternCount)
			loadBenchPrefillSampler(s, fillPatterns, patternCount)

			msg := loadBenchMsg()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tokens, _ := tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
				s.Process(msg, tokens)
			}
		})
	}

	// Worst-case: no-match forces full table scan every iteration.
	for _, patternCount := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("FullScan_P%d", patternCount), func(b *testing.B) {
			noMatchTokens, _ := NewTokenizer(2048).Tokenize([]byte("ZZZZZZZ_unique_no_match_9999"))
			msg := message.NewMessage([]byte("ZZZZZZZ_unique_no_match_9999"), nil, message.StatusInfo, 0)

			s := loadBenchNewSampler(patternCount * 2)
			fillPats := loadBenchMakeFillPatterns(patternCount)
			loadBenchPrefillSampler(s, fillPats, patternCount)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.Process(msg, noMatchTokens)
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// 4. Full preprocessor pipeline
// ──────────────────────────────────────────────────────

func BenchmarkPreprocessorPipeline(b *testing.B) {
	for _, samplerType := range []string{"noop", "adaptive_100", "adaptive_1000"} {
		b.Run(samplerType, func(b *testing.B) {
			outChan := make(chan *message.Message, 256)
			tok := NewTokenizer(2048)
			lab := NewLabeler([]Heuristic{NewJSONDetector(), NewTimestampDetector(0.5)}, nil)
			agg := NewPassThroughAggregator(256 * 1024)

			var sampler Sampler
			switch samplerType {
			case "noop":
				sampler = NewNoopSampler()
			case "adaptive_100":
				s := NewAdaptiveSampler(AdaptiveSamplerConfig{
					MaxPatterns: 100, RateLimit: 1.0, BurstSize: 1000.0,
					MatchThreshold: 0.9, ProtectImportantLogs: true,
				}, "bench")
				loadBenchPrefillSampler(s, loadBenchMakeFillPatterns(100), 50)
				sampler = s
			case "adaptive_1000":
				s := NewAdaptiveSampler(AdaptiveSamplerConfig{
					MaxPatterns: 1000, RateLimit: 1.0, BurstSize: 1000.0,
					MatchThreshold: 0.9, ProtectImportantLogs: true,
				}, "bench")
				loadBenchPrefillSampler(s, loadBenchMakeFillPatterns(1000), 500)
				sampler = s
			}

			pp := NewPreprocessor(agg, tok, lab, sampler, outChan, NewNoopJSONAggregator(), 5*time.Second, 60)
			go func() {
				for range outChan {
				}
			}()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				raw := loadBenchCorpus[i%len(loadBenchCorpus)]
				pp.Process(message.NewMessage(raw, nil, message.StatusInfo, 0))
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// 5. Concurrent throughput (multiple tailers)
// ──────────────────────────────────────────────────────

func BenchmarkTokenizer_Concurrent(b *testing.B) {
	for _, goroutines := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("Goroutines_%d", goroutines), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var wg sync.WaitGroup
			perGoroutine := b.N / goroutines
			if perGoroutine == 0 {
				perGoroutine = 1
			}
			for g := range goroutines {
				wg.Add(1)
				go func(seed int) {
					defer wg.Done()
					tok := NewTokenizer(2048)
					for i := 0; i < perGoroutine; i++ {
						tok.Tokenize(loadBenchCorpus[(seed+i)%len(loadBenchCorpus)])
					}
				}(g * perGoroutine)
			}
			wg.Wait()
		})
	}
}

func BenchmarkPreprocessorPipeline_Concurrent(b *testing.B) {
	for _, goroutines := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("Goroutines_%d", goroutines), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var wg sync.WaitGroup
			perGoroutine := b.N / goroutines
			if perGoroutine == 0 {
				perGoroutine = 1
			}
			for g := range goroutines {
				wg.Add(1)
				go func(seed int) {
					defer wg.Done()
					outChan := make(chan *message.Message, 256)
					tok := NewTokenizer(2048)
					lab := NewLabeler([]Heuristic{NewJSONDetector(), NewTimestampDetector(0.5)}, nil)
					agg := NewPassThroughAggregator(256 * 1024)
					s := NewAdaptiveSampler(AdaptiveSamplerConfig{
						MaxPatterns: 100, RateLimit: 1e9, BurstSize: math.MaxFloat64 / 2,
						MatchThreshold: 0.9, ProtectImportantLogs: true,
					}, "bench")
					loadBenchPrefillSampler(s, loadBenchMakeFillPatterns(100), 50)
					pp := NewPreprocessor(agg, tok, lab, s, outChan, NewNoopJSONAggregator(), 5*time.Second, 60)
					go func() {
						for range outChan {
						}
					}()
					for i := 0; i < perGoroutine; i++ {
						raw := loadBenchCorpus[(seed+i)%len(loadBenchCorpus)]
						pp.Process(message.NewMessage(raw, nil, message.StatusInfo, 0))
					}
					close(outChan)
				}(g * perGoroutine)
			}
			wg.Wait()
		})
	}
}

// ──────────────────────────────────────────────────────
// 6. Allocation profiling
// ──────────────────────────────────────────────────────

func BenchmarkTokenizer_AllocationProfile(b *testing.B) {
	for _, name := range []string{"short_60b", "medium_200b", "long_500b"} {
		b.Run(name, func(b *testing.B) {
			var input []byte
			switch name {
			case "short_60b":
				input = []byte("2024-01-15T10:30:45.123Z INFO ok")
			case "medium_200b":
				input = loadBenchCorpus[0]
			case "long_500b":
				input = []byte(`192.168.1.100 - user123 [15/Jan/2024:10:30:57 -0800] "GET /api/v1/users/profile?id=12345&filter=active HTTP/1.1" 200 4567 "https://example.com" "Mozilla/5.0 (X11; Linux x86_64)" forwarded_for=10.0.0.1,172.16.0.1 request_id=req-2024-01-15-abc-xyz trace_id=abcdef1234567890 span_id=1234567890abcdef parent_id=fedcba0987654321 service=web-frontend env=production version=2.3.1`)
			}
			tok := NewTokenizer(2048)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tok.Tokenize(input)
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// 7. IsMatch scaling
// ──────────────────────────────────────────────────────

func BenchmarkIsMatch_Scaling(b *testing.B) {
	tok := NewTokenizer(2048)
	target, _ := tok.Tokenize(loadBenchCorpus[0])
	for _, seqLen := range []int{10, 30, 60, 100} {
		b.Run(fmt.Sprintf("TokenLen_%d", seqLen), func(b *testing.B) {
			a := target
			if len(a) > seqLen {
				a = a[:seqLen]
			}
			nonMatch := make([]Token, len(a))
			for i := range nonMatch {
				nonMatch[i] = Token((int(a[i]) + 5) % int(End))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				IsMatch(a, nonMatch, 0.9)
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// 8. TokenGraph.MatchProbability
// ──────────────────────────────────────────────────────

func BenchmarkTokenGraph_MatchProbability(b *testing.B) {
	tok := NewTokenizer(60)
	for _, name := range []string{"timestamp", "no_timestamp", "json"} {
		b.Run(name, func(b *testing.B) {
			var tokens []Token
			switch name {
			case "timestamp":
				tokens, _ = tok.Tokenize([]byte("2024-01-15T10:30:45.123Z INFO"))
			case "no_timestamp":
				tokens, _ = tok.Tokenize([]byte("cpu=45.67 memory=2048 disk=512000 connections=100"))
			case "json":
				tokens, _ = tok.Tokenize([]byte(`{"timestamp":"2024-01-15T10:30:48.000Z","level":"INFO"}`))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				staticTokenGraph.MatchProbability(tokens)
			}
		})
	}
}
