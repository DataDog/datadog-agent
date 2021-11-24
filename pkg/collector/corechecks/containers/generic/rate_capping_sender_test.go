// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type rateCappingSuite struct {
	suite.Suite
	mockSender   *mocksender.MockSender
	cappedSender *CappedSender
}

// Artificially add 10 seconds to the sender timestamp
func (s *rateCappingSuite) tick() {
	s.cappedSender.timestamp = s.cappedSender.timestamp.Add(10 * time.Second)
}

// Put configuration back in a known state before each test
func (s *rateCappingSuite) SetupTest() {
	s.mockSender = mocksender.NewMockSender("rateTest")
	s.mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	s.cappedSender = &CappedSender{
		Sender:    s.mockSender,
		timestamp: time.Now(),
		rateCaps: map[string]float64{
			"capped.at.100": 100,
		},
	}
	cache.Cache.Flush() // Remove previous values from gocache
}

func (s *rateCappingSuite) TestUnfilteredMetric() {
	// Unfiltered metric
	s.cappedSender.Rate("non.capped", 200, "", nil)
	s.tick()
	s.cappedSender.Rate("non.capped", 2000, "", nil)
	s.tick()
	s.cappedSender.Rate("non.capped", 20000, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 3)
}

func (s *rateCappingSuite) TestUnderCap() {
	// Filtered rate under the cap is transmitted
	s.cappedSender.Rate("capped.at.100", 2000, "", nil)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 2900, "", nil)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 3800, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 3)
}

func (s *rateCappingSuite) TestOverCap() {
	// Updates over the rate are ignored
	s.cappedSender.Rate("capped.at.100", 2000, "", nil)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 3100, "", nil)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 4200, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 1)
}

func (s *rateCappingSuite) TestCapRecover() {
	// Transmit, cap then transmit
	s.cappedSender.Rate("capped.at.100", 2000, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 1)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 2500, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 2)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 4000, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 2)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 4500, "", nil)
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 3)
}

func (s *rateCappingSuite) TestTagging() {
	// Transmit both series, storing two cache entries
	s.cappedSender.Rate("capped.at.100", 200, "", []string{"first"})
	s.cappedSender.Rate("capped.at.100", 5000, "", []string{"two"})
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 2)
	s.tick()
	s.cappedSender.Rate("capped.at.100", 300, "", []string{"first"})
	s.cappedSender.Rate("capped.at.100", 5500, "", []string{"two"})
	s.mockSender.AssertNumberOfCalls(s.T(), "Rate", 4)
}

func TestDockerRateCappingSuite(t *testing.T) {
	suite.Run(t, &rateCappingSuite{})
}
