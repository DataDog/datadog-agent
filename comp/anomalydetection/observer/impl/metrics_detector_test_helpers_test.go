// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math/rand"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

type manualSeriesRemover interface {
	RemoveSeries([]observer.SeriesRef)
}

func genGaussian(rng *rand.Rand, n int, mean, stddev float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = mean + stddev*rng.NormFloat64()
	}
	return out
}

func genBimodal(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		if rng.Float64() < 0.5 {
			out[i] = -3 + rng.NormFloat64()
		} else {
			out[i] = 3 + rng.NormFloat64()
		}
	}
	return out
}
