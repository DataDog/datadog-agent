// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ProviderTestSuite struct {
	suite.Suite
	pp *Provider
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.pp = NewProvider()
}

func (suite *ProviderTestSuite) TestProvider() {
	suite.pp.numberOfPipelines = 3
	suite.pp.Start(nil, nil)
	suite.Equal(3, len(suite.pp.pipelinesChans))

	c := suite.pp.NextPipelineChan()
	suite.Equal(int32(1), suite.pp.currentChanIdx)
	suite.pp.NextPipelineChan()
	suite.Equal(int32(2), suite.pp.currentChanIdx)
	suite.pp.NextPipelineChan()
	suite.Equal(c, suite.pp.NextPipelineChan())
}

func (suite *ProviderTestSuite) TestProviderMock() {
	suite.pp.MockPipelineChans()
	suite.Equal(1, len(suite.pp.pipelinesChans))
	suite.Equal(int32(1), suite.pp.numberOfPipelines)
	suite.Equal(suite.pp.NextPipelineChan(), suite.pp.NextPipelineChan())
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
