// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestCappedSender(t *testing.T) {
	mockSender := aggregator.NewMockSender("rateTest")
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	cappedSender := &cappedSender{
		Sender:             mockSender,
		previousRateValues: make(map[string]float64),
		rateCaps: map[string]float64{
			"capped.at.100": 100,
		},
	}

	// Unfiltered metric
	cappedSender.Rate("non.capped", 200, "", nil)
	cappedSender.Rate("non.capped", 2000, "", nil)
	cappedSender.Rate("non.capped", 20000, "", nil)
	mockSender.AssertNumberOfCalls(t, "Rate", 3)

	// Filtered rate under the cap is transmitted
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.100", 200, "", nil)
	cappedSender.Rate("capped.at.100", 250, "", nil)
	mockSender.AssertNumberOfCalls(t, "Rate", 2)

	// Updates over the rate are ignored
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.100", 1000, "", nil)
	cappedSender.Rate("capped.at.100", 2000, "", nil)
	mockSender.AssertNumberOfCalls(t, "Rate", 0)

	// Back under the cap, should be transmitted
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.100", 2050, "", nil)
	mockSender.AssertNumberOfCalls(t, "Rate", 1)
}
