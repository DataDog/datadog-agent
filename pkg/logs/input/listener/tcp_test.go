// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/stretchr/testify/suite"
)

const TCP_TEST_PORT = 10512

type TCPTestSuite struct {
	suite.Suite

	outputChan chan message.Message
	pp         *pipeline.PipelineProvider
	source     *config.IntegrationConfigLogSource
	tcpl       *AbstractNetworkListener
}

func (suite *TCPTestSuite) SetupTest() {
	suite.pp = pipeline.NewPipelineProvider()
	suite.pp.MockPipelineChans()
	suite.outputChan = suite.pp.NextPipelineChan()
	suite.source = &config.IntegrationConfigLogSource{Type: config.TCP_TYPE, Port: TCP_TEST_PORT}
	tcpl, err := NewTcpListener(suite.pp, suite.source)
	suite.Nil(err)
	suite.tcpl = tcpl
	suite.tcpl.Start()
}

func (suite *TCPTestSuite) TestTCPReceivesMessages() {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", TCP_TEST_PORT))
	suite.Nil(err)
	fmt.Fprintf(conn, "hello world\n")
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
}

func TestTCPTestSuite(t *testing.T) {
	suite.Run(t, new(TCPTestSuite))
}
