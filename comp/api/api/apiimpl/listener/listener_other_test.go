// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package listener

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestUnsupportedPlatformGetIPCServerPath(t *testing.T) {
	t.Run("unsupported platform misconfigured", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("agent_ipc.use_socket", true)

		_, enabled := GetIPCServerPath()
		require.False(t, enabled)
	})
}

func TestUnsupportedPlatformGetListener(t *testing.T) {
	t.Run("unsupported platform misconfigured", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("agent_ipc.use_socket", true)

		res, err := GetListener("localhost:5009")
		require.NoError(t, err)

		defer res.Close()
		require.Equal(t, "127.0.0.1:5009", res.Addr().String())
	})
}
