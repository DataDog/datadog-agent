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

	demux *aggregator.TestAgentDemultiplexer
	c     *Collector
}

func (suite *CollectorDemuxTestSuite) SetupTest() {
	suite.c = NewCollector()
	suite.demux = aggregator.InitTestAgentDemultiplexerWithFlushInterval(100 * time.Hour)

	suite.c.Start()
}

func (suite *CollectorDemuxTestSuite) TearDownTest() {
	suite.c.Stop()
	suite.demux.Stop(false)
	suite.c = nil
}

func (suite *CollectorDemuxTestSuite) TestCancelledCheckCanSendMetrics() {
	// Test that a longqq running check can send metrics using
	// correct sender after it was cancelled, and destroys a
	// sender at the end.

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip: flip,
		flop: flop,
	}
	var err error
	sender, err = aggregator.NewSender(ch.ID())
	require.NoError(suite.T(), err)

	suite.c.RunCheck(ch)

	// Wait for Check#Run to start before cancelling the check: otherwise it may not run at all.
	_ = <-flip

	err := suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	flop <- struct{}{}

	suite.waitForCancelledCheckMetrics()
}

func (suite *CollectorDemuxTestSuite) waitForCancelledCheckMetrics() {
	agg := suite.demux.Aggregator()
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

func TestCollectorDemuxSuite(t *testing.T) {
	suite.Run(t, new(CollectorDemuxTestSuite))
}

type cancelledCheck struct {
	check.StubCheck
	flip chan struct{}
	flop chan struct{}

	sender aggregator.Sender
}

func (c *cancelledCheck) Run() error {
	c.flip <- struct{}{}

	<-c.flop

	c.sender.Gauge("test.metric", 1, "", []string{})
	c.sender.Commit()

	return nil
}
