// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

const tcpTestPort = 10512

func TestTCPShouldReceivesMessages(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewTCPListener(pp, config.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}))
	listener.Start()

	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", tcpTestPort))
	assert.Nil(t, err)

	// should receive and decode message
	fmt.Fprintf(conn, "hello world\n")
	msg := <-msgChan
	assert.Equal(t, "hello world", string(msg.Content()))
	assert.Equal(t, 1, len(listener.tailers))

	listener.Stop()
}
