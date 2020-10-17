// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import (
	"math"
)

// AdjustScoring modifies sampler coefficients to fit better the `maxTPS` condition
func (s *Sampler) AdjustScoring() {
	currentTPS := s.Backend.GetSampledScore()
	totalTPS := s.Backend.GetTotalScore()
	offset := s.signatureScoreOffset.Load()
	cardinality := float64(s.Backend.GetCardinality())

	newOffset, newSlope := adjustCoefficients(currentTPS, totalTPS, s.maxTPS, offset, cardinality)

	s.SetSignatureCoefficients(newOffset, newSlope)
}

func adjustCoefficients(currentTPS, totalTPS, maxTPS, offset, cardinality float64) (newOffset, newSlope float64) {
	// See how far we are from our maxTPS limit and make signature sampler harder/softer accordingly
	TPSratio := currentTPS / maxTPS

	// Compute how much we should change the offset
	coefficient := 1.0

	if TPSratio > 1 {
		// If above, reduce the offset
		coefficient = 0.8
		// If we keep 3x too many traces, reduce the offset even more
		if TPSratio > 3 {
			coefficient = 0.5
		}
	} else if TPSratio < 0.8 {
		// If below, increase the offset
		// Don't do it if:
		//  - we already keep all traces (with a 1% margin because of stats imprecision)
		//  - offset above maxTPS
		if currentTPS < 0.99*totalTPS && offset < maxTPS {
			coefficient = 1.1
			if TPSratio < 0.5 {
				coefficient = 1.3
			}
		}
	}

	newOffset = coefficient * offset

	// Safeguard to avoid too small offset (for guaranteed very-low volume sampling)
	if newOffset < minSignatureScoreOffset {
		newOffset = minSignatureScoreOffset
	}

	// Default slope value
	newSlope = defaultSignatureScoreSlope

	// Compute the slope based on the signature count distribution
	// TODO: explain this formula
	if offset < totalTPS {
		newSlope = math.Pow(10, math.Log10(cardinality*totalTPS/maxTPS)/math.Log10(totalTPS/minSignatureScoreOffset))
		// That's the max value we should allow. When slope == 10, we basically keep only `offset` traces per signature
		if newSlope > 10 {
			newSlope = 10
		}
	}

	return newOffset, newSlope
}
