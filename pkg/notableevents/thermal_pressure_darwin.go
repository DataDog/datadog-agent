// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

/*
#include <notify.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"

	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
)

// thermalPressureWarningLevel and thermalPressureErrorLevel are the raw
// kOSThermalPressureLevel* thresholds (Trapping, Sleeping) at which the
// collector emits a "warning" and "error" notable event respectively. Level
// 2 (Heavy) does not emit an event. See notes/NOTIFY_THERMAL_STATE.md for
// the full level table.
const (
	thermalPressureWarningLevel = 3
	thermalPressureErrorLevel   = 4
)

// thermalPressureNotificationName is the private, undocumented Darwin
// notification name posted by /usr/libexec/thermald. See
// notes/NOTIFY_THERMAL_STATE.md for background and stability caveats.
const thermalPressureNotificationName = "com.apple.system.thermalpressurelevel"

// rawThermalPressureLevel reads the current macOS thermal pressure level
// (0=Nominal, 1=Moderate, 2=Heavy, 3=Trapping, 4=Sleeping) via
// notify_register_check/notify_get_state on thermalPressureNotificationName.
// It returns ok=false if either Darwin notification call failed.
func rawThermalPressureLevel() (level int, ok bool) {
	name := C.CString(thermalPressureNotificationName)
	defer C.free(unsafe.Pointer(name))

	var token C.int
	if C.notify_register_check(name, &token) != C.NOTIFY_STATUS_OK {
		return 0, false
	}
	defer C.notify_cancel(token)

	var state C.uint64_t
	if C.notify_get_state(token, &state) != C.NOTIFY_STATUS_OK {
		return 0, false
	}

	return int(state), true
}

// thermalPressureLevelName mirrors thermalPressureLevelName in
// pkg/collector/corechecks/system/thermal/thermal_darwin.go for consistent
// terminology between the core check and this notable event.
func thermalPressureLevelName(level int) string {
	switch level {
	case 0:
		return "Nominal"
	case 1:
		return "Moderate"
	case 2:
		return "Heavy"
	case 3:
		return "Trapping"
	case 4:
		return "Sleeping"
	default:
		return "Unknown"
	}
}

// thermalPressureEventID derives a per-episode identifier from the
// transition time, hashed like other notable event identities.
func thermalPressureEventID(now time.Time) string {
	return notableeventtypes.ThermalEventIDPrefix + hashString(fmt.Sprintf("thermal-pressure-episode:%d", now.UnixNano()))
}

// thermalPressureStatus reports the event severity for level (thermalPressureWarningLevel
// or thermalPressureErrorLevel). Callers must not invoke this for any other level.
func thermalPressureStatus(level int) string {
	if level >= thermalPressureErrorLevel {
		return "error"
	}
	return "warn"
}

// newThermalPressureEvent builds the sanitized notable event for a rise into
// warning (thermalPressureWarningLevel) or error (thermalPressureErrorLevel)
// thermal pressure.
func newThermalPressureEvent(now time.Time, level int) Event {
	levelName := thermalPressureLevelName(level)
	return Event{
		ID:        thermalPressureEventID(now),
		Timestamp: now,
		EventType: "Critical Temperature",
		Title:     "macOS thermal pressure reached " + levelName,
		Message:   fmt.Sprintf("The system's thermal pressure level reached %q (level %d).", levelName, level),
		Status:    thermalPressureStatus(level),
		Custom: map[string]interface{}{
			"macos_thermal_pressure": map[string]interface{}{
				"level":      level,
				"level_name": levelName,
			},
		},
	}
}
