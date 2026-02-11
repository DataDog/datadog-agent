// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestInitConfigJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil, nil)

	var initConfig integration.Data = []byte(`java_options: -Xmx200m`)

	j.ConfigureFromInitConfig(initConfig)

	require.Contains(t, j.JavaOptions, "Xmx200m")
}

func TestConflictingInitConfigJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil, nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInitConfig(configOne)
	j.ConfigureFromInitConfig(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}

func TestConflictingInstanceJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil, nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInstance(configOne)
	j.ConfigureFromInstance(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}

func TestConflictingInstanceInitJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil, nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInitConfig(configOne)
	j.ConfigureFromInstance(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}

func TestGetPreferredDSDEndpoint(t *testing.T) {
	cfg := configmock.New(t)

	t.Run("UDS configured but not available", func(t *testing.T) {
		cfg.SetWithoutSource("dogstatsd_socket", "/tmp/nonexistent-dsd-test.sock")
		cfg.SetWithoutSource("dogstatsd_port", "8125")
		cfg.SetWithoutSource("use_dogstatsd", true)

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:localhost:8125", j.getPreferredDSDEndpoint())
	})

	t.Run("UDS not configured", func(t *testing.T) {
		cfg.SetWithoutSource("dogstatsd_socket", "")
		cfg.SetWithoutSource("dogstatsd_port", "8125")
		cfg.SetWithoutSource("use_dogstatsd", true)

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:localhost:8125", j.getPreferredDSDEndpoint())
	})

	t.Run("bind host 0.0.0.0 normalized to localhost", func(t *testing.T) {
		cfg.SetWithoutSource("dogstatsd_socket", "")
		cfg.SetWithoutSource("dogstatsd_port", "8125")
		cfg.SetWithoutSource("use_dogstatsd", true)
		cfg.SetWithoutSource("bind_host", "0.0.0.0")

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:localhost:8125", j.getPreferredDSDEndpoint())
	})

	t.Run("custom bind host and port", func(t *testing.T) {
		cfg.SetWithoutSource("dogstatsd_socket", "")
		cfg.SetWithoutSource("dogstatsd_port", "9125")
		cfg.SetWithoutSource("use_dogstatsd", true)
		cfg.SetWithoutSource("bind_host", "127.0.0.2")

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:127.0.0.2:9125", j.getPreferredDSDEndpoint())
	})
}
