// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/mock"
	"github.com/stretchr/testify/suite"
)

const tcpTestPort = 10512

type TCPTestSuite struct {
	suite.Suite

	outputChan chan message.Message
	pp         pipeline.Provider
	source     *config.IntegrationConfigLogSource
	tcpl       *TCPListener
}

func (suite *TCPTestSuite) SetupTest() {
	suite.pp = mock.NewMockProvider()
	suite.outputChan = suite.pp.NextPipelineChan()
	suite.source = &config.IntegrationConfigLogSource{Type: config.TCPType, Port: tcpTestPort, Tracker: status.NewTracker()}
	tcpl, err := NewTCPListener(suite.pp, suite.source)
	suite.Nil(err)
	suite.tcpl = tcpl
	suite.tcpl.Start()
}

func (suite *TCPTestSuite) TestTCPReceivesMessages() {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", tcpTestPort))
	suite.Nil(err)
	fmt.Fprintf(conn, "hello world\n")
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
}

func (suite *TCPTestSuite) TestLifeCycle() {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", tcpTestPort))
	suite.Nil(err)

	// conn should be still alive and message should be received
	_, err = conn.Write([]byte("hello world\n"))
	suite.Nil(err)
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))

	suite.tcpl.Stop()

	// conn should be stopped
	inBuf := make([]byte, 1024)
	_, err = conn.Read(inBuf)
	suite.Equal(io.EOF, err)

	// tcp connection should be refused
	_, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", tcpTestPort))
	suite.NotNil(err)
}

func TestTCPTestSuite(t *testing.T) {
	suite.Run(t, new(TCPTestSuite))
}
