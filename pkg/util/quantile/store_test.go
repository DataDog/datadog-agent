// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"slices"
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

	eachParsedToken(t, dsl, 16, func(k Key, n uint64) {
		if n > maxBinWidth {
			t.Fatal("n > max", n, maxBinWidth)
		}

		s.count += int(n)
		s.bins = append(s.bins, bin{k: k, n: uint16(n)})
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
		}{
			{},
			{s: "1:1", e: "1:1"},
			{s: "1:1", e: "1:1", b: 1},
			{
				// TODO: if the trimmed size is the same as before trimming adds error
				// with no benefit.
				s: "1:max 2:max 3:max",
				e: "2:max 2:max 3:max",
				b: 2,
			},
			{
				s: "1:max 1:max 1:1 2:max 3:1 4:1",
				e: "1:65535 1:65535 2:1 2:65535 3:1 4:1",
				b: 3,
			},
			{
				s: "1:max-1 2:max-1 3:1",
				e: "1:max-1 2:max-1 3:1",
				b: 3,
			},
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
				s.bins = trimLeft(s.bins, tt.b)

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
			n:     []uint32{1, 1, 2, 1, 1, 1, 1, 1, math.MaxUint16},
		},
		{
			store: "0:1 0:max",
			k:     []int32{0, 0},
			n:     []uint32{1, math.MaxUint16},
		},
	} {
		st := buildStore(t, tt.store)
		k, n := st.Cols()
		assert.Equal(t, k, tt.k, "keys don't match")
		assert.Equal(t, n, tt.n, "values don't match")
	}
}

// binBudget is the most bins a store may hold: binLimit of them, plus the full
// ones trimLeft cannot fold, which unitCapacity keeps to binLimit+1.
func binBudget(c *Config) int {
	return 2*c.binLimit + 1
}

// keyUnits returns the units the bins of a store hold for each key, in key order.
func keyUnits(bins []bin) (keys []Key, units []uint64) {
	for _, b := range bins {
		if len(keys) == 0 || keys[len(keys)-1] != b.k {
			keys = append(keys, b.k)
			units = append(units, 0)
		}
		units[len(units)-1] += uint64(b.n)
	}
	return keys, units
}

// TestInsertCountsAboveMaxCount checks that the bins count up to Config.MaxCount()
// observations exactly, and scale down past it rather than outgrow the binLimit
// budget.
func TestInsertCountsAboveMaxCount(t *testing.T) {
	c := Default()
	require.Equal(t, defaultBinLimit*math.MaxUint16, c.MaxCount())

	t.Run("at MaxCount", func(t *testing.T) {
		n := c.MaxCount()
		s := &sparseStore{}
		s.insertCounts(c, []KeyCount{{k: 42, n: uint(n)}})

		assert.Zero(t, s.shift)
		assert.Equal(t, n, s.count)
		assert.Equal(t, n, s.bins.nSum())
		assert.Len(t, s.bins, c.binLimit)
	})

	t.Run("past MaxCount", func(t *testing.T) {
		n := c.MaxCount() + 1
		s := &sparseStore{}
		s.insertCounts(c, []KeyCount{{k: 42, n: uint(n)}})

		// Halved, rounding to the nearest unit.
		assert.Equal(t, uint(1), s.shift)
		assert.Equal(t, (n+1)/2, s.count)
		assert.Equal(t, s.count, s.bins.nSum())
		assert.LessOrEqual(t, len(s.bins), c.binLimit)
	})
}

// TestInsertCountsScaledRanks checks that scaling keeps the share of the
// observations up to each key, which quantiles are made of, within a unit.
func TestInsertCountsScaledRanks(t *testing.T) {
	c := Default()

	var (
		kcs   []KeyCount
		total uint64
	)
	for i := 1; i <= 1000; i++ {
		// Uneven counts, so that units hold different remainders. They stay
		// within a 32-bit uint, but add up to ~2.4e12: a shift of 14.
		n := 2_200_000_000 + uint64(i*i%9973)*40_000
		kcs = append(kcs, KeyCount{k: Key(i), n: uint(n)})
		total += n
	}

	s := &sparseStore{}
	s.insertCounts(c, slices.Clone(kcs))

	require.Equal(t, uint(14), s.shift)
	require.Equal(t, s.count, s.bins.nSum())
	// Otherwise trimLeft moves observations to higher keys, scaled or not.
	require.LessOrEqual(t, len(s.bins), c.binLimit, "the bins must not be trimmed")

	keys, units := keyUnits(s.bins)
	require.Len(t, keys, len(kcs))

	var exact, scaled uint64
	for i, kc := range kcs {
		require.Equal(t, kc.k, keys[i])
		exact += uint64(kc.n)
		scaled += units[i]
		require.InDelta(t, float64(exact)/float64(total), float64(scaled)/float64(s.count),
			1/float64(s.count), "share of the observations up to key %d", kc.k)
	}
}

