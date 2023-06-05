// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"
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

func BenchmarkExtractTagsMetadata(b *testing.B) {
	conf := enrichConfig{
		defaultHostname: "hostname",
	}
	for i := 20; i <= 200; i += 20 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			baseTags := append([]string{hostTagPrefix + "foo", entityIDTagPrefix + "bar"}, buildTags(i/10)...)
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				tags, _, _, _, _ = extractTagsMetadata(baseTags, "", []byte{}, conf)
			}
		})
	}
}
