// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildStore creates a store with the bins defined by a simple dsl:
//
//	<key>:<n> <key>:<n> ...
//
// For example, `0:3 1:1 2:1 2:1 3:max`
// TODO: move to main_test.go
func buildStore(t *testing.T, dsl string) *sparseStore {
	s := &sparseStore{}

	eachParsedToken(t, dsl, 32, func(k Key, n uint64) {
		if n > maxBinWidth {
			t.Fatal("n > max", n, uint64(maxBinWidth))
		}

		s.count += n
		s.bins = append(s.bins, bin{k: k, n: uint32(n)})
	})

	return s
}

func TestStore(t *testing.T) {
	t.Run("merge", func(t *testing.T) {
		type mt struct {
			s, o, exp string
			binLimit  int
		}

		for _, tt := range []mt{
			{s: "1:1", o: "", exp: "1:1"},
			{s: "", o: "1:1", exp: "1:1"},
			{s: "1:3", o: "1:2", exp: "1:5"},
			{s: "1:max-1", o: "1:max-2", exp: "1:max-3 1:max"},
			{s: "1:1 2:1 3:1", o: "5:1 6:1 10:1", exp: "1:1 2:1 3:1 5:1 6:1 10:1"},

			// binLimit
			{
				s:        "0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1 9:1 10:1",
				o:        "0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1 9:1",
				exp:      "8:18 9:2 10:1",
				binLimit: 3,
			},
		} {

			t.Run("", func(t *testing.T) {
				var (
					c   = Default()
					s   = buildStore(t, tt.s)
					o   = buildStore(t, tt.o)
					exp = buildStore(t, tt.exp)
				)

				if tt.binLimit != 0 {
					c.binLimit = tt.binLimit
				}

				// TODO|TEST: check that o is not mutated.
				s.merge(c, o)

				if exp.count != s.count {
					t.Errorf("s.count=%d, want %d", s.count, exp.count)
				}

				if nsum := s.bins.nSum(); exp.count != nsum {
					t.Errorf("nSum=%d, want %d", nsum, exp.count)
				}

				require.Equal(t, exp.bins.String(), s.bins.String())
				require.EqualValues(t, exp, s)
			})
		}

	})

	t.Run("trimLeft", func(t *testing.T) {
		for _, tt := range []struct {
			s, e string
			b    int
			// lost is what the lowest bin kept cannot count.
			lost uint64
		}{
			{},
			{s: "1:1", e: "1:1"},
			{s: "1:1", e: "1:1", b: 1},
			{s: "5:10 6:20", e: "5:10 6:20", b: 2},
			// The lowest bins collapse into the lowest bin kept.
			{s: "0:2 1:3 2:4 3:5", e: "2:9 3:5", b: 2},
			{s: "-3:5 -2:3 -1:2 0:1", e: "-1:10 0:1", b: 2},
			{s: "0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1 9:1", e: "6:7 7:1 8:1 9:1", b: 4},
			{s: "0:50000 1:50000 2:1", e: "2:100001", b: 1},
			// Up to what it can count.
			{s: "0:max 1:1", e: "1:max", b: 1, lost: 1},
			{s: "1:max 2:max 3:max", e: "2:max 3:max", b: 2, lost: maxBinWidth},
			{s: "1:max 1:max 1:1 2:max 3:1 4:1", e: "2:max 3:1 4:1", b: 3, lost: 2*maxBinWidth + 1},
			{s: "1:max-1 2:max-1 3:1", e: "1:max-1 2:max-1 3:1", b: 3},
		} {

			t.Run("", func(t *testing.T) {
				var (
					c   = Default()
					s   = buildStore(t, tt.s)
					exp = buildStore(t, tt.e)
				)

				if tt.b != 0 {
					c.binLimit = tt.b
				}
				bins, lost := trimLeft(s.bins, tt.b)

				require.Equal(t, exp.bins.String(), binList(bins).String())
				require.Equal(t, tt.lost, lost)
				require.Equal(t, s.count, binList(bins).nSum()+lost, "trimLeft keeps or reports every observation")
			})
		}

	})

	t.Run("insert", func(t *testing.T) {
		type insertTest struct {
			s    *sparseStore
			keys []Key
			exp  string
		}

		c := func(startState string, expected string, keys ...Key) insertTest {
			return insertTest{
				s:    buildStore(t, startState),
				keys: keys,
				exp:  expected,
			}
		}

		for _, tt := range []insertTest{
			c("",
				"0:3 1:1 2:1 5:1 9:1",
				0, 0, 0, 1, 2, 5, 9,
			),
			c("0:2", "-3:1 -2:1 -1:1 0:2", -1, -2, -3),
			c("0:2", "0:4", 0, 0),
			c("0:max", "0:1 0:max", 0),
			c("0:max 0:max", "0:1 0:max 0:max", 0),
			c("0:1 0:max 0:max", "0:3 0:max 0:max", 0, 0),
			c("1:1 3:1 4:1 5:1 6:1 7:1", "1:1 2:1 3:2 4:1 5:1 6:1 7:1", 2, 3),
			c("1:1 3:1", "1:1 2:3 3:1", 2, 2, 2),
			c("0:max-3", "0:2 0:max", make([]Key, 5)...),
		} {
			// TODO|TEST: that we never exceed binLimit.
			t.Run("", func(t *testing.T) {
				s := tt.s
				s.insert(Default(), tt.keys)

				exp := buildStore(t, tt.exp)
				if exp.count != s.count {
					t.Errorf("s.count=%d, want %d", s.count, exp.count)
				}

				if nsum := s.bins.nSum(); exp.count != nsum {
					t.Errorf("nSum=%d, want %d", nsum, exp.count)
				}

				require.Equal(t, exp.bins.String(), s.bins.String())
				require.Equal(t, exp.bins, s.bins)

			})

		}
	})

}

