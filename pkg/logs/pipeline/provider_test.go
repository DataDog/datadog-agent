// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

type ProviderTestSuite struct {
	suite.Suite
	p          *provider
	a          *auditor.RegistryAuditor
	fakeIntake net.Listener
}

func newTestSender(suite *ProviderTestSuite) *sender.Sender {
	input := make(chan *message.Payload, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	destination := tcp.AddrToDestination(suite.fakeIntake.Addr(), destinationsCtx, statusinterface.NewStatusProviderMock())
	destinations := client.NewDestinations([]client.Destination{destination}, nil)

	cfg := configmock.New(suite.T())
	sender := sender.NewSender(cfg, input, auditor.NewNullAuditor(), destinations, 0, nil, nil, metrics.NewNoopPipelineMonitor(""))
	return sender
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.fakeIntake = mock.NewMockLogsIntake(suite.T())
	suite.a = auditor.New("", auditor.DefaultRegistryFilename, time.Hour, health.RegisterLiveness("fake"))
	sender := newTestSender(suite)
	suite.p = &provider{
		numberOfPipelines:    3,
		auditor:              suite.a,
		pipelines:            []*Pipeline{},
		endpoints:            config.NewEndpoints(config.Endpoint{}, nil, true, false),
		currentPipelineIndex: atomic.NewUint32(0),
		sender:               sender,
		compression:          compressionfx.NewMockCompressor(),
	}
}

func (suite *ProviderTestSuite) TestProvider() {
	suite.a.Start()
	suite.p.Start()
	suite.Equal(uint32(0), suite.p.currentPipelineIndex.Load())
	suite.Equal(3, len(suite.p.pipelines))

	c := suite.p.NextPipelineChan()
	suite.Equal(uint32(1), suite.p.currentPipelineIndex.Load())
	suite.Equal(suite.p.pipelines[1].InputChan, c)

	c = suite.p.NextPipelineChan()
	suite.Equal(uint32(2), suite.p.currentPipelineIndex.Load())
	suite.Equal(suite.p.pipelines[2].InputChan, c)

	c = suite.p.NextPipelineChan()
	suite.Equal(uint32(3), suite.p.currentPipelineIndex.Load())
	suite.Equal(suite.p.pipelines[0].InputChan, c) // wraps

	c = suite.p.NextPipelineChan()
	suite.Equal(uint32(4), suite.p.currentPipelineIndex.Load())
	suite.Equal(suite.p.pipelines[1].InputChan, c)

	suite.p.Stop()
	suite.a.Stop()
	suite.Nil(suite.p.NextPipelineChan())
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
