// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

const udpTestPort = 10513

func TestUDPShouldReceiveMessage(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewUDPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), defaultFrameSize)
	listener.Start()

	conn, err := net.Dial("udp", fmt.Sprintf("localhost:%d", udpTestPort))
	assert.Nil(t, err)

	var msg message.Message

	fmt.Fprintf(conn, "hello world\n")
	msg = <-msgChan
	assert.Equal(t, "hello world", string(msg.Content()))

	listener.Stop()
}

func TestUDPShouldProperlyTruncateTooBigMessages(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewUDPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), 100)
	listener.Start()

	conn, err := net.Dial("udp", fmt.Sprintf("localhost:%d", udpTestPort))
	assert.Nil(t, err)

	var msg message.Message

	fmt.Fprintf(conn, strings.Repeat("a", 80)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 80), string(msg.Content()))

	fmt.Fprintf(conn, strings.Repeat("a", 100)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 100), string(msg.Content()))

	fmt.Fprintf(conn, strings.Repeat("a", 70)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 70), string(msg.Content()))

	listener.Stop()
}
