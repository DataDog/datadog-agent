// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func testBuilder() *Builder {
	bldr := NewBuilder(NewNullFactory(), 10)
	return bldr
}

func TestBuilder_Add_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("abc")
	bldr.Add("def")
	bldr.Add("abc")

	tags := bldr.Close()
	tags.validate(t)

	require.Equal(t, tags.Sorted(), []string{"abc", "def"})
}

func TestBuilder_AddKV_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.AddKV("host", "foo")
	bldr.AddKV("host", "bar")
	bldr.AddKV("host", "foo")

	tags := bldr.Close()
	tags.validate(t)

	require.Equal(t, tags.Sorted(), []string{"host:bar", "host:foo"})
}

func TestBuilder_AddTags_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("host:foo")
	bldr.AddTags(NewTags([]string{"abc", "def"}))

	tags := bldr.Close()
	tags.validate(t)

	require.Equal(t, tags.Sorted(), []string{"abc", "def", "host:foo"})
}

func TestBuilder_Contains(t *testing.T) {
	bldr := testBuilder()

	bldr.AddKV("host", "foo")
	bldr.AddKV("host", "bar")
	bldr.AddKV("host", "foo")

	require.True(t, bldr.Contains("host:foo"))
	require.False(t, bldr.Contains("host:bing"))
}

func ExampleBuilder() {
	shards := []int{1, 4, 19}

	bldr := NewBuilder(DefaultFactory, 5)
	for _, shard := range shards {
		bldr.AddKV("shard", strconv.Itoa(shard))
	}

	tags := bldr.Close()

	fmt.Printf("%s", tags.Sorted())
	// Output: [shard:1 shard:19 shard:4]
}

var benchmarkBuilderResult *Tags

/*
The original implementation of Builder used a map[uint64]struct{} to track seen
tags, by hash.  Its performance model, over a "reasonable" range of sizes
(0-100), was:

	Call:
	lm(formula = time ~ size + numDups, data = stats)

	Residuals:
		 Min       1Q   Median       3Q      Max
	-1611.27  -361.32     6.17   443.31   833.40

	Coefficients:
				Estimate Std. Error t value Pr(>|t|)
	(Intercept) 1729.905    137.837   12.55   <2e-16 ***
	size         143.315      3.670   39.05   <2e-16 ***
	numDups      -96.641      6.278  -15.39   <2e-16 ***
	---
	Signif. codes:  0 ‘***’ 0.001 ‘**’ 0.01 ‘*’ 0.05 ‘.’ 0.1 ‘ ’ 1

Which is to say, about 1.7µs plus 143ns per tag, with some time savings from
duplicate tags (likely saved by having a smaller result, and performing fewer
inserts).

The updated implementation, which simply performs a linear search in the existing
hashes, has the following model:

	Call:
	lm(formula = time ~ size + numDups, data = stats)

	Residuals:
		Min      1Q  Median      3Q     Max
	-706.36  -81.03   14.57  121.98  333.56

	Coefficients:
				Estimate Std. Error t value Pr(>|t|)
	(Intercept) 1290.343     57.796  22.326  < 2e-16 ***
	size          53.199      1.453  36.624  < 2e-16 ***
	numDups      -16.658      2.482  -6.713 5.33e-08 ***
	---
	Signif. codes:  0 ‘***’ 0.001 ‘**’ 0.01 ‘*’ 0.05 ‘.’ 0.1 ‘ ’ 1

Which is to say, about 1.3µs plus 53ns per tag, with less savings for duplicate
tags. So, universally faster.  Same-paramter benchmark comparisons bear this
out:

	Builder/size=10/numDupes=0-16   3.14µs ± 0%  1.69µs ± 0%  -46.10%
	Builder/size=20/numDupes=0-16   4.77µs ± 0%  2.23µs ± 0%  -53.27%
	Builder/size=20/numDupes=10-16  3.62µs ± 0%  2.22µs ± 0%  -38.78%
	Builder/size=30/numDupes=0-16   6.83µs ± 0%  2.91µs ± 0%  -57.41%
	Builder/size=40/numDupes=0-16   7.52µs ± 0%  3.30µs ± 0%  -56.10%
	Builder/size=40/numDupes=20-16  5.85µs ± 0%  3.22µs ± 0%  -44.98%
	Builder/size=50/numDupes=0-16   8.45µs ± 0%  3.68µs ± 0%  -56.46%
	Builder/size=50/numDupes=25-16  6.50µs ± 0%  3.50µs ± 0%  -46.09%
	Builder/size=60/numDupes=0-16   10.8µs ± 0%   4.4µs ± 0%  -59.64%
	Builder/size=60/numDupes=30-16  8.26µs ± 0%  3.99µs ± 0%  -51.74%
	Builder/size=70/numDupes=0-16   11.6µs ± 0%   4.9µs ± 0%  -57.59%
	Builder/size=70/numDupes=35-16  8.99µs ± 0%  4.57µs ± 0%  -49.13%
	Builder/size=80/numDupes=0-16   12.4µs ± 0%   5.8µs ± 0%  -53.16%
	Builder/size=80/numDupes=40-16  9.94µs ± 0%  4.87µs ± 0%  -51.03%
	Builder/size=90/numDupes=0-16   13.0µs ± 0%   6.0µs ± 0%  -53.75%
	Builder/size=90/numDupes=45-16  9.90µs ± 0%  5.48µs ± 0%  -44.61%

*/

func BenchmarkBuilder(b *testing.B) {
	bench := func(size, numDupes int) func(*testing.B) {
		return func(b *testing.B) {
			f := NewNullFactory()
			bldr := NewBuilder(f, size)
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				rng := rand.New(rand.NewSource(int64(i)))
				tags := make([]string, 0, size)

				for i := 0; i < size-numDupes; i++ {
					t := fmt.Sprintf("tag%d", i)
					tags = append(tags, t)
				}

				// remainder are dupes of the existing tags
				for i := size - numDupes; i < size; i++ {
					t := tags[rng.Intn(len(tags))]
					tags = append(tags, t)
				}

				rng.Shuffle(len(tags), func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })

				bldr.Reset(size)
				b.StartTimer()

				for _, t := range tags {
					bldr.Add(t)
				}
				benchmarkBuilderResult = bldr.Close()
			}
		}
	}

	for size := 10; size < 100; size += 10 {
		for numDupes := 0; numDupes < size; numDupes += size >> 1 {
			b.Run(fmt.Sprintf("size=%d/numDupes=%d", size, numDupes), bench(size, numDupes))
		}
	}
}
