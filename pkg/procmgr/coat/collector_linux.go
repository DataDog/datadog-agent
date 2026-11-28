// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package coat

import (
	"context"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/linuxunits"
)

// linuxDaemonServiceUnits are the systemd units that may run dd-procmgrd, in
// selection preference order (stable → exp → legacy → legacy-exp).
var linuxDaemonServiceUnits = []struct {
	unit string
	kind daemonServiceCandidateKind
}{
	{linuxunits.Procmgr.Procmgr.Name, daemonCandidateStable},
	{linuxunits.Procmgr.Procmgr.ExpName(), daemonCandidateExp},
	{linuxunits.Systemd.LegacyProcmgrd.Name, daemonCandidateLegacy},
	{linuxunits.Systemd.LegacyProcmgrd.ExpName(), daemonCandidateLegacyExp},
}

// legacySupervisors are the systemd units supervising a service when procmgr does not, by service ID.
var legacySupervisors = map[string]units.Unit{
	"ddot":             linuxunits.Systemd.DDOT,
	"agent-data-plane": linuxunits.Systemd.DataPlane,
	"process":          linuxunits.Systemd.Process,
	"sysprobe":         linuxunits.Systemd.Sysprobe,
	"trace":            linuxunits.Systemd.Trace,
	"security":         linuxunits.Systemd.Security,
	"action":           linuxunits.Systemd.Action,
}

func detectLegacySupervisor(ctx context.Context, service MigratableService) ManagementMode {
	legacy, ok := legacySupervisors[service.ID]
	if !ok {
		return ManagementModeNone
	}
	for _, unit := range units.StableAndExpNames(legacy) {
		if isSystemdUnitActive(ctx, unit) {
			return ManagementModeSystemd
		}
	}
	return ManagementModeNone
}

func isSystemdUnitActive(parent context.Context, unit string) bool {
	ctx, cancel := clientContext(parent)
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemctl", "is-active", unit).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

func detectDaemonServiceState(parent context.Context) string {
	results := make([]daemonServiceCandidateResult, 0, len(linuxDaemonServiceUnits))
	for _, candidate := range linuxDaemonServiceUnits {
		results = append(results, daemonServiceCandidateResult{
			kind:  candidate.kind,
			state: querySystemdUnitMappedState(parent, candidate.unit),
		})
	}
	return selectDaemonServiceState(results)
}

func querySystemdUnitMappedState(parent context.Context, unit string) string {
	ctx, cancel := clientContext(parent)
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemctl", "show",
		"-p", "LoadState", "-p", "ActiveState", "--", unit,
	).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ProcessStateUnknown
	}
	return mapSystemdShowOutput(string(out))
}
