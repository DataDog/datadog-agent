// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import "strings"

// ServiceIDDDOT is the service ID of the DDOT extension in the migratable service catalog.
const ServiceIDDDOT = "ddot"

const (
	ProcessStateNotInstalled = "not_installed"
	ProcessStateUnknown      = "unknown"
	ProcessStateCreated      = "created"
	ProcessStateStarting     = "starting"
	ProcessStateRunning      = "running"
	ProcessStateStopping     = "stopping"
	ProcessStateStopped      = "stopped"
	ProcessStateCrashed      = "crashed"
	ProcessStateExited       = "exited"
	ProcessStateFailed       = "failed"
)

func (s Snapshot) ServiceProcessState(id string) string {
	for _, service := range s.Services {
		if service.ID != id {
			continue
		}
		switch service.ManagementMode {
		case ManagementModeProcmgr:
			return service.ProcmgrState
		case ManagementModeSystemd, ManagementModeWindowsService:
			// These modes are only set when the unit or service is active.
			return ProcessStateRunning
		}
		if !service.Installed && !service.ProcmgrConfigured {
			return ProcessStateNotInstalled
		}
		if service.ProcmgrConfigured && !s.Daemon.Reachable {
			return ProcessStateUnknown
		}
		return ProcessStateStopped
	}
	return ProcessStateUnknown
}

// parseProcmgrState normalizes a process state name reported by either transport into a
// ProcessState* constant: the gRPC client reports pb.ProcessState.String() (e.g. "RUNNING"),
// while the dd-procmgr CLI reports state_name() (e.g. "Running").
func parseProcmgrState(name string) string {
	switch strings.ToUpper(name) {
	case "CREATED":
		return ProcessStateCreated
	case "STARTING":
		return ProcessStateStarting
	case "RUNNING":
		return ProcessStateRunning
	case "STOPPING":
		return ProcessStateStopping
	case "STOPPED":
		return ProcessStateStopped
	case "CRASHED":
		return ProcessStateCrashed
	case "EXITED":
		return ProcessStateExited
	case "FAILED":
		return ProcessStateFailed
	default:
		return ProcessStateUnknown
	}
}