func TestCols(t *testing.T) {
	for _, tt := range []struct {
		store string
		k     []int32
		n     []uint32
	}{
		{
			store: "",
		},
		{
			store: "0:1 1:1 2:2 3:1 4:1 5:1 8:1 9:1 10:max",
			k:     []int32{0, 1, 2, 3, 4, 5, 8, 9, 10},
			n:     []uint32{1, 1, 2, 1, 1, 1, 1, 1, maxBinWidth},
		},
		{
			store: "0:1 0:max",
			k:     []int32{0, 0},
			n:     []uint32{1, maxBinWidth},
		},
	} {
		st := buildStore(t, tt.store)
		k, n := st.Cols()
		assert.Equal(t, k, tt.k, "keys don't match")
		assert.Equal(t, n, tt.n, "values don't match")
	}
}

// TestInsertCountsAboveMaxCount checks that a count above Config.MaxCount(),
// what binLimit uint16 bins could count, now fits in a single bin.
func TestInsertCountsAboveMaxCount(t *testing.T) {
	c := Default()
	require.Equal(t, defaultBinLimit*math.MaxUint16, c.MaxCount())

	n := uint64(c.MaxCount()) + 1
	s := &sparseStore{}
	s.insertCounts(c, []KeyCount{{k: 42, n: uint(n)}})

	assert.Equal(t, n, s.count)
	assert.Equal(t, n, s.bins.nSum())
	assert.Len(t, s.bins, 1)
}

// expandRuns lays out runs the way appendTrimmed must match: every bin
// appendSafe gives them, then trimLeft.
func expandRuns(runs []run, limit int) ([]bin, uint64) {
	var bins []bin
	for _, r := range runs {
		bins = appendSafe(bins, r.k, r.n)
	}
	return trimLeft(bins, limit)
}

// TestAppendTrimmed checks that appendTrimmed gives the bins trimLeft keeps out
// of what appendSafe would give, without laying out the others.
func TestAppendTrimmed(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	sizes := []uint64{0, 1, 2, maxBinWidth - 1, maxBinWidth, maxBinWidth + 1, 3*maxBinWidth + 5, 7 * maxBinWidth}

	for i := 0; i < 2000; i++ {
		var runs []run
		for k := Key(rng.IntN(5)); len(runs) < 1+rng.IntN(8); k += Key(rng.IntN(3)) {
			runs = append(runs, run{k: k, n: sizes[rng.IntN(len(sizes))]})
		}
		limit := 1 + rng.IntN(12)

		want, _ := expandRuns(runs, limit)
		got := appendTrimmed(nil, runs, limit)
		require.Equal(t, binList(want).String(), binList(got).String(), "runs %v, limit %d", runs, limit)
	}
}

// TestInsertCountsBoundedWhateverTheCount inserts counts that would take
// billions of bins, over many keys at once. The store must only allocate and
// keep the binLimit bins it can count.
func TestInsertCountsBoundedWhateverTheCount(t *testing.T) {
	if bits.UintSize < 64 {
		t.Skip("the counts do not fit in a 32-bit uint")
	}

	c := Default()
	kcs := make([]KeyCount, 5000)
	for i := range kcs {
		kcs[i] = KeyCount{k: Key(i + 1), n: math.MaxUint}
	}

	s := &sparseStore{}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	s.insertCounts(c, kcs)
	runtime.ReadMemStats(&after)

	require.LessOrEqual(t, after.TotalAlloc-before.TotalAlloc, uint64(1<<20), "bytes allocated by the insert")
	require.Len(t, s.bins, c.binLimit)
	require.Equal(t, s.bins.nSum(), s.count)
	// The bins kept are full ones of the highest key.
	for _, b := range s.bins {
		require.Equal(t, bin{k: 5000, n: maxBinWidth}, b)
	}
}
