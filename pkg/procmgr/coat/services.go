// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

// MigratableService describes an agent service that can be supervised by dd-procmgrd.
// Add future migrations by appending to migratableServices.
type MigratableService struct {
	// ID is the stable telemetry tag value (e.g. "ddot", "trace").
	ID string
	// ProcmgrProcessName is the process name in processes.d and dd-procmgrd.
	ProcmgrProcessName string
	// ProcmgrConfigFile is the basename under processes.d/.
	ProcmgrConfigFile string
	// InstallMarkerRels are paths relative to the agent install root; if any exist, the
	// service payload is considered installed (e.g. DDOT extension and standalone DEB paths).
	InstallMarkerRels []string
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
		InstallMarkerRels: []string{
			"ext/ddot/embedded/bin/otel-agent",
			"embedded/bin/otel-agent",
		},
		WindowsPackageName: "datadog-agent-ddot",
		LegacySystemdUnits: []string{
			"datadog-agent-ddot.service",
			"datadog-agent-ddot-exp.service",
		},
		LegacyWindowsService: "datadog-otel-agent",
	},
	{
		ID:                 "agent-data-plane",
		ProcmgrProcessName: "datadog-agent-data-plane",
		ProcmgrConfigFile:  "datadog-agent-data-plane.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/agent-data-plane",
			"bin/agent/agent-data-plane",
		},
		LegacySystemdUnits: []string{
			"datadog-agent-data-plane.service",
			"datadog-agent-data-plane-exp.service",
		},
	},
	{
		ID:                 "process",
		ProcmgrProcessName: "datadog-agent-process",
		ProcmgrConfigFile:  "datadog-agent-process.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/process-agent",
			"bin/agent/process-agent",
		},
		LegacySystemdUnits: []string{
			"datadog-agent-process.service",
			"datadog-agent-process-exp.service",
		},
		LegacyWindowsService: "datadog-process-agent",
	},
	{
		ID:                 "sysprobe",
		ProcmgrProcessName: "datadog-agent-sysprobe",
		ProcmgrConfigFile:  "datadog-agent-sysprobe.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/system-probe",
			"bin/agent/system-probe",
		},
		LegacySystemdUnits: []string{
			"datadog-agent-sysprobe.service",
			"datadog-agent-sysprobe-exp.service",
		},
		LegacyWindowsService: "datadog-system-probe",
	},
	{
		// Combined Private Action Runner. Still a legacy systemd/SCM unit on hosts that have
		// not split it into par-control + action-executor under dd-procmgrd.
		ID:                 "action",
		ProcmgrProcessName: "datadog-agent-action",
		ProcmgrConfigFile:  "datadog-agent-action.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/privateactionrunner",
			"bin/agent/privateactionrunner",
		},
		LegacySystemdUnits: []string{
			"datadog-agent-action.service",
			"datadog-agent-action-exp.service",
		},
		LegacyWindowsService: "datadog-agent-action",
	},
	{
		// On-demand executor spawned by par-control. Procmgr-native: no legacy unit owns it.
		ID:                 "action-executor",
		ProcmgrProcessName: "datadog-agent-action-executor",
		ProcmgrConfigFile:  "datadog-agent-action-executor.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/privateactionrunner",
			"bin/agent/privateactionrunner",
		},
	},
	{
		// PAR control plane. Procmgr-native: no legacy unit owns it.
		ID:                 "par-control",
		ProcmgrProcessName: "datadog-agent-par-control",
		ProcmgrConfigFile:  "datadog-agent-par-control.yaml",
		InstallMarkerRels: []string{
			"embedded/bin/par-control",
			"bin/agent/par-control",
		},
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

// ProcmgrConfigFiles returns the processes.d basenames the catalog tracks. Callers that ship
// processes.d entries (the installer embeds) use this to assert every shipped config is
// registered, so a new migration cannot land silently without COAT and flare coverage.
func ProcmgrConfigFiles() []string {
	out := make([]string, 0, len(migratableServices))
	for _, service := range migratableServices {
		out = append(out, service.ProcmgrConfigFile)
	}
	return out
}
