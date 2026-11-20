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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/windowsunits"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// legacySupervisors are the SCM services supervising a service when procmgr does not, by service ID.
var legacySupervisors = map[string]units.Unit{
	"ddot":     windowsunits.SCM.OTel,
	"process":  windowsunits.SCM.Process,
	"sysprobe": windowsunits.SCM.Sysprobe,
	"trace":    windowsunits.SCM.Trace,
	"security": windowsunits.SCM.Security,
	"action":   windowsunits.SCM.Action,
}

func detectLegacySupervisor(_ context.Context, service MigratableService) ManagementMode {
	legacy, ok := legacySupervisors[service.ID]
	if !ok {
		return ManagementModeNone
	}
	running, err := winutil.IsServiceRunning(legacy.Name)
	if err == nil && running {
		return ManagementModeWindowsService
	}
	return ManagementModeNone
}

func detectDaemonServiceState(_ context.Context) string {
	state, err := winutil.QueryServiceState(windowsunits.SCM.Procmgr.Name)
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