// TestInsertCountsExtremeCounts checks that counts adding up past a uint64 do
// not overflow.
func TestInsertCountsExtremeCounts(t *testing.T) {
	c := Default()
	s := &sparseStore{}
	s.insertCounts(c, []KeyCount{{k: 1, n: math.MaxUint}, {k: 2, n: math.MaxUint}, {k: 3, n: math.MaxUint}})

	require.Equal(t, s.count, s.bins.nSum())
	require.LessOrEqual(t, uint64(s.count), uint64(c.MaxCount()))
	require.LessOrEqual(t, len(s.bins), binBudget(c))

	keys, units := keyUnits(s.bins)
	require.Equal(t, []Key{1, 2, 3}, keys)
	for _, u := range units {
		require.InDelta(t, 1.0/3, float64(u)/float64(s.count), 1e-6)
	}
}

// TestInsertPastMaxCount checks that the keys pushing a store past
// Config.MaxCount(), and the ones inserted once it is scaled, are scaled too.
func TestInsertPastMaxCount(t *testing.T) {
	c := Default()
	s := &sparseStore{}
	s.insertCounts(c, []KeyCount{{k: 1, n: uint(c.MaxCount())}})
	require.Zero(t, s.shift)

	keys := make([]Key, 512)
	for i := range keys {
		keys[i] = 2
	}
	s.insert(c, slices.Clone(keys))

	require.Equal(t, uint(1), s.shift)
	require.Equal(t, s.count, s.bins.nSum())
	require.LessOrEqual(t, len(s.bins), binBudget(c))
	_, units := keyUnits(s.bins)
	require.Equal(t, []uint64{uint64(c.MaxCount()) / 2, 256}, units)

	s.insert(c, keys)

	require.Equal(t, uint(1), s.shift)
	require.Equal(t, s.count, s.bins.nSum())
	_, units = keyUnits(s.bins)
	require.Equal(t, []uint64{uint64(c.MaxCount()) / 2, 512}, units)
}

// TestMergeScaled checks that merge brings both stores to a common shift, enough
// for the observations of both, without mutating the merged one.
func TestMergeScaled(t *testing.T) {
	c := Default()

	for _, tt := range []struct {
		name      string
		s, o      uint
		wantShift uint
	}{
		// 3e9/2^4 and 1e9/2^2 units: o is rescaled to s.
		{name: "o to s", s: 3e9, o: 1e9, wantShift: 4},
		// Same as above, the other way around.
		{name: "s to o", s: 1e9, o: 3e9, wantShift: 4},
		// 2e9/2^3 units each: both need rescaling.
		{name: "both", s: 2e9, o: 2e9, wantShift: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, o := &sparseStore{}, &sparseStore{}
			s.insertCounts(c, []KeyCount{{k: 1, n: tt.s}})
			o.insertCounts(c, []KeyCount{{k: 2, n: tt.o}})
			oBefore := sparseStore{bins: slices.Clone(o.bins), count: o.count, shift: o.shift}

			s.merge(c, o)

			require.Equal(t, oBefore, *o, "o must not be mutated")
			require.Equal(t, tt.wantShift, s.shift)
			require.Equal(t, s.count, s.bins.nSum())
			require.LessOrEqual(t, len(s.bins), binBudget(c))

			keys, units := keyUnits(s.bins)
			require.Equal(t, []Key{1, 2}, keys)
			require.InDelta(t, float64(tt.s)/float64(tt.s+tt.o), float64(units[0])/float64(s.count), 1e-6)
		})
	}
}

func TestColsScaled(t *testing.T) {
	s := buildStore(t, "0:1 1:max")

	s.shift = 3
	_, n := s.Cols()
	assert.Equal(t, []uint32{8, math.MaxUint16 * 8}, n)

	// Past maxColsShift, the counts still fit a uint32.
	s.shift = 40
	_, n = s.Cols()
	assert.Equal(t, []uint32{1 << 16, math.MaxUint16 << 16}, n)
}
