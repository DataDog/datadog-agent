// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package quantile

import (
	"math"
	"math/rand"
)

var randomFloats []float64

func init() {
	// generate a list of guaranteed random numbers for the probabilistic round
	randomFloats = make([]float64, 100)
	r := rand.New(rand.NewSource(7337))
	for i := 0; i < 100; i++ {
		randomFloats[i] = r.Float64()
	}
}

// WeightedSliceSummary associates a weight to a slice summary.
type WeightedSliceSummary struct {
	Weight float64
	*SliceSummary
}

func probabilisticRound(g int, weight float64, randFloat func() float64) int {
	raw := weight * float64(g)
	decimal := raw - math.Floor(raw)
	limit := randFloat()

	iraw := int(raw)
	if limit > decimal {
		iraw++
	}

	return iraw
}

// WeighSummary applies a weight factor to a slice summary and return it as a
// new slice.
func WeighSummary(s *SliceSummary, weight float64) *SliceSummary {
	// Deterministic random number generation based on a list because rand.Seed
	// is expensive to run
	i := 0
	randFloat := func() float64 {
		i++
		return randomFloats[i%len(randomFloats)]
	}

	sw := NewSliceSummary()
	sw.Entries = make([]Entry, 0, len(s.Entries))

	gsum := 0
	for _, e := range s.Entries {
		newg := probabilisticRound(e.G, weight, randFloat)
		// if an entry is down to 0 delete it
		if newg != 0 {
			sw.Entries = append(sw.Entries,
				Entry{V: e.V, G: newg, Delta: e.Delta},
			)
			gsum += newg
		}
	}

	sw.N = gsum
	return sw
}

// BySlicesWeighted BySlices() is the BySlices version but combines multiple
// weighted slice summaries before returning the histogram
func BySlicesWeighted(summaries ...WeightedSliceSummary) []SummarySlice {
	if len(summaries) == 0 {
		return []SummarySlice{}
	}

	mergeSummary := WeighSummary(summaries[0].SliceSummary, summaries[0].Weight)
	if len(summaries) > 1 {
		for _, s := range summaries[1:] {
			sw := WeighSummary(s.SliceSummary, s.Weight)
			mergeSummary.Merge(sw)
		}
	}

	return mergeSummary.BySlices()
}
