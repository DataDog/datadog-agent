// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	"fmt"
	"testing"
)

func benchmarkDeduplicateTags(numberOfTags int, b *testing.B) {
	tags := make([]string, 0, numberOfTags+1)
	for i := 0; i < numberOfTags/2; i++ {
		tags = append(tags, fmt.Sprintf("aveeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeerylong:tag%d", i))
	}
	for i := 0; i < numberOfTags/2; i++ {
		tags = append(tags, fmt.Sprintf("aveeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeerylong:tag%d", i))
	}

	tempTags := make([]string, len(tags))
	copy(tempTags, tags)
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		deduplicateTags(tempTags)
	}
}
func BenchmarkDeduplicateTags2(b *testing.B)    { benchmarkDeduplicateTags(2, b) }
func BenchmarkDeduplicateTags4(b *testing.B)    { benchmarkDeduplicateTags(4, b) }
func BenchmarkDeduplicateTags10(b *testing.B)   { benchmarkDeduplicateTags(10, b) }
func BenchmarkDeduplicateTags20(b *testing.B)   { benchmarkDeduplicateTags(20, b) }
func BenchmarkDeduplicateTags40(b *testing.B)   { benchmarkDeduplicateTags(40, b) }
func BenchmarkDeduplicateTags60(b *testing.B)   { benchmarkDeduplicateTags(60, b) }
func BenchmarkDeduplicateTags100(b *testing.B)  { benchmarkDeduplicateTags(100, b) }
func BenchmarkDeduplicateTags1000(b *testing.B) { benchmarkDeduplicateTags(1000, b) }
