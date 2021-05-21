//+build zlib

package serializer

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
)

func generateData(points int, items int, tags int) metrics.Series {
	series := metrics.Series{}
	for i := 0; i < items; i++ {
		series = append(series, &metrics.Serie{
			Points: func() []metrics.Point {
				ps := make([]metrics.Point, points)
				for p := 0; p < points; p++ {
					ps[p] = metrics.Point{Ts: float64(p * i), Value: float64(p + i)}
				}
				return ps
			}(),
			MType:    metrics.APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags: func() []string {
				ts := make([]string, tags)
				for t := 0; t < tags; t++ {
					ts[t] = fmt.Sprintf("tag%d:foobar", t)
				}
				return ts
			}(),
		})
	}
	return series
}

func benchmarkJSONPayloadBuilderUsage(b *testing.B, points int, items int, tags int) {

	series := generateData(points, items, tags)
	payloadBuilder := stream.NewJSONPayloadBuilder(true)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		payloadBuilder.Build(series)
	}
}

func benchmarkJSONPayloadBuilderThroughput(points int, items int, tags int, runs int) { //nolint:unuse
	series := generateData(points, items, tags)
	json, _ := series.MarshalJSON()
	initialSize := len(json)
	metricsCount := len(series)

	payloadBuilder := stream.NewJSONPayloadBuilder(true)
	var totalTime time.Duration

	for i := 0; i < runs; i++ {
		start := time.Now()
		payloadBuilder.Build(series)
		totalTime += time.Since(start)
	}
	avgTime := int64(totalTime) / int64(runs)
	speed := float64(initialSize) / (float64(avgTime) / float64(time.Second))
	metricRate := int(float64(metricsCount) / (float64(avgTime) / float64(time.Second)))
	megabyte := float64(1024 * 1024)
	fmt.Printf("inputSize: %d bytes \t # of metrics: %d \t tags: %d \t points: %d \t avg duration: %s \t throughput: %f MB/sec \t metrics/sec: %d\n", initialSize, metricsCount, tags, points, fmt.Sprint(time.Duration(avgTime)), speed/megabyte, metricRate)
}

// Uncomment to run these benchmarks. These are non-standard benchmarks to collect custom metrics
// so they should not be run as part of normal CI.

// func TestJSONPayloadBuilderThroughputPoints(t *testing.T) {
// 	// # of points and items chosen to be approximately the same # of bytes per payload between tests
// 	benchmarkJSONPayloadBuilderThroughput(0, 21000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 20000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(2, 20000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(5, 15000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(10, 10000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(100, 2000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(200, 1000, 1, 10)
// }

// func TestJSONPayloadBuilderThroughputTags(t *testing.T) {
// 	// # of points and items chosen to be approximately the same # of bytes per payload between tests
// 	benchmarkJSONPayloadBuilderThroughput(1, 21000, 1, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 21000, 2, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 19000, 5, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 15000, 10, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 2000, 100, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 200, 1000, 10)
// 	benchmarkJSONPayloadBuilderThroughput(1, 20, 10000, 10)
// }

// func TestJSONPayloadBuilderThroughputHighRate(t *testing.T) {
// 	// warning - These tests are very slow
// 	benchmarkJSONPayloadBuilderThroughput(1, 1000000, 1, 1)
// 	benchmarkJSONPayloadBuilderThroughput(1, 1000000, 10, 1)
// 	benchmarkJSONPayloadBuilderThroughput(2, 1000000, 1, 1)
// 	benchmarkJSONPayloadBuilderThroughput(2, 1000000, 10, 1)
// 	benchmarkJSONPayloadBuilderThroughput(4, 500000, 1, 1)
// 	benchmarkJSONPayloadBuilderThroughput(4, 500000, 10, 1)
// }

func BenchmarkJSONPayloadBuilderThroughputPoints0(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 0, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints2(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 2, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints5(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 5, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 10, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 100, 100, 1)
}

func BenchmarkJSONPayloadBuilderTags0(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 0)
}
func BenchmarkJSONPayloadBuilderTags1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderTags2(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 2)
}
func BenchmarkJSONPayloadBuilderTags5(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 5)
}
func BenchmarkJSONPayloadBuilderTags10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 10)
}
func BenchmarkJSONPayloadBuilderTags100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 100)
}

func BenchmarkJSONPayloadBuilderItems1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 1, 1)
}
func BenchmarkJSONPayloadBuilderItems10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 10, 1)
}
func BenchmarkJSONPayloadBuilderItems100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderItems1000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 1000, 1)
}
func BenchmarkJSONPayloadBuilderItems10000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 10000, 1)
}
func BenchmarkJSONPayloadBuilderItems100000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100000, 1)
}
