package dogstatsd

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

func buildTags(tagCount int) []string {
	tags := make([]string, 0, tagCount)
	for i := 0; i < tagCount; i++ {
		tags = append(tags, fmt.Sprintf("tag%d:val%d", i, i))
	}

	return tags
}

// used to store the result and avoid optimizations
var tags []string

func BenchmarkEnrichTags(b *testing.B) {
	originalGetTags := getTags

	for i := 20; i <= 200; i += 20 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			baseTags := append([]string{hostTagPrefix + "foo", entityIDTagPrefix + "bar"}, buildTags(i/10)...)
			extraTags := buildTags(i / 2)
			taggerTags := buildTags(i)
			originTagsFunc := func() []string {
				return extraTags
			}
			getTags = func(entity string, cardinality collectors.TagCardinality) ([]string, error) {
				return taggerTags, nil
			}
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				tags, _ = enrichTags(baseTags, "hostname", originTagsFunc, false)
			}
		})
	}

	// Revert to original value
	getTags = originalGetTags
}
