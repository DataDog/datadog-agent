package dogstatsd

import (
	"fmt"
	"testing"
)

func buildRawSample(tagCount int) []byte {
	tags := "tag0:val0"
	for i := 1; i < tagCount; i++ {
		tags += fmt.Sprintf(",tag%d:val%d", i, i)
	}

	return []byte(fmt.Sprintf("daemon:666|h|@0.5|#%s", tags))
}

// used to store the result and avoid optimizations
var sample MetricSample

func BenchmarkParseMetric(b *testing.B) {
	for i := 1; i < 1000; i *= 4 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			rawSample := buildRawSample(i)
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				sample, _ = parseMetricMessage(rawSample, "", []string{}, "default-hostname")
			}
		})
	}
}
