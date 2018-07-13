// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

const udpTestPort = 10513

func TestUDPShouldReceiveMessage(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewUDPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: udpTestPort}))
	listener.Start()

	conn, err := net.Dial("udp", fmt.Sprintf("localhost:%d", udpTestPort))
	assert.Nil(t, err)

	fmt.Fprintf(conn, "hello world\n")
	msg := <-msgChan
	assert.Equal(t, "hello world", string(msg.Content()))

	listener.Stop()
}

func TestIsConnectionClosedError(t *testing.T) {
	listener := NewUDPListener(nil, nil)
	assert.True(t, listener.isClosedConnError(errors.New("use of closed network connection")))
	assert.False(t, listener.isClosedConnError(io.EOF))
}
