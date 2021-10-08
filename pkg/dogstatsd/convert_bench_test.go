package dogstatsd

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func buildRawSample(tagCount int, multipleValues bool) []byte {
	tags := "tag0:val0"
	for i := 1; i < tagCount; i++ {
		tags += fmt.Sprintf(",tag%d:val%d", i, i)
	}

	if multipleValues {
		return []byte(fmt.Sprintf("daemon:666:777|h|@0.5|#%s", tags))
	}
	return []byte(fmt.Sprintf("daemon:666|h|@0.5|#%s", tags))
}

// used to store the result and avoid optimizations
var (
	benchSamples []metrics.MetricSample
)

func runParseMetricBenchmark(b *testing.B, multipleValues bool) {
	parser := newParser(newFloat64ListPool())
	namespaceBlacklist := []string{}

	for i := 1; i < 1000; i *= 4 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			rawSample := buildRawSample(i, multipleValues)
			sb.ResetTimer()
			samples := make([]metrics.MetricSample, 0, 2)

			for n := 0; n < sb.N; n++ {

				parsed, err := parser.parseMetricSample(rawSample)
				if err != nil {
					continue
				}

				benchSamples = enrichMetricSample(samples, parsed, "", namespaceBlacklist, "default-hostname", "", true, false)
			}
		})
	}
}

func BenchmarkParseMetric(b *testing.B) {
	runParseMetricBenchmark(b, false)
}

func BenchmarkParseMultipleMetric(b *testing.B) {
	runParseMetricBenchmark(b, true)
}
