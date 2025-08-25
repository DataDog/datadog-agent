// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func TestSummary(t *testing.T) {

	var (
		cases = []struct {
			v   float64
			exp Summary
		}{
			{
				v: -1,
				exp: Summary{
					Min: -1,
					Max: -1,
					Sum: -1,
					Avg: -1,
					Cnt: 1,
				},
			},
			{
				v: -2,
				exp: Summary{
					Min: -2,
					Max: -1,
					Sum: -3,
					Avg: -1.5,
					Cnt: 2,
				},
			},
			{
				v: 0,
				exp: Summary{
					Min: -2,
					Max: 0,
					Sum: -3,
					Avg: -1,
					Cnt: 3,
				},
			},
		}
	)

	t.Run("InsertN", func(t *testing.T) {
		var (
			s, exp = Summary{}, Summary{}
		)

		for i := 0; i < 20; i++ {
			v := math.Floor(rand.Float64() * 10000)
			nInsert := rand.Intn(1000) + 1

			s.InsertN(v, float64(nInsert))

			for j := 0; j < nInsert; j++ {
				exp.Insert(v)
			}
		}

		if err := CheckEqual(s, exp); err != nil {
			t.Fatal(err)
		}

	})

	t.Run("String", func(t *testing.T) {
		s := &Summary{
			Min: 1,
			Max: 2,
			Sum: 3,
			Avg: 4,
			Cnt: 5,
		}

		require.Equal(t, "min=1.0000 max=2.0000 avg=4.0000 sum=3.0000 cnt=5", s.String())
	})

	t.Run("Reset", func(t *testing.T) {
		s := &Summary{
			Min: 1,
			Max: 2,
			Sum: 3,
			Avg: 4,
			Cnt: 5,
		}
		s.Reset()
		require.Equal(t, Summary{}, *s)
	})

	t.Run("Observe", func(t *testing.T) {
		var (
			seen []float64
			s    = &Summary{}
		)
		for _, tc := range cases {
			seen = append(seen, tc.v)
			s.Insert(tc.v)
			require.Equal(t, tc.exp, *s, "seen=%v", seen)
		}
	})

	t.Run("Merge", func(t *testing.T) {
		var (
			seen []float64
			s    = &Summary{}
		)
		for _, tc := range cases {
			o := &Summary{}
			o.Insert(tc.v)
			seen = append(seen, tc.v)
			s.Merge(*o)
			require.Equal(t, tc.exp, *s, "seen=%v", seen)
		}
	})

	t.Run("Merge/Empty", func(t *testing.T) {
		s := &Summary{
			Min: 1,
			Max: 2,
			Sum: 3,
			Avg: 4,
			Cnt: 5,
		}
		s.Merge(Summary{})

		require.Equal(t, Summary{
			Min: 1,
			Max: 2,
			Sum: 3,
			Avg: 4,
			Cnt: 5,
		}, *s)
	})

	t.Run("Quick", func(t *testing.T) {

		f := func(in []float64) bool {
			var (
				os, ms = &Summary{}, &Summary{}
				seen   []float64

				add = func(v float64) {
					seen = append(seen, v)

					// observe v using os
					os.Insert(v)

					// merge into ms
					other := &Summary{}
					other.Insert(v)
					ms.Merge(*other)
				}
			)

			getexp := func(seen []float64) Summary {
				var s Summary
				if len(seen) == 0 {
					return s
				}

				s.Min, s.Max = seen[0], seen[0]

				for _, v := range seen {
					if v > s.Max {
						s.Max = v
					}

					if v < s.Min {
						s.Min = v
					}

					s.Cnt++
					s.Sum += v
				}

				s.Avg = s.Sum / float64(s.Cnt)
				return s
			}

			check := func(name string, a, e Summary) {

				if err := CheckEqual(a, e); err != nil {
					t.Fatal(name, err)
				}
			}

			for _, v := range in {
				add(v)
				expected := getexp(seen)
				check("observe", *os, expected)
				check("merge", *ms, expected)

			}

			return true
		}

		maxCount := 1000
		if testing.Short() {
			maxCount = 10
		}

		if err := quick.Check(f, &quick.Config{
			MaxCount: maxCount,
			Values: func(v []reflect.Value, r *rand.Rand) {
				// Use smaller values to avoid overflow.
				// TODO: test behavior around overflows.
				out := make([]float64, r.Intn(1000))
				for i := range out {
					out[i] = r.ExpFloat64()
				}

				v[0] = reflect.ValueOf(out)
			},
			// Values:
		}); err != nil {
			t.Fatal(err)
		}
	})

}
