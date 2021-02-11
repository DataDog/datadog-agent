// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"testing"

	"github.com/cihub/seelog"
)

func getTestSampler() *Sampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	extraRate := 1.0
	targetTPS := 0.0

	return newSampler(extraRate, targetTPS, nil)
}

func TestSamplerAccessRace(t *testing.T) {
	// regression test: even though the sampler is channel protected, it
	// has getters accessing its fields.
	s := newSampler(1, 2, nil)
	go func() {
		for i := 0; i < 10000; i++ {
			s.SetSignatureCoefficients(float64(i), float64(i)/2)
		}
	}()
	for i := 0; i < 5000; i++ {
		s.GetAllCountScores()
	}
}
