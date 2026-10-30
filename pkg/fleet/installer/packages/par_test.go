// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestConfigurePARPackageDefault(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		for _, target := range []string{"datadog.yaml", "datadog.yaml.example"} {
			t.Run(goos+"/"+target, func(t *testing.T) {
				t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "")
				dir := t.TempDir()
				path := filepath.Join(dir, target)
				original := "# API key filled by the install script\n# api_key: <API_KEY>\nprivate_action_runner:\n  self_enroll: false\n  actions_allowlist: [custom.action]\n"
				require.NoError(t, os.WriteFile(path, []byte(original), 0640))
				require.NoError(t, configurePARDefaultAt(dir, goos, true, false, func() (bool, error) { return true, nil }))
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var cfg struct {
					PAR map[string]any `yaml:"private_action_runner"`
				}
				require.NoError(t, yaml.Unmarshal(data, &cfg))
				assert.Equal(t, true, cfg.PAR["enabled"])
				assert.Equal(t, false, cfg.PAR["self_enroll"])
				assert.Equal(t, []any{"custom.action"}, cfg.PAR["actions_allowlist"])
				assert.Contains(t, string(data), "# api_key: <API_KEY>")
				if target == "datadog.yaml.example" {
					_, err := os.Stat(filepath.Join(dir, "datadog.yaml"))
					require.True(t, os.IsNotExist(err), "leave configuration creation to the install script")
				}
				// Reinstalls preserve explicit opt-outs and are idempotent.
				t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false")
				require.NoError(t, configurePARDefaultAt(dir, goos, true, false, func() (bool, error) { return true, nil }))
				t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "")
				require.NoError(t, configurePARDefaultAt(dir, goos, true, false, func() (bool, error) { return true, nil }))
				data, err = os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, yaml.Unmarshal(data, &cfg))
				assert.Equal(t, false, cfg.PAR["enabled"])
			})
		}
	}
}

func TestConfigurePARPackageDefaultMissingConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, configurePARDefaultAt(dir, "windows", true, false, nil))
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}
