// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

// MigratableService describes an agent service that can be supervised by dd-procmgrd.
// Add future migrations by appending to migratableServices.
type MigratableService struct {
	// ID is the stable telemetry tag value (e.g. "ddot", "trace").
	ID string
	// ProcmgrProcessName is the process name in processes.d and dd-procmgrd.
	ProcmgrProcessName string
	// ProcmgrConfigFile is the basename under processes.d/.
	ProcmgrConfigFile string
	// InstallMarkerRel is a path relative to the agent install root used to detect installation.
	InstallMarkerRel string
	// WindowsPackageName is the fleet installer package name used to locate the install marker on Windows.
	WindowsPackageName string
	// LegacySystemdUnits are systemd units checked when procmgr is not supervising the service.
	LegacySystemdUnits []string
	// LegacyWindowsService is the Windows SCM service name used as the legacy supervisor.
	LegacyWindowsService string
}

// migratableServices is the catalog of services tracked for procmgr migration telemetry.
var migratableServices = []MigratableService{
	{
		ID:                 "ddot",
		ProcmgrProcessName: "datadog-agent-ddot",
		ProcmgrConfigFile:  "datadog-agent-ddot.yaml",
		InstallMarkerRel:   "ext/ddot/embedded/bin/otel-agent",
		WindowsPackageName: "datadog-agent-ddot",
		LegacySystemdUnits: []string{
			"datadog-agent-ddot.service",
			"datadog-agent-ddot-exp.service",
		},
		LegacyWindowsService: "datadog-otel-agent",
	},
}

func serviceByID(id string) (MigratableService, bool) {
	for _, service := range migratableServices {
		if service.ID == id {
			return service, true
		}
	}
	return MigratableService{}, false
}
