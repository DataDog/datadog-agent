// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"testing"
)

// BenchmarkBuildTags benchmarks the buildTags function used in forwardMessages().
// Real-world data shows 8-28 tags per log at ~42 logs/second throughput.
func BenchmarkBuildTags(b *testing.B) {
	testCases := []struct {
		name         string
		baseTags     int
		providerTags int
		parsingTags  int
	}{
		{"8_tags", 2, 4, 2},
		{"15_tags", 3, 8, 4},
		{"28_tags", 4, 18, 6},
	}

	for _, tc := range testCases {
		baseTags := generateTags("base", tc.baseTags)
		providerTags := generateTags("provider", tc.providerTags)
		parsingTags := generateTags("parsing", tc.parsingTags)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = buildTags(baseTags, providerTags, parsingTags)
			}
		})
	}
}

func generateTags(prefix string, count int) []string {
	tags := make([]string, count)
	for i := 0; i < count; i++ {
		tags[i] = fmt.Sprintf("%s_tag_%d:value_%d", prefix, i, i)
	}
	return tags
}
