// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// Validate that unionCacheKey works, and that for at least one scenario it
// meets its requirements.
func TestUnionCacheKey(t *testing.T) {
	r := rand.New(rand.NewSource(100))
	a := r.Uint64()
	b := r.Uint64()
	tt := r.Uint64()

	require.NotEqual(t, unionCacheKey(a, b), unionCacheKey(a, b^tt))
	require.NotEqual(t, unionCacheKey(a, b), unionCacheKey(a^tt, b))
	require.NotEqual(t, unionCacheKey(a, b), unionCacheKey(a^tt, b^tt))
	require.NotEqual(t, unionCacheKey(a, b^tt), unionCacheKey(a^tt, b))
	require.NotEqual(t, unionCacheKey(a, b^tt), unionCacheKey(a^tt, b^tt))
	require.NotEqual(t, unionCacheKey(a^tt, b), unionCacheKey(a^tt, b^tt))
}

// global to avoid optimization of the benchmark call
var unionCacheResult uint64

func BenchmarkUnionCacheKey(b *testing.B) {
	r := rand.New(rand.NewSource(100))
	aHash := r.Uint64()
	bHash := r.Uint64()
	aIncr := r.Uint64()
	bIncr := r.Uint64()

	for i := 0; i < b.N; i++ {
		unionCacheResult = unionCacheKey(aHash, bHash)
		aHash += aIncr
		bHash += bIncr
	}
}

func TestUnioncCacheKey_Fuzz(t *testing.T) {
	type pair struct {
		a, b uint64
	}
	seen := map[uint64]pair{}
	fuzz(t, func(seed int64) {
		r := rand.New(rand.NewSource(seed))
		for i := 0; i < 10000; i++ {
			p := pair{
				a: r.Uint64(),
				b: r.Uint64(),
			}
			k := unionCacheKey(p.a, p.b)
			if existing, found := seen[k]; found {
				require.Equal(t, existing, p)
			} else {
				seen[k] = p
			}
		}
	})
}

var benchmarkUnionResult *Tags

func BenchmarkUnion(b *testing.B) {
	bench := func(size int) func(*testing.B) {
		return func(b *testing.B) {
			b.StopTimer()
			f, _ := NewCachingFactory(100, 1)
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				rng := rand.New(rand.NewSource(int64(i)))

				n := size // exactly of the given size
				lBuilder := f.NewBuilder(n)
				for i := 0; i < n; i++ {
					t := fmt.Sprintf("tag%d", i)
					lBuilder.Add(t)
				}
				l := lBuilder.Close()
				require.Equal(b, size, l.Len())

				n = size + rng.Intn(size/2) // somewhat larger than given size
				rBuilder := f.NewBuilder(n)
				for i := 0; i < n; i++ {
					// 50% of the tags are in l
					t := fmt.Sprintf("tag%d", rng.Intn(2*size))
					rBuilder.Add(t)
				}
				r := rBuilder.Close()

				b.StartTimer()

				benchmarkUnionResult = union(l, r)
			}
		}
	}

	for size := 20; size < 110; size += 10 {
		b.Run(fmt.Sprintf("size=%d", size), bench(size))
	}
}
