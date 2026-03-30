// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"
)

// diverseLogContent returns distinct line shapes (JSON, kv, syslog, plain) for series s
// so neighboring series exercise different tokenizer / pattern signatures.
func diverseLogContent(s int) []byte {
	switch s % 20 {
	case 0:
		return []byte(fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s))
	case 1:
		return []byte(fmt.Sprintf(`{"@timestamp":"2024-01-01T00:00:00Z","message":"evt-%d","severity":"WARN","svc":"api"}`, s))
	case 2:
		return []byte(fmt.Sprintf(`{"trace_id":"%08x","span_id":"%08x","msg":"child span","ok":true}`, s, s+1))
	case 3:
		return []byte(fmt.Sprintf(`{"nested":{"user":%d,"shard":3},"event":"login","ip":"10.0.0.%d"}`, s, s%255))
	case 4:
		return []byte(fmt.Sprintf(`level=INFO ts=1704067200 series=%d msg="request done" duration_ms=12`, s))
	case 5:
		return []byte(fmt.Sprintf(`level=ERROR logger=com.example req=%d stack=java.lang.Exception`, s))
	case 6:
		return []byte(fmt.Sprintf(`[2024-01-15 14:30:00] INFO  worker-%d  task=flush completed=true`, s))
	case 7:
		return []byte(fmt.Sprintf(`<134>1 2024-01-15T14:30:00Z host app-%d - - - msg="syslog style"`, s))
	case 8:
		return []byte(fmt.Sprintf(`10.1.2.3 - - [15/Jan/2024:14:30:00 +0000] "GET /api/v%d/items HTTP/1.1" 200 4321`, s%50))
	case 9:
		return []byte(fmt.Sprintf(`time="2024-01-15T14:30:00Z" level=debug msg="slow query" series=%d ms=450`, s))
	case 10:
		return []byte(fmt.Sprintf(`{"arr":[%d,2,3],"obj":{"k":"v"},"flag":false}`, s))
	case 11:
		return []byte(fmt.Sprintf(`ERROR: connection reset by peer series=%d errno=104`, s))
	case 12:
		return []byte(fmt.Sprintf(`{"msg":"unicode 测试 %d café","meta":{"region":"eu-west-1"}}`, s))
	case 13:
		return []byte(fmt.Sprintf(`kafka: topic=logs partition=%d offset=999 key=null`, s))
	case 14:
		return []byte(fmt.Sprintf(`{"a":1,"b":%d,"c":{"d":[1,2]}}`, s))
	case 15:
		return []byte(fmt.Sprintf(`[pid=12345] series=%d action=gc pause_ms=3`, s))
	case 16:
		return []byte(fmt.Sprintf(`{"http":{"method":"POST","path":"/hook/%d","status":201}}`, s))
	case 17:
		return []byte(fmt.Sprintf(`plain text line series=%d no json here`, s))
	case 18:
		return []byte(fmt.Sprintf(`{"double":%d.%d,"scientific":1.23e-4}`, s/10, s%10))
	case 19:
		return []byte(fmt.Sprintf(`merge key=value series=%d another=42`, s))
	default:
		return []byte(fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s))
	}
}

// BenchmarkLogExtraction_SeriesCount measures raw log ingestion cost with
// the default catalog extractors across increasing series count.
func BenchmarkLogExtraction_SeriesCount(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     []byte(fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s)),
					status:      "error",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _, extractors, _ := defaultCatalog().Instantiate(benchmarkSettings)
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: extractors,
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}

// BenchmarkLogExtraction_DiversePatterns is like SeriesCount but assigns each series one
// of twenty distinct line shapes (JSON, structured kv, syslog, access log, plain text)
// so ingestion reflects higher pattern cardinality.
func BenchmarkLogExtraction_DiversePatterns(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     diverseLogContent(s),
					status:      "error",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _, extractors, _ := defaultCatalog().Instantiate(benchmarkSettings)
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: extractors,
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}

// BenchmarkLogExtraction_DiversePatterns is like SeriesCount but assigns each series one
// of twenty distinct line shapes (JSON, structured kv, syslog, access log, plain text)
// so ingestion reflects higher pattern cardinality.
func BenchmarkLogExtraction_DiversePatterns(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     diverseLogContent(s),
					status:      "info",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _, extractors, _ := defaultCatalog().Instantiate(benchmarkSettings)
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: extractors,
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}
