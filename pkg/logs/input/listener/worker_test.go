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
)

const port = 10493

type WorkerTestSuite struct {
	suite.Suite

	w       *Worker
	conn    net.Conn
	msgChan chan message.Message
}

func (suite *WorkerTestSuite) SetupTest() {
	source := config.NewLogSource("", &config.LogsConfig{Type: config.TCPType, Port: port})
	msgChan := make(chan message.Message)
	r, w := net.Pipe()

	suite.w = NewWorker(source, r, msgChan)
	suite.conn = w
	suite.msgChan = msgChan

	suite.w.Start()
}

func (suite *WorkerTestSuite) TearDownTest() {
	suite.w.Stop()
}

func (suite *WorkerTestSuite) TestReadAndForward() {
	var msg message.Message

	// should receive and decode one message
	suite.conn.Write([]byte("foo\n"))
	msg = <-suite.msgChan
	suite.Equal("foo", string(msg.Content()))

	// should receive and decode two messages
	suite.conn.Write([]byte("bar\nboo\n"))
	msg = <-suite.msgChan
	suite.Equal("bar", string(msg.Content()))
	msg = <-suite.msgChan
	suite.Equal("boo", string(msg.Content()))
}

func TestWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerTestSuite))
}
