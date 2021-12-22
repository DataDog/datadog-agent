// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"fmt"
	"testing"
)

// TODO: remove this, as it's no longer useful
func BenchmarkExtractTagsMetadata(b *testing.B) {
	for i := 20; i <= 200; i += 20 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			hostTag := hostTagPrefix + "foo"
			entityIDTag := entityIDTagPrefix + "bar"
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				_, _, _, _ = extractTagsMetadata(hostTag, entityIDTag, "", "hostname", "", false)
			}
		})
	}
}
