// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConfigIgnoredComms(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("discovery.ignored_command_names", "dummy1 dummy2")

		discovery := newDiscovery()
		require.NotEmpty(t, discovery)

		assert.Equal(t, len(discovery.config.ignoreComms), 2)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_DISCOVERY_IGNORED_COMMAND_NAMES", "dummy1 dummy2")

		discovery := newDiscovery()
		require.NotEmpty(t, discovery)

		assert.Equal(t, len(discovery.config.ignoreComms), 2)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		discovery := newDiscovery()
		require.NotEmpty(t, discovery)

		assert.Equal(t, len(discovery.config.ignoreComms), 10)
	})
}
