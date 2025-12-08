// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetIPCServerAddressPort(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		configmock.New(t)
		_, enabled := getIPCServerAddressPort()
		require.False(t, enabled)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetWithoutSource("agent_ipc.port", 1234)

		hostPort, enabled := getIPCServerAddressPort()
		require.Equal(t, "localhost:1234", hostPort)
		require.True(t, enabled)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetWithoutSource("agent_ipc.port", 0)

		_, enabled := getIPCServerAddressPort()
		require.False(t, enabled)
	})
}

func TestGetListener(t *testing.T) {
	t.Run("localhost without port", func(t *testing.T) {
		configmock.New(t)

		_, err := getListener("localhost")
		require.Error(t, err)
	})

	t.Run("localhost with port", func(t *testing.T) {
		configmock.New(t)

		res, err := getListener("localhost:5009")
		require.NoError(t, err)

		defer res.Close()
		require.Equal(t, "127.0.0.1:5009", res.Addr().String())
	})

	t.Run("ipv4 with port", func(t *testing.T) {
		configmock.New(t)

		res, err := getListener("127.0.0.1:5009")
		require.NoError(t, err)

		defer res.Close()
		require.Equal(t, "127.0.0.1:5009", res.Addr().String())
	})

	t.Run("ipv4 without port", func(t *testing.T) {
		configmock.New(t)

		_, err := getListener("127.0.0.1")
		require.Error(t, err)
	})
}
