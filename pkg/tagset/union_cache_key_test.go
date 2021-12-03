package tagset

import (
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
