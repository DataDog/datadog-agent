// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func nextFloat64(x float64, n int) float64 {
	direction := math.Inf(n)
	if n < 0 {
		n *= -1
	}

	for i := 0; i < n; i++ {
		x = math.Nextafter(x, direction)
	}
	return x
}

func TestULPDistance(t *testing.T) {
	next := nextFloat64
	require.EqualValues(t, next(0, 1), math.SmallestNonzeroFloat64)
	require.EqualValues(t, next(0, -1), -math.SmallestNonzeroFloat64)
	now := time.Now().UnixNano()
	t.Log(float64(now), now, nextFloat64(float64(now), 1)-float64(now))
	for _, tc := range []struct {
		a, b float64
		exp  uint64
	}{
		{math.NaN(), math.NaN(), math.MaxUint64},
		{math.NaN(), 1, math.MaxUint64},
		{1, math.NaN(), math.MaxUint64},
		{math.Inf(1), math.Inf(-1), math.MaxUint64},
		{0, 0, 0},
		{next(0, -1), next(0, 1), 2},
		{math.MaxFloat64, math.MaxFloat64 - 1, 0},
		{128, 128.0000001, 3518437},
	} {
		act := ulpDistance(tc.a, tc.b)

		if tc.exp != act {
			t.Fatalf("ulpDistance(%g,%g) != %d: actual=%d", tc.a, tc.b, tc.exp, act)
		}

	}
}

func TestFloat64Equal(t *testing.T) {
	next := nextFloat64
	type testCase struct {
		a, b float64
	}

	t.Run("Debug", func(t *testing.T) {
		// make sure ulpLimit isn't too high
		for v := float64(1); v < float64(math.MaxFloat32); v *= 10 {
			var (
				vLimit = next(v, ulpLimit)

				// vLimit is alway > v
				absDelta = vLimit - v
				relDelta = absDelta / vLimit
			)

			// arbitrary sanity check to make sure we keep precision
			if relDelta > 1e-12 {
				t.Fatal("ulpLimit too high", v, absDelta, relDelta)
				return
			}
		}
	})

	t.Run("Ok", func(t *testing.T) {
		for _, tc := range []testCase{
			{0, 0},
			{1, next(1, ulpLimit)},
			{next(0, ulpLimit-ulpLimit/2), next(0, ulpLimit/2)},
		} {
			if err := checkFloat64Equal("", tc.a, tc.b); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("Err", func(t *testing.T) {
		h1 := ulpLimit / 2
		h2 := ulpLimit - h1
		require.Equal(t, h1+h2, ulpLimit)

		for _, tc := range []testCase{
			{math.NaN(), math.NaN()},
			{1, next(1, ulpLimit+1)},
			// {next(0, -16), next(0, 1)},
		} {
			err := checkFloat64Equal("", tc.a, tc.b)
			if err == nil {
				t.Fatal("expected error", tc)
			}
		}
	})

}

func TestSummaryEqual(t *testing.T) {
	get := func(add ...Summary) Summary {
		s := Summary{
			Min: 1,
			Max: 2,
			Sum: 3,
			Avg: 4,
			Cnt: 5,
		}

		for _, a := range add {
			s.Min += a.Min
			s.Max += a.Max
			s.Sum += a.Sum
			s.Avg += a.Avg
			s.Cnt += a.Cnt
		}

		return s
	}

	type testCase struct {
		a, b  Summary
		equal bool
	}

	for _, tc := range []testCase{
		{get(Summary{}), get(), true},
		{get(Summary{Min: 1}), get(), false},
		{get(Summary{Cnt: 1}), get(), false},
		{get(Summary{Max: 1}), get(), false},
		{get(Summary{Sum: 1}), get(), false},
		{get(Summary{Avg: 1}), get(), false},
	} {
		err := CheckEqual(tc.a, tc.b)
		if tc.equal {
			if err != nil {
				t.Fatal(tc, err)
			}
		} else {
			if err == nil {
				t.Fatal(tc, "expected an error")
			}
		}

	}

}
