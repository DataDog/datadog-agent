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
)

// linuxDaemonServiceUnits are the systemd units that may run dd-procmgrd, in
// selection preference order (stable → exp → legacy → legacy-exp).
var linuxDaemonServiceUnits = []struct {
	unit string
	kind daemonServiceCandidateKind
}{
	{"datadog-agent-procmgr.service", daemonCandidateStable},
	{"datadog-agent-procmgr-exp.service", daemonCandidateExp},
	{"datadog-agent-procmgrd.service", daemonCandidateLegacy},
	{"datadog-agent-procmgrd-exp.service", daemonCandidateLegacyExp},
}

func detectLegacySupervisor(ctx context.Context, service MigratableService) ManagementMode {
	for _, unit := range service.LegacySystemdUnits {
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
