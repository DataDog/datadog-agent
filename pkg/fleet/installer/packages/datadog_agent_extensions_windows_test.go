// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func TestExtensionConfigRead(t *testing.T) {
	originalDir := paths.AgentConfigDir
	t.Cleanup(func() { paths.AgentConfigDir = originalDir })
	t.Setenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE", "")

	for _, state := range []string{"missing", "present"} {
		t.Run(state, func(t *testing.T) {
			switch state {
			case "missing":
				paths.AgentConfigDir = filepath.Join(t.TempDir(), "missing")
			case "present":
				paths.AgentConfigDir = t.TempDir()
			}
			if state == "present" {
				require.NoError(t, os.WriteFile(filepath.Join(paths.AgentConfigDir, "datadog.yaml"), []byte(`
infrastructure_mode: end_user_device
installer:
  registry:
    url: custom.example
    extensions:
      datadog-agent:
        ddot:
          url: custom-ddot.example
`), 0600))
			}

			config, ok := loadDatadogAgentConfig()
			registryEnv := &env.Env{APIKey: "synthetic-key"}
			overrides := setRegistryConfig(registryEnv)
			if state == "present" {
				require.True(t, ok)
				require.Equal(t, infrastructureModeEndUserDevice, config.InfrastructureMode)
				require.Equal(t, "custom.example", registryEnv.RegistryOverride)
				require.Contains(t, overrides, "ddot")
				require.Equal(t, "custom-ddot.example", overrides["ddot"].URL)
			} else {
				require.False(t, ok)
				require.Zero(t, config)
				require.Nil(t, overrides)
				require.Equal(t, &env.Env{APIKey: "synthetic-key"}, registryEnv)
			}
		})
	}
}
