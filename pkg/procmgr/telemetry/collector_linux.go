// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package telemetry

import (
	"context"
	"os/exec"
	"strings"
)

func detectLegacySupervisor(service MigratableService) ManagementMode {
	for _, unit := range service.LegacySystemdUnits {
		if isSystemdUnitActive(unit) {
			return ManagementModeSystemd
		}
	}
	return ManagementModeNone
}

func isSystemdUnitActive(unit string) bool {
	ctx, cancel := clientContext(context.Background())
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemctl", "is-active", unit).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}
