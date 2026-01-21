// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package listener

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestLinuxGetIPCServerPath(t *testing.T) {
	t.Run("default unix socket", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetWithoutSource("agent_ipc.use_socket", true)

		path, enabled := GetIPCServerPath()
		require.True(t, enabled)
		require.Equal(t, "/opt/datadog-agent/run/agent_ipc.socket", path)
	})
}

func TestLinuxGetListener(t *testing.T) {
	t.Run("socket listener", func(t *testing.T) {
		dir := t.TempDir()
		socketPath := filepath.Join(dir, "agent_ipc.socket")
		cfg := configmock.New(t)
		cfg.SetWithoutSource("agent_ipc.use_socket", true)
		cfg.SetWithoutSource("agent_ipc.socket_path", socketPath)

		res, err := GetListener(socketPath)
		require.NoError(t, err)

		defer res.Close()
		require.Equal(t, "unix", res.Addr().Network())
		require.Equal(t, socketPath, res.Addr().String())
	})
}
