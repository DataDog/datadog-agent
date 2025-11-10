// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Run("config sanity", func(t *testing.T) {
		var b strings.Builder
		for binLimit := 512; binLimit <= 4096; binLimit *= 2 {
			c, err := NewConfig(0, 0, binLimit)
			if err != nil {
				t.Fatal(err)
			}

			d := ""
			if c.binLimit == defaultBinLimit {
				d = "(default)"
			}

			fmt.Fprintf(&b, "with binLimit=%d maxCount=%d %s\n", c.binLimit, c.MaxCount(), d)
		}

		t.Log("\n" + b.String())
		// arbitrary limits that I think are reasonable.
		c := Default()
		require.True(t, c.MaxCount() > 5e7)
		require.True(t, c.norm.max > math.MaxFloat32 && c.norm.max > math.MaxUint64)
	})

	t.Run("norm", func(t *testing.T) {
		c := Default()
		t.Logf("%+v\n", c)

		next := func(v, towards float64) float64 {
			return math.Nextafter(v, towards)
		}

		npos := func(v float64) float64 {
			return next(v, math.Inf(1))
		}

		nneg := func(v float64) float64 {
			return next(v, math.Inf(-1))
		}

		t.Run("key", func(t *testing.T) {
			for _, tt := range []struct {
				v float64

				ek Key // expected key
			}{
				// 0 <= v < c.norm.min
				{v: -0},
				{v: 0},
				{v: nneg(c.norm.min)},

				// c.norm.min >= v < c.norm.max
				{v: c.norm.min, ek: 1},
				{v: npos(c.norm.min), ek: 1},
				{v: c.norm.min * c.gamma.v, ek: 2},

				{v: c.norm.max, ek: maxKey},
			} {
				ak := c.key(tt.v)
				if tt.ek != ak {
					t.Fatalf("key(%g)=%d expected=%d", tt.v, ak, tt.ek)
				}
			}
		})

		t.Run("f64", func(t *testing.T) {
			for _, tt := range []struct {
				k  Key
				ev float64
			}{
				// 0 <= v < c.norm.min
				{k: InfKey(1), ev: math.Inf(1)},
				{k: InfKey(-1), ev: math.Inf(-1)},
				{k: 2, ev: c.f64(2)},
			} {
				av := c.f64(tt.k)
				require.Equal(t, tt.ev, av)
			}
		})

		t.Run("key(f64(k)) == k", func(t *testing.T) {
			for k := Key(-maxKey); k <= maxKey; k++ {
				var (
					v  = c.f64(k)
					ak = c.key(v)
				)

				if k == ak {
					continue
				}

				knext, kprev := c.key(npos(v)), c.key(nneg(v))
				msg := fmt.Sprintf("k=%d f64(k)=%g key(f64(k))=%d knext=%d kprev=%d", k, v, ak, knext, kprev)

				// math.Log(v) isn't very accurate when v is near 1, but the key should change
				// somewhere between key(v - 1ulp) and key(v + 1ulp).
				if knext-kprev != 1 || math.Abs(v) > 2 || math.Abs(v) < .5 {
					t.Fatal(msg)
				}
			}

		})

	})

}

type score struct {
	n    int
	last struct {
		v, qv float64
	}
	relerr struct {
		sum, max float64
	}
}

func (s *score) print() {
	fmt.Printf("%g: qv=%g n=%d relative_err:(max=%g, avg=%g)\n",
		s.last.v, s.last.qv, s.n, s.relerr.max, s.relerr.sum/float64(s.n))
}

func (s *score) update(v, qv float64) {
	s.n++
	s.last.v, s.last.qv = v, qv
	abserr := math.Abs(v - qv)

	// update relative error
	relerr := abserr
	if maxv := math.Max(math.Abs(v), math.Abs(qv)); maxv > 0 {
		relerr /= maxv
	}

	s.relerr.sum += relerr
	if relerr > s.relerr.max {
		s.relerr.max = relerr
	}
}

func TestRelativeError(t *testing.T) {

	var (
		c                   = Default()
		posinf              = float32(math.Inf(1))
		start, stop float32 = 1e-6, math.MaxInt64
		isPow2              = func(v float64) bool {
			if v <= 1 {
				return false
			}
			f, _ := math.Frexp(v)
			return f == .5
		}
	)

	if testing.Short() {
		// this test takes a really really long time for the whole range of float32s
		start, stop = 1, float32(math.Pow(c.gamma.v, 5))
	}

	s := &score{}
	for v32 := start; v32 <= stop; v32 = math.Nextafter32(v32, posinf) {
		v := float64(v32)
		qv := c.f64(c.key(v))
		s.update(v, qv)
		if isPow2(v) {
			s.print()
		}

		if limit := .01; s.relerr.max > limit {
			t.Fatalf("max relative error is too high: %g > %g", s.relerr.max, limit)
		}
	}

	s.print()
}
