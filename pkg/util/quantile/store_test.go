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

// heldObservations returns, for each key, the observations a store holds: its
// units scaled back, plus its remainder.
func heldObservations(s *sparseStore) map[Key]uint64 {
	held := map[Key]uint64{}
	for _, b := range s.bins {
		held[b.k] += uint64(b.n) << s.shift()
	}
	for _, r := range s.pending() {
		held[r.k] += r.n
	}
	return held
}

// requireSameStore checks that two stores hold the same bins, at the same scale,
// with the same remainders.
func requireSameStore(t *testing.T, want, got *sparseStore) {
	t.Helper()
	require.Equal(t, want.shift(), got.shift(), "shift")
	require.Equal(t, want.count, got.count, "count")
	require.Equal(t, want.bins, got.bins, "bins")
	require.True(t, slices.Equal(want.pending(), got.pending()),
		"remainders: want %v, got %v", want.pending(), got.pending())
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

		assert.Nil(t, s.scaled)
		assert.Equal(t, n, s.count)
		assert.Equal(t, n, s.bins.nSum())
		assert.Len(t, s.bins, c.binLimit)
	})

	t.Run("past MaxCount", func(t *testing.T) {
		n := c.MaxCount() + 1
		s := &sparseStore{}
		s.insertCounts(c, []KeyCount{{k: 42, n: uint(n)}})

		// Halved, with the odd observation kept aside.
		assert.Equal(t, uint(1), s.shift())
		assert.Equal(t, n/2, s.count)
		assert.Equal(t, s.count, s.bins.nSum())
		assert.Equal(t, []remainder{{k: 42, n: 1}}, s.pending())
		assert.LessOrEqual(t, len(s.bins), c.binLimit)
	})
}

// TestInsertCountsScaledKeepsObservations checks that scaling loses no
// observation: each key holds its observations divided by 1<<shift, and the rest
// in its remainder.
func TestInsertCountsScaledKeepsObservations(t *testing.T) {
	c := Default()

	var kcs []KeyCount
	want := map[Key]uint64{}
	for i := 1; i <= 1000; i++ {
		// Uneven counts, so that keys keep different remainders. They stay
		// within a 32-bit uint, but add up to ~2.4e12: a shift of 14.
		n := 2_200_000_000 + uint64(i*i%9973)*40_000 + uint64(i)
		kcs = append(kcs, KeyCount{k: Key(i), n: uint(n)})
		want[Key(i)] = n
	}

	s := &sparseStore{}
	s.insertCounts(c, kcs)

	require.Equal(t, uint(14), s.shift())
	require.Equal(t, s.count, s.bins.nSum())
	// Otherwise trimLeft moves observations to higher keys, scaled or not.
	require.LessOrEqual(t, len(s.bins), c.binLimit, "the bins must not be trimmed")
	require.Equal(t, want, heldObservations(s))
}

// TestInsertCountsBatching checks that a scaled store ends up the same whether
// counts come in many small inserts or in a single one, rescaling included: the
// observations short of a unit carry over from an insert to the next.
func TestInsertCountsBatching(t *testing.T) {
	const (
		// Halved 4 times on its own, 5 times with a second one.
		big     = 3_000_000_000
		inserts = 1000
	)
	c := Default()

	repeated := &sparseStore{}
	repeated.insertCounts(c, []KeyCount{{k: 1, n: big}})
	require.Equal(t, uint(4), repeated.shift())
	for i := 0; i < inserts; i++ {
		// Short of half a unit, just past it, and a few units and a bit.
		repeated.insertCounts(c, []KeyCount{{k: 2, n: 7}, {k: 3, n: 9}, {k: 4, n: 50}})
	}
	repeated.insertCounts(c, []KeyCount{{k: 5, n: big}})

	batched := &sparseStore{}
	batched.insertCounts(c, []KeyCount{
		{k: 1, n: big}, {k: 2, n: 7 * inserts}, {k: 3, n: 9 * inserts}, {k: 4, n: 50 * inserts}, {k: 5, n: big},
	})

	require.Equal(t, uint(5), batched.shift())
	requireSameStore(t, batched, repeated)
	require.Equal(t, map[Key]uint64{1: big, 2: 7 * inserts, 3: 9 * inserts, 4: 50 * inserts, 5: big},
		heldObservations(repeated))
}

