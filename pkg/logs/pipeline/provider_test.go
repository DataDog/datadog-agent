// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/suite"
)

type ProviderTestSuite struct {
	suite.Suite
	p *provider
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.p = &provider{
		numberOfPipelines: 3,
		chanSizes:         10,
		pipelinesChans:    [](chan message.Message){},
		currentChanIdx:    0,
	}
}

func (suite *ProviderTestSuite) TestProvider() {
	suite.p.Start(nil, nil)
	suite.Equal(3, len(suite.p.pipelinesChans))

	c := suite.p.NextPipelineChan()
	suite.Equal(int32(1), suite.p.currentChanIdx)
	suite.p.NextPipelineChan()
	suite.Equal(int32(2), suite.p.currentChanIdx)
	suite.p.NextPipelineChan()
	suite.Equal(c, suite.p.NextPipelineChan())
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
