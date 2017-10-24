// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestCappedSender(t *testing.T) {
	mockSender := aggregator.NewMockSender("rateTest")
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	cappedSender := &cappedSender{
		Sender:             mockSender,
		previousRateValues: make(map[string]float64),
		previousTimes:      make(map[string]time.Time),
		timestamp:          time.Now(),
		rateCaps: map[string]float64{
			"capped.at.10": 10,
		},
	}

	// Unfiltered metric
	cappedSender.Rate("non.capped", 200, "", nil)
	tick(cappedSender)
	cappedSender.Rate("non.capped", 2000, "", nil)
	tick(cappedSender)
	cappedSender.Rate("non.capped", 20000, "", nil)
	tick(cappedSender)
	mockSender.AssertNumberOfCalls(t, "Rate", 3)

	// Filtered rate under the cap is transmitted
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.10", 200, "", nil)
	tick(cappedSender)
	cappedSender.Rate("capped.at.10", 250, "", nil)
	tick(cappedSender)
	mockSender.AssertNumberOfCalls(t, "Rate", 2)

	// Updates over the rate are ignored
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.10", 2000, "", nil)
	tick(cappedSender)
	cappedSender.Rate("capped.at.10", 3000, "", nil)
	tick(cappedSender)
	mockSender.AssertNumberOfCalls(t, "Rate", 0)

	// Back under the cap, should be transmitted
	mockSender.ResetCalls()
	cappedSender.Rate("capped.at.10", 3050, "", nil)
	tick(cappedSender)
	mockSender.AssertNumberOfCalls(t, "Rate", 1)
}

// Artificially add 10 seconds to the sender timestamp
func tick(sender *cappedSender) {
	sender.timestamp = sender.timestamp.Add(10 * time.Second)
}
