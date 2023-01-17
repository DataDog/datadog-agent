// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/quantile/summary"

	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	var (
		c      = Default()
		values []float64

		s1          = &Sketch{}
		s2          = &Sketch{}
		sInsert     = &Sketch{}
		sInsertMany = &Sketch{}
	)

	for i := -50; i <= 50; i++ {
		v := float64(i)
		sInsert.Insert(c, v)

		values = append(values, v)
		if i&1 == 0 {
			s1.Insert(c, v)
		} else {
			s2.Insert(c, v)
		}
	}

	sInsertMany.InsertMany(c, values)
	s1.Merge(c, s2)
	require.Len(t, sInsert.bins, len(values))
	for _, s := range []*Sketch{s1, sInsertMany, sInsert} {
		require.EqualValues(t, 0, s.Quantile(c, .50))
		require.EqualValues(t, sInsert.Quantile(c, .99), -sInsert.Quantile(c, .01))
		for p := 0; p < 50; p++ {
			q := float64(p) / 100
			pos, neg := s.Quantile(c, .50+q), -s.Quantile(c, .50-q)

			if math.Abs(pos-neg) > 1e-6 {
				t.Errorf("Quantile(%g) = %g Quantile(%g) = %g", .50+q, pos, .50-q, neg)
			}

		}
		require.EqualValues(t, sInsert.bins, s.bins) // make sure they all have the same bins
	}

}

func TestString(t *testing.T) {
	var (
		s, c    = &Sketch{}, Default()
		nvalues = 5
	)

	for i := 0; i < nvalues; i++ {
		s.Insert(c, float64(i))
	}

	// TODO: more in depth tests, we just make sure this doesn't panic.
	t.Log(s.String())
}

func TestReset(t *testing.T) {
	var (
		s, c      = &Sketch{}, Default()
		checkBins = func(nbins int) {
			t.Helper()
			switch {
			case s.bins.Len() != nbins:
				t.Fatalf("s.bins.Len() != nbins. got:%d, want:%d", s.bins.Len(), nbins)
			case s.count != nbins:
				t.Fatalf("s.count != nbins. got:%d, want:%d", s.count, nbins)
			case int(s.Basic.Cnt) != nbins:
				t.Fatalf("s.Basic.Cnt != nbins. got:%d, want:%d", s.Basic.Cnt, nbins)
			}
		}
	)

	// setup: insert values until we have 10 bins
	nbins := 10
	for i := 0; i < nbins; i++ {
		v := math.Pow(c.gamma.v, float64(i))
		s.Insert(c, v)
	}
	checkBins(nbins)

	// reset
	s.Reset()
	checkBins(0)

	empty := summary.Summary{}
	if s.Basic != empty {
		t.Fatalf("%s should be empty", s.Basic.String())
	}

}
func TestQuantile(t *testing.T) {
	var (
		c = Default()

		// create a sketch with nbins bins.
		create = func(nbins int) *Sketch {
			t.Helper()

			s := &Sketch{}
			k := c.key(1)
			for i := 0; i < nbins; i++ {
				v := c.f64(k)
				if c.key(v) != k {
					t.Errorf("key(f64(%d)) != k. got: %d, want: %d", k, c.key(v), k)
				}

				s.Insert(c, v)
				k++
			}

			for _, b := range s.bins {
				if b.n != 1 {
					t.Log(c.f64(b.k))
					t.Fatalf("s should only have a single item per bin: %v\n%s", b, s)
				}
			}

			if s.bins.Len() != nbins {
				t.Fatalf("s.bins.Len() != nbins. got:%d, want:%d\n%s",
					s.bins.Len(), nbins, s)
			}

			return s
		}
	)

	type qtest struct {
		s   *Sketch
		q   float64
		exp float64
	}

	sIncr101 := arange(t, c, 101)
	sBin101 := create(101)

	for i, tt := range []qtest{
		{sBin101, .99, c.f64(sBin101.bins[99].k)},
		{sIncr101, 0, 0},
		{sIncr101, .01, c.f64(c.key(1))},
		{sIncr101, .98, c.f64(c.key(98))},
	} {

		t.Run(fmt.Sprintf("#%d/q=%g/exp=%g", i, tt.q, tt.exp), func(t *testing.T) {
			v := tt.s.Quantile(c, tt.q)

			if v != tt.exp {
				t.Log(tt.s)
				t.Fatalf("Quantile(%g) wrong. got:(k=%d, v=%g) want:(k=%d, v=%g)", tt.q,
					c.key(v), v,
					c.key(tt.exp), tt.exp,
				)
			}
		})

	}
}

func TestRank(t *testing.T) {
	t.Run("101", func(t *testing.T) {
		// when cnt=101:
		//  rank(0)   = 0
		//  rank(.50) = 50
		//  rank(.99) = 99
		//  ...
		cnt := 101

		for p := float64(0); p <= 100; p++ {
			q := p / 100.0

			if r := rank(cnt, q); r != p {
				t.Errorf("rank(%d, %g) wrong. got:%g want:%g", cnt, q, r, p)
			}
		}
	})
}
