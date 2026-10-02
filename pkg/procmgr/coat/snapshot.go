// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package coat reports dd-procmgrd health and agent service supervision mode via COAT gauges.
package coat

import (
	"os"
	"runtime"
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

// serviceSupervisors are the tags emitted on agent_service_running. ManagementModeNone is
// excluded: "not up under anyone" is absence after zero-drop, not a supervisor:none series.
var serviceSupervisors = []string{
	string(ManagementModeProcmgr),
	string(ManagementModeSystemd),
	string(ManagementModeWindowsService),
}

// serviceRunningUnder reports whether agent_service_running for supervisor should be set for
// service. Unlike management_mode (ownership: procmgr = listed in any state), this is up-only:
// procmgr requires ProcessStateRunning; legacy modes already imply the unit/service is active.
func serviceRunningUnder(service ServiceSnapshot, supervisor string) bool {
	switch ManagementMode(supervisor) {
	case ManagementModeProcmgr:
		return service.ManagementMode == ManagementModeProcmgr &&
			service.ProcmgrState == ProcessStateRunning
	case ManagementModeSystemd, ManagementModeWindowsService:
		return service.ManagementMode == ManagementMode(supervisor)
	default:
		return false
	}
}

// ManagementMode describes how an agent service process is supervised on the host.
type ManagementMode string

// DaemonSnapshot captures dd-procmgrd reachability and summary state.
type DaemonSnapshot struct {
	Reachable        bool   `json:"reachable"`
	Ready            bool   `json:"ready"`
	RunningProcesses uint32 `json:"running_processes"`
	// Version is the dd-procmgrd build version. Empty when the daemon is unreachable.
	Version string `json:"version,omitempty"`
	// UptimeSeconds is how long dd-procmgrd has been running, which distinguishes a daemon
	// that has just restarted from one that has been up since boot.
	UptimeSeconds uint64 `json:"uptime_seconds"`
	// TotalProcesses is the process count dd-procmgrd reports. A mismatch with the number of
	// processes in a List result is itself diagnostic.
	TotalProcesses uint32 `json:"total_processes"`
	// ServiceState is the mapped OS unit/SCM state of dd-procmgrd
	// (running|starting|stopping|stopped|failed|unknown|not_installed).
	// Empty on non-linux/windows hosts. Independent of Reachable/Ready.
	ServiceState string `json:"service_state,omitempty"`
}

// ProcessSnapshot captures a single managed process reported by dd-procmgrd.
//
// List fills every field down to User; the ones after that come only from Describe and stay
// zero-valued in a List result. Neither call reports the process environment, which is left out
// deliberately so it cannot reach a flare.
type ProcessSnapshot struct {
	Name string `json:"name"`
	// State is one of the ProcessState* constants, already normalized by the client.
	State        string   `json:"state"`
	UUID         string   `json:"uuid,omitempty"`
	PID          uint32   `json:"pid"`
	Command      string   `json:"command,omitempty"`
	Args         []string `json:"args,omitempty"`
	RestartCount uint32   `json:"restart_count"`
	// LastExitCode and LastSignal are nil when the process has not exited yet, which is
	// distinct from having exited with code 0.
	LastExitCode *int32 `json:"last_exit_code,omitempty"`
	LastSignal   *int32 `json:"last_signal,omitempty"`
	Profile      string `json:"profile,omitempty"`
	User         string `json:"user,omitempty"`

	// Describe-only fields follow.
	Description string `json:"description,omitempty"`
	WorkingDir  string `json:"working_dir,omitempty"`
	// RuntimeUser is the identity the process actually runs as, which can differ from User.
	RuntimeUser   string `json:"runtime_user,omitempty"`
	RestartPolicy string `json:"restart_policy,omitempty"`
	AutoStart     bool   `json:"auto_start"`
	// ConditionPathExists is the path gating the start, if any. dd-procmgrd does not report
	// condition_config_any over its RPC, so a config-gated process is only identifiable as
	// State == ProcessStateCreated while AutoStart is true.
	ConditionPathExists string   `json:"condition_path_exists,omitempty"`
	After               []string `json:"after,omitempty"`
	Before              []string `json:"before,omitempty"`
	Stdout              string   `json:"stdout,omitempty"`
	Stderr              string   `json:"stderr,omitempty"`
}

// ServiceSnapshot captures install and supervision state for a migratable agent service.
type ServiceSnapshot struct {
	ID                string `json:"id"`
	Installed         bool   `json:"installed"`
	ProcmgrConfigured bool   `json:"procmgr_configured"`
	// ProcmgrState is one of the ProcessState* constants, already normalized by the client.
	ProcmgrState   string         `json:"procmgr_state"`
	ManagementMode ManagementMode `json:"management_mode"`
}

// Snapshot aggregates procmgr daemon and per-service supervision state.
type Snapshot struct {
	Daemon   DaemonSnapshot    `json:"daemon"`
	Services []ServiceSnapshot `json:"services"`
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
