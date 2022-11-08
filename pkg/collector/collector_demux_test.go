// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type CollectorDemuxTestSuite struct {
	suite.Suite
	c *Collector
}

func (suite *CollectorDemuxTestSuite) SetupTest() {
	suite.c = NewCollector()
	suite.c.Start()
}

func (suite *CollectorDemuxTestSuite) TearDownTest() {
	suite.c.Stop()
	suite.c = nil
}

func (suite *CollectorDemuxTestSuite) TestCancelledCheckCanSendMetrics() {
	// Test that a long running check can send metrics using
	// correct sender after it was cancelled, and destroys a
	// sender at the end.

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(100 * time.Hour)
	defer demux.Stop(false)

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip: flip,
		flop: flop,
	}

	sender, _ := aggregator.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	suite.c.RunCheck(ch)

	// Wait for Check#Run to start before cancelling the check: otherwise it may not run at all.
	_ = <-flip

	err := suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	flop <- struct{}{}

	suite.waitForCancelledCheckMetrics(demux.Aggregator())

	newSender, err := aggregator.GetSender(ch.ID())
	assert.Nil(suite.T(), err)
	assert.NotEqual(suite.T(), sender, newSender) // GetSedner returns a new instance, which means the old sender was destroyed correctly.
}

func (suite *CollectorDemuxTestSuite) waitForCancelledCheckMetrics(agg *aggregator.BufferedAggregator) {
	require.Eventually(suite.T(), func() bool {
		series, _ := agg.GetSeriesAndSketches(time.Time{})
		for _, serie := range series {
			if serie.Name == "test.metric" {
				assert.Empty(suite.T(), serie.Host)
				assert.Equal(suite.T(), serie.MType, metrics.APIGaugeType)
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func (suite *CollectorDemuxTestSuite) TestCancelledCheckDestroysSender() {
	// Test that a check destroys a sender if it is not running when it is cancelled.

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(100 * time.Hour)
	defer demux.Stop(false)

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip: flip,
		flop: flop,
	}

	sender, _ := aggregator.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	suite.c.RunCheck(ch)
	<-flip
	flop <- struct{}{}
	suite.c.checks[ch.ID()].Wait()
	err := suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	suite.waitForCancelledCheckMetrics(demux.Aggregator())

	newSender, err := aggregator.GetSender(ch.ID())
	assert.Nil(suite.T(), err)
	assert.NotEqual(suite.T(), sender, newSender) // GetSedner returns a new instance, which means the old sender was destroyed correctly.
}

func (suite *CollectorDemuxTestSuite) TestRescheduledCheckReusesSampler() {
	// When a check is cancelled and then scheduled again while the aggregator still holds on to sampler (because it contains unsent metrics)

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(100 * time.Hour)
	defer demux.Stop(false)

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip: flip,
		flop: flop,
	}

	sender, err := aggregator.GetSender(ch.ID())
	assert.NoError(suite.T(), err)
	sender.DisableDefaultHostname(true)

	suite.c.RunCheck(ch)

	<-flip
	flop <- struct{}{}

	err = suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	// Wait for the check to drop the sender
	require.Eventually(suite.T(), func() bool {
		// returns error if sender was not found, which is what we are waiting for
		sender, _ := demux.PeekSender(ch.ID())
		return sender == nil
	}, time.Second, 10*time.Millisecond)

	//create new sender and try registering sampler before flush
	sender, err = aggregator.GetSender(ch.ID())
	assert.NoError(suite.T(), err)

	// flush
	suite.waitForCancelledCheckMetrics(demux.Aggregator())

	sender, _ = aggregator.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	// Run the check again
	suite.c.RunCheck(ch)

	<-flip
	flop <- struct{}{}

	// flush again, check should contain metrics
	suite.waitForCancelledCheckMetrics(demux.Aggregator())
}

func TestCollectorSuite(t *testing.T) {
	suite.Run(t, new(CollectorDemuxTestSuite))
}

type cancelledCheck struct {
	check.StubCheck
	flip chan struct{}
	flop chan struct{}
}

func (c *cancelledCheck) Run() error {
	c.flip <- struct{}{}

	<-c.flop
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	s.Gauge("test.metric", 1, "", []string{})
	s.Commit()

	return nil
}
