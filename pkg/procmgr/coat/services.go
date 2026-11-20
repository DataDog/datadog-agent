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
	// InstallMarkerRels are paths relative to the agent install root; if any exist, the
	// service payload is considered installed (e.g. DDOT extension and standalone DEB paths).
	InstallMarkerRels []string
	// WindowsPackageName is the fleet installer package name used to locate the install marker on Windows.
	WindowsPackageName string
}

// ProcmgrConfigFile is the basename under processes.d/.
func (s MigratableService) ProcmgrConfigFile() string {
	return s.ProcmgrProcessName + ".yaml"
}

// migratableServices is the catalog of services tracked for procmgr migration telemetry.
var migratableServices = []MigratableService{
	{
		ID:                 "ddot",
		ProcmgrProcessName: "datadog-agent-ddot",
		InstallMarkerRels: []string{
			"ext/ddot/embedded/bin/otel-agent",
			"embedded/bin/otel-agent",
		},
		WindowsPackageName: "datadog-agent-ddot",
	},
	{
		ID:                 "agent-data-plane",
		ProcmgrProcessName: "datadog-agent-data-plane",
		InstallMarkerRels: []string{
			"embedded/bin/agent-data-plane",
			"bin/agent/agent-data-plane",
		},
	},
	{
		ID:                 "process",
		ProcmgrProcessName: "datadog-agent-process",
		InstallMarkerRels: []string{
			"embedded/bin/process-agent",
			"bin/agent/process-agent",
		},
	},
	{
		ID:                 "sysprobe",
		ProcmgrProcessName: "datadog-agent-sysprobe",
		InstallMarkerRels: []string{
			"embedded/bin/system-probe",
			"bin/agent/system-probe",
		},
	},
	{
		ID:                 "trace",
		ProcmgrProcessName: "datadog-agent-trace",
		InstallMarkerRels: []string{
			"embedded/bin/trace-agent",
			"bin/agent/trace-agent",
		},
	},
	{
		ID:                 "security",
		ProcmgrProcessName: "datadog-agent-security",
		InstallMarkerRels: []string{
			"embedded/bin/security-agent",
			"bin/agent/security-agent",
		},
	},
	{
		// Combined Private Action Runner. Still a legacy systemd/SCM unit on hosts that have
		// not split it into par-control + action-executor under dd-procmgrd.
		ID:                 "action",
		ProcmgrProcessName: "datadog-agent-action",
		InstallMarkerRels: []string{
			"embedded/bin/privateactionrunner",
			"bin/agent/privateactionrunner",
		},
	},
	{
		// On-demand executor spawned by par-control. Procmgr-native: no legacy unit owns it.
		ID:                 "action-executor",
		ProcmgrProcessName: "datadog-agent-action-executor",
		InstallMarkerRels: []string{
			"embedded/bin/privateactionrunner",
			"bin/agent/privateactionrunner",
		},
	},
	{
		// PAR control plane. Procmgr-native: no legacy unit owns it.
		ID:                 "par-control",
		ProcmgrProcessName: "datadog-agent-par-control",
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

// ProcmgrConfigFiles returns the processes.d basenames the catalog tracks. Drift tests compare
// this set to the installer embeds so a new migration cannot land silently without COAT coverage.
func ProcmgrConfigFiles() []string {
	out := make([]string, 0, len(migratableServices))
	for _, service := range migratableServices {
		out = append(out, service.ProcmgrConfigFile())
	}
	return out
}
