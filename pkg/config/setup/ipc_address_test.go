// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

const (
	localhostStr = "localhost"
	localhostV4  = "127.0.0.1"
	localhostV6  = "::1"
)

func TestGetIPCAddress(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		cfg := getConfig()
		val, err := GetIPCAddress(cfg)
		require.NoError(t, err)
		require.Equal(t, localhostStr, val)
	})

	t.Run("ipc_address from file", func(t *testing.T) {
		cfg := getConfig()
		cfg.Set("ipc_address", localhostV4, model.SourceFile)
		val, err := GetIPCAddress(cfg)
		require.NoError(t, err)
		require.Equal(t, localhostV4, val)
	})

	t.Run("ipc_address from env", func(t *testing.T) {
		cfg := getConfig()
		t.Setenv("DD_IPC_ADDRESS", localhostV4)
		val, err := GetIPCAddress(cfg)
		require.NoError(t, err)
		require.Equal(t, localhostV4, val)
	})

	t.Run("ipc_address takes precedence over cmd_host", func(t *testing.T) {
		cfg := getConfig()
		cfg.Set("ipc_address", localhostV4, model.SourceFile)
		cfg.Set("cmd_host", localhostV6, model.SourceFile)
		val, err := GetIPCAddress(cfg)
		require.NoError(t, err)
		require.Equal(t, localhostV4, val)
	})

	t.Run("ipc_address takes precedence over cmd_host", func(t *testing.T) {
		cfg := getConfig()
		cfg.Set("cmd_host", localhostV6, model.SourceFile)
		val, err := GetIPCAddress(cfg)
		require.NoError(t, err)
		require.Equal(t, localhostV6, val)
	})

	t.Run("error if not local", func(t *testing.T) {
		cfg := getConfig()
		cfg.Set("cmd_host", "111.111.111.111", model.SourceFile)
		_, err := GetIPCAddress(cfg)
		require.Error(t, err)
	})
}

func getConfig() model.Config {
	cfg := model.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	cfg.BindEnv("ipc_address")
	cfg.BindEnvAndSetDefault("cmd_host", localhostStr)
	return cfg
}
