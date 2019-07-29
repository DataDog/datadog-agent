// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package quantile

import (
	"encoding/json"
	"math/rand"
	"testing"
)

const randlen = 1000

func randSlice(n int) []float64 {
	// use those
	vals := make([]float64, 0, randlen)
	for i := 0; i < n; i++ {
		vals = append(vals, rand.Float64()*100000)
	}

	return vals
}

func BenchmarkGKSliceInsertion(b *testing.B) {
	s := NewSliceSummary()

	vals := randSlice(randlen)

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		s.Insert(vals[n%randlen], uint64(n))
	}
}

func BenchmarkGKSliceInsertionPreallocd(b *testing.B) {
	s := NewSliceSummary()
	s.Entries = make([]Entry, 0, 100)

	vals := randSlice(randlen)

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		s.Insert(vals[n%randlen], uint64(n))
	}
}

func BGKSliceQuantiles(b *testing.B, n int) {
	s := NewSliceSummary()
	vals := randSlice(n)
	for i := 0; i < n; i++ {
		s.Insert(vals[i], uint64(i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		s.Quantile(rand.Float64())
	}
}
func BenchmarkGKSliceQuantiles10(b *testing.B) {
	BGKSliceQuantiles(b, 10)
}
func BenchmarkGKSliceQuantiles100(b *testing.B) {
	BGKSliceQuantiles(b, 100)
}
func BenchmarkGKSliceQuantiles1000(b *testing.B) {
	BGKSliceQuantiles(b, 1000)
}
func BenchmarkGKSliceQuantiles10000(b *testing.B) {
	BGKSliceQuantiles(b, 10000)
}
func BenchmarkGKSliceQuantiles100000(b *testing.B) {
	BGKSliceQuantiles(b, 100000)
}

func BGKSliceEncoding(b *testing.B, n int) {
	s := NewSliceSummary()
	vals := randSlice(n)
	for i := 0; i < n; i++ {
		s.Insert(vals[i], uint64(i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		blob, _ := json.Marshal(&s)
		var ss SliceSummary
		json.Unmarshal(blob, &ss)
	}
}
func BenchmarkGKSliceEncoding10(b *testing.B) {
	BGKSliceEncoding(b, 10)
}
func BenchmarkGKSliceEncoding100(b *testing.B) {
	BGKSliceEncoding(b, 100)
}

// not worth encoding larger as we're constant in mem
func BenchmarkGKSliceEncoding1000(b *testing.B) {
	BGKSliceEncoding(b, 1000)
}
