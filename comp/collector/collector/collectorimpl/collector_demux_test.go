// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package collectorimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type CollectorDemuxTestSuite struct {
	suite.Suite

	demux demultiplexer.FakeSamplerMock
	c     *collectorImpl
}

func (suite *CollectorDemuxTestSuite) SetupTest() {
	suite.demux = fxutil.Test[demultiplexer.FakeSamplerMock](suite.T(), logimpl.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
	suite.c = newCollector(fxutil.Test[dependencies](suite.T(),
		core.MockBundle(),
		fx.Provide(func() sender.SenderManager {
			return suite.demux
		}),
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{"check_cancel_timeout": 500 * time.Millisecond},
		})))

	suite.c.start(context.TODO())
}

func (suite *CollectorDemuxTestSuite) TearDownTest() {
	suite.c.stop(context.TODO())
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
		flip:  flip,
		flop:  flop,
		demux: suite.demux,
	}

	sender, _ := suite.demux.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	suite.c.RunCheck(ch)

	// Wait for Check#Run to start before cancelling the check: otherwise it may not run at all.
	<-flip

	err := suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	flop <- struct{}{}

	suite.waitForCancelledCheckMetrics()

	newSender, err := suite.demux.GetSender(ch.ID())
	assert.Nil(suite.T(), err)
	assert.NotEqual(suite.T(), sender, newSender) // GetSedner returns a new instance, which means the old sender was destroyed correctly.
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

func (suite *CollectorDemuxTestSuite) TestCancelledCheckDestroysSender() {
	// Test that a check destroys a sender if it is not running when it is cancelled.

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip:  flip,
		flop:  flop,
		demux: suite.demux,
	}

	sender, _ := suite.demux.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	suite.c.RunCheck(ch)
	<-flip
	flop <- struct{}{}
	suite.c.checks[ch.ID()].Wait()
	err := suite.c.StopCheck(ch.ID())
	assert.NoError(suite.T(), err)

	suite.waitForCancelledCheckMetrics()

	newSender, err := suite.demux.GetSender(ch.ID())
	assert.Nil(suite.T(), err)
	assert.NotEqual(suite.T(), sender, newSender) // GetSender returns a new instance, which means the old sender was destroyed correctly.
}

func (suite *CollectorDemuxTestSuite) TestRescheduledCheckReusesSampler() {
	// When a check is cancelled and then scheduled again while the aggregator still holds on to sampler (because it contains unsent metrics)

	flip := make(chan struct{})
	flop := make(chan struct{})

	ch := &cancelledCheck{
		flip:  flip,
		flop:  flop,
		demux: suite.demux,
	}

	sender, err := suite.demux.GetSender(ch.ID())
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
		sender, _ := suite.demux.GetAgentDemultiplexer().PeekSender(ch.ID())
		return sender == nil
	}, time.Second, 10*time.Millisecond)

	// create new sender and try registering sampler before flush
	_, err = suite.demux.GetSender(ch.ID())
	assert.NoError(suite.T(), err)

	// flush
	suite.waitForCancelledCheckMetrics()

	sender, _ = suite.demux.GetSender(ch.ID())
	sender.DisableDefaultHostname(true)

	// Run the check again
	suite.c.RunCheck(ch)

	<-flip
	flop <- struct{}{}

	// flush again, check should contain metrics
	suite.waitForCancelledCheckMetrics()
}

func TestCollectorDemuxSuite(t *testing.T) {
	suite.Run(t, new(CollectorDemuxTestSuite))
}

type cancelledCheck struct {
	stub.StubCheck
	flip  chan struct{}
	flop  chan struct{}
	demux demultiplexer.FakeSamplerMock
}

func (c *cancelledCheck) Run() error {
	c.flip <- struct{}{}

	<-c.flop
	s, err := c.demux.GetSender(c.ID())
	if err != nil {
		return err
	}

	s.Gauge("test.metric", 1, "", []string{})
	s.Commit()

	return nil
}
