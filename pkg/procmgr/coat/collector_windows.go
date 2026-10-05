// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package coat

import (
	"context"
	"errors"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// windowsProcmgrServiceName is the SCM service that runs dd-procmgrd on Windows.
// Packaging currently defines a single stable name (no distinct exp/legacy SCM name).
const windowsProcmgrServiceName = "dd-procmgr-service"

func detectLegacySupervisor(_ context.Context, service MigratableService) ManagementMode {
	if service.LegacyWindowsService == "" {
		return ManagementModeNone
	}
	running, err := winutil.IsServiceRunning(service.LegacyWindowsService)
	if err == nil && running {
		return ManagementModeWindowsService
	}
	return ManagementModeNone
}

func detectDaemonServiceState(_ context.Context) string {
	state, err := winutil.QueryServiceState(windowsProcmgrServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return selectDaemonServiceState([]daemonServiceCandidateResult{{
				kind:  daemonCandidateStable,
				state: ProcessStateNotInstalled,
			}})
		}
		return selectDaemonServiceState([]daemonServiceCandidateResult{{
			kind:  daemonCandidateStable,
			state: ProcessStateUnknown,
		}})
	}
	return selectDaemonServiceState([]daemonServiceCandidateResult{{
		kind:  daemonCandidateStable,
		state: mapWindowsSCMState(uint32(state)),
	}})
}
