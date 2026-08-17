// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

// ServiceIDDDOT is the service ID of the DDOT extension in the migratable service catalog.
const ServiceIDDDOT = "ddot"

const (
	ProcessStateUnset        = ""
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
			return procmgrProcessState(service.ProcmgrState)
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
	return ProcessStateUnset
}

func procmgrProcessState(state pb.ProcessState) string {
	switch state {
	case pb.ProcessState_CREATED:
		return ProcessStateCreated
	case pb.ProcessState_STARTING:
		return ProcessStateStarting
	case pb.ProcessState_RUNNING:
		return ProcessStateRunning
	case pb.ProcessState_STOPPING:
		return ProcessStateStopping
	case pb.ProcessState_STOPPED:
		return ProcessStateStopped
	case pb.ProcessState_CRASHED:
		return ProcessStateCrashed
	case pb.ProcessState_EXITED:
		return ProcessStateExited
	case pb.ProcessState_FAILED:
		return ProcessStateFailed
	default:
		return ProcessStateUnknown
	}
}
