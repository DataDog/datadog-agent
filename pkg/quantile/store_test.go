// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// buildStore creates a store with the bins defined by a simple dsl:
//   <key>:<n> <key>:<n> ...
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
