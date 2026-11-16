// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"fmt"
	"testing"
)

var benchLines = []string{
	`{"msg":"log from series 12","level":"info"}`,
	`{"@timestamp":"2024-01-01T00:00:00Z","message":"evt-7","severity":"WARN","svc":"api"}`,
	`{"trace_id":"deadbeef","span_id":"cafebabe","msg":"child span","ok":true}`,
	`{"nested":{"user":42,"shard":3},"event":"login","ip":"10.0.0.42"}`,
	`level=INFO ts=1704067200 series=99 msg="request done" duration_ms=12`,
	`level=ERROR logger=com.example req=51 stack=java.lang.Exception`,
	`[2024-01-15 14:30:00] INFO  worker-3  task=flush completed=true`,
	`<134>1 2024-01-15T14:30:00Z host app-7 - - - msg="syslog style"`,
	`10.1.2.3 - - [15/Jan/2024:14:30:00 +0000] "GET /api/v3/items HTTP/1.1" 200 4321`,
	`time="2024-01-15T14:30:00Z" level=debug msg="slow query" series=22 ms=450`,
	`ERROR: connection reset by peer series=8 errno=104`,
	`kafka: topic=logs partition=4 offset=999 key=null`,
	`[pid=12345] series=11 action=gc pause_ms=3`,
	`{"http":{"method":"POST","path":"/hook/3","status":201}}`,
	`plain text line series=5 no json here`,
}

func BenchmarkTokenize(b *testing.B) {
	t := NewTokenizer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range benchLines {
			_ = t.Tokenize(line)
		}
	}
}

func BenchmarkTokenizePerShape(b *testing.B) {
	for idx, line := range benchLines {
		line := line
		b.Run(fmt.Sprintf("shape=%d", idx), func(b *testing.B) {
			t := NewTokenizer()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = t.Tokenize(line)
			}
		})
	}
}

func BenchmarkMessageSignature(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range benchLines {
			_ = MessageSignature(line)
		}
	}
}

func BenchmarkPatternClustererProcess(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pc := NewPatternClusterer()
		for _, line := range benchLines {
			pc.Process(line, 1704067200)
		}
	}
}
