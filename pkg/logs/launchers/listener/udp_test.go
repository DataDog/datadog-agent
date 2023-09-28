// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// use a randomly assigned port
var udpTestPort = 0

func TestUDPShouldReceiveMessage(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), 9000)
	listener.Start()

	conn, err := net.Dial("udp", listener.tailer.Conn.LocalAddr().String())
	assert.Nil(t, err)

	var msg *message.Message

	fmt.Fprintf(conn, "hello world\n")
	msg = <-msgChan
	assert.Equal(t, "hello world", string(msg.GetContent()))

	listener.Stop()
}

func TestUDPShouldStopWhenNotStarted(t *testing.T) {
	pp := mock.NewMockProvider()
	listener := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), 9000)
	// Don't call start, this is similar to if `startNewTailer` fails when start is called (such as if the port is already in use)
	listener.Stop()
}
