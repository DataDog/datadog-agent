// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import "strings"

// daemonServiceStates are the states reported as a tag on the
// procmgr_daemon_service_state gauge. Vocab matches procmgr_process_state for the
// overlapping values; process-only states (created, crashed, exited) are omitted.
var daemonServiceStates = []string{
	ProcessStateRunning,
	ProcessStateStarting,
	ProcessStateStopping,
	ProcessStateStopped,
	ProcessStateFailed,
	ProcessStateUnknown,
	ProcessStateNotInstalled,
}

// Windows SCM state constants from winsvc.h, kept numeric so mapping tests do not
// need a windows build tag.
const (
	scmStopped         uint32 = 1
	scmStartPending    uint32 = 2
	scmStopPending     uint32 = 3
	scmRunning         uint32 = 4
	scmContinuePending uint32 = 5
	scmPausePending    uint32 = 6
	scmPaused          uint32 = 7
)

type daemonServiceCandidateKind int

const (
	daemonCandidateStable daemonServiceCandidateKind = iota
	daemonCandidateExp
	daemonCandidateLegacy
	daemonCandidateLegacyExp
)

type daemonServiceCandidateResult struct {
	kind  daemonServiceCandidateKind
	state string
}

func daemonServiceStateIsActive(current, state string) bool {
	return current != "" && current == state
}

// selectDaemonServiceState picks exactly one mapped state from candidate unit/service
// query results, preferring a running unit, then a sole transitional unit, then a failed
// unit, then the first loaded candidate in stable → exp → legacy → legacy-exp order.
//
// A failure outranks the flavor order because both units stay installed during a fleet
// experiment: stable is loaded but inactive while exp runs. Falling straight through to
// stable would report an exp unit that failed as a quiet "stopped", which reads as nothing
// being run rather than as the daemon breaking.
func selectDaemonServiceState(results []daemonServiceCandidateResult) string {
	if len(results) == 0 {
		return ProcessStateNotInstalled
	}

	for _, r := range results {
		if r.state == ProcessStateRunning {
			return ProcessStateRunning
		}
	}

	var transitional *daemonServiceCandidateResult
	for i := range results {
		r := &results[i]
		if r.state != ProcessStateStarting && r.state != ProcessStateStopping {
			continue
		}
		if transitional != nil {
			transitional = nil
			break
		}
		transitional = r
	}
	if transitional != nil {
		return transitional.state
	}

	for _, r := range results {
		if r.state == ProcessStateFailed {
			return ProcessStateFailed
		}
	}

	for _, kind := range []daemonServiceCandidateKind{
		daemonCandidateStable,
		daemonCandidateExp,
		daemonCandidateLegacy,
		daemonCandidateLegacyExp,
	} {
		for _, r := range results {
			if r.kind == kind && r.state != ProcessStateNotInstalled {
				return r.state
			}
		}
	}
	return ProcessStateNotInstalled
}

// mapSystemdUnitState maps systemd LoadState/ActiveState onto the process_state vocabulary.
func mapSystemdUnitState(loadState, activeState string) string {
	if loadState == "not-found" {
		return ProcessStateNotInstalled
	}
	switch activeState {
	case "active":
		return ProcessStateRunning
	case "activating":
		return ProcessStateStarting
	case "deactivating":
		return ProcessStateStopping
	case "failed":
		return ProcessStateFailed
	case "inactive", "maintenance":
		return ProcessStateStopped
	default:
		return ProcessStateUnknown
	}
}

// mapSystemdShowOutput parses `systemctl show -p LoadState -p ActiveState` output.
func mapSystemdShowOutput(out string) string {
	loadState, activeState := "", ""
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "LoadState":
			loadState = val
		case "ActiveState":
			activeState = val
		}
	}
	if loadState == "" && activeState == "" {
		return ProcessStateUnknown
	}
	return mapSystemdUnitState(loadState, activeState)
}

// mapWindowsSCMState maps a Windows SCM service state onto the process_state vocabulary.
func mapWindowsSCMState(state uint32) string {
	switch state {
	case scmRunning:
		return ProcessStateRunning
	case scmStartPending:
		return ProcessStateStarting
	case scmStopPending:
		return ProcessStateStopping
	case scmStopped, scmPaused, scmPausePending, scmContinuePending:
		return ProcessStateStopped
	default:
		return ProcessStateUnknown
	}
}
