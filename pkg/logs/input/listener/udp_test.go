// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/mock"
	"github.com/stretchr/testify/suite"
)

const udpTestPort = 10513

type UDPTestSuite struct {
	suite.Suite

	outputChan chan message.Message
	pp         pipeline.Provider
	source     *config.IntegrationConfigLogSource
	udpl       *UDPListener
}

func (suite *UDPTestSuite) SetupTest() {
	suite.pp = mock.NewMockProvider()
	suite.outputChan = suite.pp.NextPipelineChan()
	suite.source = &config.IntegrationConfigLogSource{Type: config.UDPType, Port: udpTestPort, Tracker: status.NewTracker()}
	udpl, err := NewUDPListener(suite.pp, suite.source)
	suite.Nil(err)
	suite.udpl = udpl
	suite.udpl.Start()
}

func (suite *UDPTestSuite) TestUDPReceivesMessages() {
	conn, err := net.Dial("udp", fmt.Sprintf("localhost:%d", udpTestPort))
	suite.Nil(err)
	fmt.Fprintf(conn, "hello world\n")
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
}

func TestUDPTestSuite(t *testing.T) {
	suite.Run(t, new(UDPTestSuite))
}
