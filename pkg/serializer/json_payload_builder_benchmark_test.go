//+build zlib

package serializer

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
)

func benchmarkJSONPayloadBuilderThroughput(points int, items int, runs int) {
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
			Tags:     []string{"tag1", "tag2:yes"},
		})
	}
	json, _ := series.MarshalJSON()
	initialSize := len(json)

	payloadBuilder := stream.NewJSONPayloadBuilder(true)
	var totalTime time.Duration

	for i := 0; i < runs; i++ {
		start := time.Now()
		payloadBuilder.Build(series)
		totalTime += time.Since(start)
	}
	avgTime := int64(totalTime) / int64(runs)
	speed := float64(initialSize) / (float64(avgTime) / float64(time.Second))
	megabyte := float64(1024 * 1024)
	fmt.Printf("inputSize: %d bytes \t avg duration: %s \t throughput: %f MB/sec\n", initialSize, fmt.Sprint(time.Duration(avgTime)), speed/megabyte)
}

func TestJSONPayloadBuilderThroughput(t *testing.T) {
	// # of points and items chosen to be approximately the same # of bytes per payload between tests
	benchmarkJSONPayloadBuilderThroughput(0, 21000, 10)
	benchmarkJSONPayloadBuilderThroughput(1, 20000, 10)
	benchmarkJSONPayloadBuilderThroughput(2, 20000, 10)
	benchmarkJSONPayloadBuilderThroughput(5, 15000, 10)
	benchmarkJSONPayloadBuilderThroughput(10, 10000, 10)
	benchmarkJSONPayloadBuilderThroughput(100, 2000, 10)
	benchmarkJSONPayloadBuilderThroughput(200, 1000, 10)
}
