// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"

	internalsystemd "github.com/DataDog/datadog-agent/pkg/rshell/internal/systemd"
)

// SystemdTargetConfig selects the systemd host used by systemd-aware
// builtins. The zero value uses standard local paths. When explicit fields
// are used, omitted fields stay unavailable and never fall back to local
// paths.
type SystemdTargetConfig struct {
	JournalDirs          []string
	MachineIDPath        string
	JournalControlSocket string
	ManagerBusSocket     string
}

// WithSystemdTarget configures the trusted systemd target. Scripts cannot
// override these paths.
func WithSystemdTarget(config SystemdTargetConfig) RunnerOption {
	return func(r *Runner) error {
		target, err := internalsystemd.ResolveTarget(internalsystemd.Target{
			JournalDirs:          append([]string(nil), config.JournalDirs...),
			MachineIDPath:        config.MachineIDPath,
			JournalControlSocket: config.JournalControlSocket,
			ManagerBusSocket:     config.ManagerBusSocket,
		})
		if err != nil {
			return fmt.Errorf("WithSystemdTarget: %w", err)
		}
		r.systemdTarget = target
		r.systemdTargetConfigured = true
		return nil
	}
}
