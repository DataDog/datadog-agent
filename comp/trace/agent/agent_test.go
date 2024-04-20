// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent defines the tracer agent.
package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/stretchr/testify/assert"
)

func TestFindAddr(t *testing.T) {
	t.Run("pipe", func(t *testing.T) {
		addr, err := findAddr(&config.AgentConfig{StatsdPipeName: "sock.pipe"})
		assert.NoError(t, err)
		assert.Equal(t, addr, `\\.\pipe\sock.pipe`)
	})

	t.Run("udp-localhost", func(t *testing.T) {
		addr, err := findAddr(&config.AgentConfig{
			StatsdHost: "localhost",
			StatsdPort: 123,
		})
		assert.NoError(t, err)
		assert.Equal(t, addr, `localhost:123`)
	})

	t.Run("udp-ipv6", func(t *testing.T) {
		addr, err := findAddr(&config.AgentConfig{
			StatsdHost: "::1",
			StatsdPort: 123,
		})
		assert.NoError(t, err)
		assert.Equal(t, addr, `[::1]:123`) // must add the square brackets to properly connect
	})

	t.Run("socket", func(t *testing.T) {
		addr, err := findAddr(&config.AgentConfig{StatsdSocket: "pipe.sock"})
		assert.NoError(t, err)
		assert.Equal(t, addr, `unix://pipe.sock`)
	})

	t.Run("error", func(t *testing.T) {
		_, err := findAddr(&config.AgentConfig{})
		assert.Error(t, err)
	})
}
