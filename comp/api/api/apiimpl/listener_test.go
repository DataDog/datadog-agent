// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetIPCServerAddressPort(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		config.Mock(t)
		_, _, enabled := getIPCServerAddressPort()
		require.False(t, enabled)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("agent_ipc.port", 1234)

		host, hostPort, enabled := getIPCServerAddressPort()
		require.Equal(t, "localhost", host)
		require.Equal(t, "localhost:1234", hostPort)
		require.True(t, enabled)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("agent_ipc.port", 0)

		_, _, enabled := getIPCServerAddressPort()
		require.False(t, enabled)
	})
}
