// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

// setPARDefault opts supported Linux installer deployments into PAR without
// changing the Agent-wide default or overwriting an operator's configuration.
// systemd must actually be running: merely having systemctl in PATH does not
// imply that the host can launch dd-procmgr (notably inside containers).
func (s *Setup) setPARDefault(packages []packageWithVersion, goos string, systemdRunning func() (bool, error)) error {
	if goos != "linux" || !s.Env.ProcessManagerEnabled || s.Env.FIPSMode {
		return nil
	}
	if s.Config.DatadogYAML.PrivateActionRunner.Enabled != nil || os.Getenv("DD_PRIVATE_ACTION_RUNNER_ENABLED") != "" {
		return nil
	}
	installingAgent := false
	for _, pkg := range packages {
		if pkg.name == DatadogAgentPackage {
			installingAgent = true
			break
		}
	}
	if !installingAgent {
		return nil
	}
	running, err := systemdRunning()
	if err != nil {
		return fmt.Errorf("check systemd support: %w", err)
	}
	if !running {
		return nil
	}

	// A missing key can acquire the installer default on a reinstall, but an
	// existing true or false is an explicit choice. Leave other PAR settings
	// (including self_enroll, split_enabled and the allowlist) untouched.
	data, err := os.ReadFile(filepath.Join(s.configDir, "datadog.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing Agent configuration: %w", err)
	}
	var current struct {
		PrivateActionRunner struct {
			Enabled *bool `yaml:"enabled"`
		} `yaml:"private_action_runner"`
	}
	if err := yaml.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("read existing PAR setting: %w", err)
	}
	if current.PrivateActionRunner.Enabled == nil {
		s.Config.DatadogYAML.PrivateActionRunner.Enabled = config.BoolToPtr(true)
	}
	return nil
}
