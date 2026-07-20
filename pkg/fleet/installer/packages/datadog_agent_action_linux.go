// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

const parProcmgrConfigName = "datadog-agent-action.yaml"

// isPARProcessManagerEnabled reads private_action_runner.use_process_manager from the given
// datadog.yaml path. It defaults to false (including when the file or key is absent), matching
// the Private Action Runner's dd-procmgr supervision being opt-in.
func isPARProcessManagerEnabled(datadogYamlPath string) (bool, error) {
	data, err := os.ReadFile(datadogYamlPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read datadog.yaml: %w", err)
	}
	var cfg struct {
		PrivateActionRunner struct {
			UseProcessManager bool `yaml:"use_process_manager"`
		} `yaml:"private_action_runner"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false, fmt.Errorf("failed to parse datadog.yaml: %w", err)
	}
	return cfg.PrivateActionRunner.UseProcessManager, nil
}

// writePARProcmgrConfig writes the dd-procmgr config for the Private Action Runner into the
// package processes.d. The supervising dd-procmgr resolves ${DD_CONF_DIR} to its stable or
// experiment config directory at launch.
func writePARProcmgrConfig(installRoot string) error {
	processesDir := filepath.Join(installRoot, "processes.d")
	config := strings.ReplaceAll(embedded.PARProcessConfig, "/opt/datadog-agent", installRoot)
	if err := os.MkdirAll(processesDir, 0755); err != nil {
		return fmt.Errorf("failed to write private action runner procmgr config: %w", err)
	}
	path := filepath.Join(processesDir, parProcmgrConfigName)
	return os.WriteFile(path, []byte(config), 0644)
}

// removePARProcmgrConfig removes the dd-procmgr config for the Private Action Runner, restoring
// the legacy systemd unit as the sole supervisor.
func removePARProcmgrConfig(installRoot string) error {
	path := filepath.Join(installRoot, "processes.d", parProcmgrConfigName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove private action runner procmgr config: %w", err)
	}
	return nil
}
