// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
)

type ProviderTestSuite struct {
	suite.Suite
	p *provider
	a auditor.Component
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.a = auditorMock.NewAuditor()
	suite.p = &provider{
		numberOfPipelines:    3,
		auditor:              suite.a,
		pipelines:            []*Pipeline{},
		endpoints:            config.NewEndpoints(config.Endpoint{}, nil, true, false),
		currentPipelineIndex: atomic.NewUint32(0),
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
