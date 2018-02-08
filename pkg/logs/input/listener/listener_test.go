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
)

func TestLifeCycle(t *testing.T) {
	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.TCPType, Port: 8081}),
		config.NewLogSource("", &config.LogsConfig{Type: config.UDPType, Port: 8082}),
		config.NewLogSource("", &config.LogsConfig{Type: config.TCPType, Port: 8083}),
	}
	pipeline := pipeline.NewMockProvider()
	networklisteners := New(sources, pipeline)

	// tcp and udp listeners should be started
	networklisteners.Start()
	assert.Equal(t, 3, len(networklisteners.listeners))

	// all tcp and udp listeners should be stopped
	networklisteners.Stop()
	assert.Equal(t, 0, len(networklisteners.listeners))
}
