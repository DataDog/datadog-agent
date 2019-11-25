// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sampler

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func getTestSampler() *Sampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	extraRate := 1.0
	maxTPS := 0.0

	return newSampler(extraRate, maxTPS)
}

func TestSamplerAccessRace(t *testing.T) {
	// regression test: even though the sampler is channel protected, it
	// has getters accessing its fields.
	s := newSampler(1, 2)
	go func() {
		for i := 0; i < 10000; i++ {
			s.SetSignatureCoefficients(float64(i), float64(i)/2)
		}
	}()
	for i := 0; i < 5000; i++ {
		s.GetState()
		s.GetAllCountScores()
	}
}

func TestSamplerLoop(t *testing.T) {
	s := getTestSampler()

	exit := make(chan bool)

	go func() {
		s.Run()
		close(exit)
	}()

	s.Stop()

	select {
	case <-exit:
		return
	case <-time.After(time.Second * 1):
		assert.Fail(t, "Sampler took more than 1 second to close")
	}
}

func TestCombineRates(t *testing.T) {
	var combineRatesTests = []struct {
		rate1, rate2 float64
		expected     float64
	}{
		{0.1, 1.0, 1.0},
		{0.3, 0.2, 0.44},
		{0.0, 0.5, 0.5},
	}
	for _, tt := range combineRatesTests {
		assert.Equal(t, tt.expected, CombineRates(tt.rate1, tt.rate2))
		assert.Equal(t, tt.expected, CombineRates(tt.rate2, tt.rate1))
	}
}

func TestAddSampleRate(t *testing.T) {
	assert := assert.New(t)
	tID := randomTraceID()

	root := &pb.Span{TraceID: tID, SpanID: 1, ParentID: 0, Start: 123, Duration: 100000, Service: "mcnulty", Type: "web"}

	AddGlobalRate(root, 0.4)
	assert.Equal(0.4, root.Metrics["_sample_rate"], "sample rate should be 40%%")

	AddGlobalRate(root, 0.5)
	assert.Equal(0.2, root.Metrics["_sample_rate"], "sample rate should be 20%% (50%% of 40%%)")
}
