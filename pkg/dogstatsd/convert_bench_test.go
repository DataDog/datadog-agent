package dogstatsd

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func buildRawSample(tagCount int) []byte {
	tags := "tag0:val0"
	for i := 1; i < tagCount; i++ {
		tags += fmt.Sprintf(",tag%d:val%d", i, i)
	}

	return []byte(fmt.Sprintf("daemon:666|h|@0.5|#%s", tags))
}

// used to store the result and avoid optimizations
var sample metrics.MetricSample

func BenchmarkParseMetric(b *testing.B) {
	for i := 1; i < 1000; i *= 4 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			rawSample := buildRawSample(i)
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				sample, _ = parseAndEnrichMetricMessage(rawSample, "", []string{}, "default-hostname")
			}
		})
	}
}
