// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	pipeline "github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/mock"
)

func TestLifeCycle(t *testing.T) {
	sources := []*config.IntegrationConfigLogSource{
		{Type: config.TCPType, Port: 8080, Tracker: status.NewTracker()},
		{Type: config.UDPType, Port: 8081, Tracker: status.NewTracker()},
		{Type: config.TCPType, Port: 8082, Tracker: status.NewTracker()},
	}
	pipeline := pipeline.NewMockProvider()
	listener := New(sources, pipeline)

	// tcp and udp listeners should be started
	listener.Start()
	assert.Equal(t, 2, len(listener.tcpListeners))
	assert.Equal(t, 1, len(listener.udpListeners))

	// all tcp and udp listeners should be stopped
	listener.Stop()
	assert.Equal(t, 0, len(listener.tcpListeners))
	assert.Equal(t, 0, len(listener.udpListeners))
}
