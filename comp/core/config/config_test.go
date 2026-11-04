// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestRealConfig(t *testing.T) {
	// point the ConfFilePath to a valid, but empty config file so that it does
	// not use the config file on the developer's system
	dir := t.TempDir()
	configFilePath := filepath.Join(dir, "datadog.yaml")
	_ = os.WriteFile(configFilePath, []byte("{}"), 0o666)

	config := NewMockFromYAMLFile(t, configFilePath)

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	require.Equal(t, "https://example.com", config.GetString("dd_url"))
}

func TestMockConfig(t *testing.T) {
	t.Setenv("DD_APP_KEY", "abc1234")
	t.Setenv("DD_URL", "https://example.com")

	config := NewMock(t)

	// values are set from env..
	require.Equal(t, "abc1234", config.GetString("app_key"))
	require.Equal(t, "https://example.com", config.GetString("dd_url"))

	// but defaults are set
	require.Equal(t, "localhost", config.GetString("cmd_host"))

	// values can also be set
	config.Set("app_key", "newvalue", model.SourceAgentRuntime)
	require.Equal(t, "newvalue", config.GetString("app_key"))
}

// TODO: test various bundle params

type activeConfigStream struct{}

func (activeConfigStream) IsActive() bool { return true }

// The streamed snapshot only carries the core agent's config, so the security agent's own
// file still has to be merged on the configstream path.
func TestSecurityAgentConfigMergedWithConfigStream(t *testing.T) {
	dir := t.TempDir()
	securityAgentPath := filepath.Join(dir, "security-agent.yaml")
	require.NoError(t, os.WriteFile(securityAgentPath, []byte("runtime_security_config:\n  enabled: true\n"), 0o600))

	cfg, err := newConfig(dependencies{
		Params:    NewSecurityAgentParams([]string{filepath.Join(dir, "datadog.yaml"), securityAgentPath}),
		Cfgstream: activeConfigStream{},
	})
	require.NoError(t, err)
	require.True(t, cfg.GetBool("runtime_security_config.enabled"))
}
