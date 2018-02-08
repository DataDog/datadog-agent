// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ProviderTestSuite struct {
	suite.Suite
	p *provider
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.p = &provider{
		numberOfPipelines: 3,
		pipelines:         []*Pipeline{},
	}
}

func (suite *ProviderTestSuite) TestProvider() {
	suite.p.Start()
	suite.Equal(int32(0), suite.p.currentPipelineIndex)
	suite.Equal(3, len(suite.p.pipelines))

	c := suite.p.NextPipelineChan()
	suite.Equal(int32(1), suite.p.currentPipelineIndex)

	suite.p.NextPipelineChan()
	suite.Equal(int32(2), suite.p.currentPipelineIndex)

	suite.p.NextPipelineChan()
	suite.Equal(int32(0), suite.p.currentPipelineIndex)
	suite.Equal(c, suite.p.NextPipelineChan())
	suite.Equal(int32(1), suite.p.currentPipelineIndex)

	suite.p.Stop()
	suite.Nil(suite.p.NextPipelineChan())
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
