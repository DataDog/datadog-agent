// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"net"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	pipeline "github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

const port = 10493

type ConnectionHandlerTestSuite struct {
	suite.Suite

	h       *ConnectionHandler
	msgChan chan message.Message
}

func (suite *ConnectionHandlerTestSuite) SetupTest() {
	pp := pipeline.NewMockProvider()
	source := config.NewLogSource("", &config.LogsConfig{Type: config.TCPType, Port: port})
	suite.h = NewConnectionHandler(pp, source)
	suite.msgChan = pp.NextPipelineChan()
}

func (suite *ConnectionHandlerTestSuite) TestHandleConnection() {
	r, w := net.Pipe()
	suite.h.HandleConnection(r)

	var msg message.Message

	// should receive and decode one message
	w.Write([]byte("foo\n"))
	msg = <-suite.msgChan
	suite.Equal("foo", string(msg.Content()))

	// should receive and decode two messages
	w.Write([]byte("bar\nboo\n"))
	msg = <-suite.msgChan
	suite.Equal("bar", string(msg.Content()))
	msg = <-suite.msgChan
	suite.Equal("boo", string(msg.Content()))
}

func (suite *ConnectionHandlerTestSuite) TestLifeCyle() {
	r1, w1 := net.Pipe()
	suite.h.HandleConnection(r1)

	r2, w2 := net.Pipe()
	suite.h.HandleConnection(r2)

	// stop should not be blocking
	go w1.Close()
	w2.Close()
	suite.h.Stop()
}

func TestConnectionHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(ConnectionHandlerTestSuite))
}
