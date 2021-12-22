// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func buildTags(tagCount int) *tagset.Tags {
	bldr := tagset.NewBuilder(tagCount)
	for i := 0; i < tagCount; i++ {
		bldr.Add(fmt.Sprintf("tag%d:val%d", i, i))
	}

	return bldr.Close()
}

// used to store the result and avoid optimizations
var tags *tagset.Tags

func BenchmarkExtractTagsMetadata(b *testing.B) {
	for i := 20; i <= 200; i += 20 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			baseTags := tagset.Union(tagset.NewTags([]string{hostTagPrefix + "foo", entityIDTagPrefix + "bar"}), buildTags(i/10))
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				tags, _, _, _, _ = extractTagsMetadata(baseTags.UnsafeReadOnlySlice(), "hostname", "", false)
			}
		})
	}
}