// TestInsertCountsFoldsRemainders checks that a store keeps remainders for at
// most binLimit keys, folding the lowest ones into higher keys without losing any
// observation.
func TestInsertCountsFoldsRemainders(t *testing.T) {
	c, err := NewConfig(0, 0, 4)
	require.NoError(t, err)

	s := &sparseStore{}
	// 1e7 is 156250 units of 64 observations, with no remainder.
	s.insertCounts(c, []KeyCount{{k: 100, n: 10_000_000}})
	require.Equal(t, uint(6), s.shift())

	var total uint64 = 10_000_000
	for k := Key(1); k <= 20; k++ {
		s.insertCounts(c, []KeyCount{{k: k, n: 10}})
		total += 10
		require.LessOrEqual(t, len(s.pending()), c.binLimit)
	}

	var held uint64
	for _, n := range heldObservations(s) {
		held += n
	}
	require.Equal(t, total, held)
	require.Equal(t, s.count, s.bins.nSum())
	require.LessOrEqual(t, len(s.bins), binBudget(c))
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
	require.Equal(t, map[Key]uint64{1: math.MaxUint, 2: math.MaxUint, 3: math.MaxUint}, heldObservations(s))
}

// TestInsertPastMaxCount checks that the keys pushing a store past
// Config.MaxCount(), and the ones inserted once it is scaled, are scaled too.
func TestInsertPastMaxCount(t *testing.T) {
	c := Default()
	s := &sparseStore{}
	s.insertCounts(c, []KeyCount{{k: 1, n: uint(c.MaxCount())}})
	require.Nil(t, s.scaled)

	keys := make([]Key, 511)
	for i := range keys {
		keys[i] = 2
	}
	s.insert(c, slices.Clone(keys))

	require.Equal(t, uint(1), s.shift())
	require.Equal(t, s.count, s.bins.nSum())
	require.LessOrEqual(t, len(s.bins), binBudget(c))
	_, units := keyUnits(s.bins)
	require.Equal(t, []uint64{uint64(c.MaxCount()) / 2, 255}, units)
	require.Equal(t, []remainder{{k: 2, n: 1}}, s.pending())

	s.insert(c, keys)

	require.Equal(t, uint(1), s.shift())
	require.Equal(t, s.count, s.bins.nSum())
	_, units = keyUnits(s.bins)
	require.Equal(t, []uint64{uint64(c.MaxCount()) / 2, 511}, units)
	require.Empty(t, s.pending())
}

// TestMergeScaled checks that merge brings both stores to a common shift, enough
// for the observations of both, without losing any or mutating the merged store.
func TestMergeScaled(t *testing.T) {
	c := Default()

	for _, tt := range []struct {
		name      string
		s, o      uint
		wantShift uint
	}{
		// Halved 4 and 2 times: o is rescaled to s.
		{name: "o to s", s: 3e9 + 5, o: 1e9 + 3, wantShift: 4},
		// Same as above, the other way around.
		{name: "s to o", s: 1e9 + 3, o: 3e9 + 5, wantShift: 4},
		// Halved 3 times each: both need rescaling.
		{name: "both", s: 2e9 + 7, o: 2e9 + 7, wantShift: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, o := &sparseStore{}, &sparseStore{}
			s.insertCounts(c, []KeyCount{{k: 1, n: tt.s}})
			o.insertCounts(c, []KeyCount{{k: 2, n: tt.o}})
			oBefore := sparseStore{bins: slices.Clone(o.bins), count: o.count, scaled: o.scaled.clone()}

			s.merge(c, o)

			require.Equal(t, oBefore, *o, "o must not be mutated")
			require.Equal(t, tt.wantShift, s.shift())
			require.Equal(t, s.count, s.bins.nSum())
			require.LessOrEqual(t, len(s.bins), binBudget(c))
			require.Equal(t, map[Key]uint64{1: uint64(tt.s), 2: uint64(tt.o)}, heldObservations(s))
		})
	}
}

func TestColsScaled(t *testing.T) {
	s := buildStore(t, "0:1 1:max")

	s.scaled = &scaling{shift: 3}
	_, n := s.Cols()
	assert.Equal(t, []uint32{8, math.MaxUint16 * 8}, n)

	// Past maxColsShift, the counts still fit a uint32.
	s.scaled = &scaling{shift: 40}
	_, n = s.Cols()
	assert.Equal(t, []uint32{1 << 16, math.MaxUint16 << 16}, n)
}
