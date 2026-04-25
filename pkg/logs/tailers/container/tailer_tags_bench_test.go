// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"fmt"
	"testing"
)

// BenchmarkBuildTags benchmarks the buildTags function used in buildMessage().
func BenchmarkBuildTags(b *testing.B) {
	testCases := []struct {
		name         string
		parsingTags  int
		providerTags int
	}{
		{"5_tags", 2, 3},
		{"12_tags", 4, 8},
		{"20_tags", 6, 14},
	}

	for _, tc := range testCases {
		parsingTags := generateTags("parsing", tc.parsingTags)
		providerTags := generateTags("provider", tc.providerTags)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = buildTags(parsingTags, providerTags)
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
