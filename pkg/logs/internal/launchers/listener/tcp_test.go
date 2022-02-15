// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

// use a randomly assigned port
var tcpTestPort = 0

func TestTCPShouldReceivesMessages(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewTCPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 9000)
	listener.Start()

	conn, err := net.Dial("tcp", fmt.Sprintf("%s", listener.listener.Addr()))
	assert.Nil(t, err)

	var msg *message.Message

	fmt.Fprintf(conn, "hello world\n")
	msg = <-msgChan
	assert.Equal(t, "hello world", string(msg.Content))
	assert.Equal(t, 1, len(listener.tailers))

	listener.Stop()
}

func TestTCPDoesNotTruncateMessagesThatAreBiggerThanTheReadBufferSize(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewTCPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 100)
	listener.Start()

	conn, err := net.Dial("tcp", fmt.Sprintf("%s", listener.listener.Addr()))
	assert.Nil(t, err)

	var msg *message.Message

	fmt.Fprintf(conn, strings.Repeat("a", 80)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 80), string(msg.Content))

	fmt.Fprintf(conn, strings.Repeat("a", 200)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 200), string(msg.Content))

	fmt.Fprintf(conn, strings.Repeat("a", 70)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 70), string(msg.Content))

	listener.Stop()
}
