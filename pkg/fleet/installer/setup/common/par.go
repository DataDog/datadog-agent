// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

// setPARDefault opts supported installer deployments into PAR without
// changing the Agent-wide default or overwriting an operator's configuration.
// systemd must actually be running: merely having systemctl in PATH does not
// imply that the host can launch dd-procmgr (notably inside containers).
func (s *Setup) setPARDefault(packages []packageWithVersion, goos string, systemdRunning func() (bool, error)) error {
	if !s.Env.ProcessManagerEnabled || s.Env.FIPSMode {
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
	enabled, err := config.PARInstallerDefault(filepath.Join(s.configDir, "datadog.yaml"), goos, s.Env.ProcessManagerEnabled, s.Env.FIPSMode, systemdRunning)
	if err != nil {
		return err
	}
	s.Config.DatadogYAML.PrivateActionRunner.Enabled = enabled
	return nil
}
