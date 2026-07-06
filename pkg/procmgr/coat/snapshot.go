// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package coat reports dd-procmgrd health and agent service supervision mode via COAT gauges.
package coat

import (
	"os"
	"runtime"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

const (
	defaultProcmgrSocketLinux = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
	defaultProcmgrSocketWin   = `\\.\pipe\datadog-procmgrd`
	processesDirRel           = "processes.d"

	// ManagementModeNone means the service is not supervised by procmgr or a known legacy supervisor.
	ManagementModeNone ManagementMode = "none"
	// ManagementModeProcmgr means the service is running under dd-procmgrd.
	ManagementModeProcmgr ManagementMode = "procmgr"
	// ManagementModeSystemd means the service is running under a legacy systemd unit.
	ManagementModeSystemd ManagementMode = "systemd"
	// ManagementModeWindowsService means the service is running under a legacy Windows service.
	ManagementModeWindowsService ManagementMode = "windows_service"
)

var managementModes = []ManagementMode{
	ManagementModeNone,
	ManagementModeProcmgr,
	ManagementModeSystemd,
	ManagementModeWindowsService,
}

// ManagementMode describes how an agent service process is supervised on the host.
type ManagementMode string

// DaemonSnapshot captures dd-procmgrd reachability and summary state.
type DaemonSnapshot struct {
	Reachable        bool
	Ready            bool
	RunningProcesses uint32
}

// ProcessSnapshot captures a single managed process reported by dd-procmgrd.
type ProcessSnapshot struct {
	Name  string
	State pb.ProcessState
}

// ServiceSnapshot captures install and supervision state for a migratable agent service.
type ServiceSnapshot struct {
	ID                string
	Installed         bool
	ProcmgrConfigured bool
	ProcmgrState      pb.ProcessState
	ManagementMode    ManagementMode
}

// Snapshot aggregates procmgr daemon and per-service supervision state.
type Snapshot struct {
	Daemon   DaemonSnapshot
	Services []ServiceSnapshot
}

func procmgrSocketPath() string {
	if path := os.Getenv("DD_PM_SOCKET_PATH"); path != "" {
		return path
	}
	if runtime.GOOS == "windows" {
		return defaultProcmgrSocketWin
	}
	return defaultProcmgrSocketLinux
}
