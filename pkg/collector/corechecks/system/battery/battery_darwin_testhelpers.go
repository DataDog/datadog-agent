// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin && test

package battery

/*
#include "battery_darwin.h"
*/
import "C"

// testCBatteryInfo is a test fixture with all fields populated
// Had to move this here because cgo is not supported in test files
var testCBatteryInfo = C.BatteryInfo{
	found:               true,
	cycleCount:          C.OptionalInt{hasValue: true, value: 500},
	designCapacity:      C.OptionalInt{hasValue: true, value: 5000},  // 5000 mAh
	appleRawMaxCapacity: C.OptionalInt{hasValue: true, value: 4500},  // 4500 mAh (90% health)
	currentCapacity:     C.OptionalInt{hasValue: true, value: 80},    // 80%
	voltage:             C.OptionalInt{hasValue: true, value: 12000}, // 12V
	instantAmperage:     C.OptionalInt{hasValue: true, value: -1000}, // discharging
	isCharging:          C.OptionalBool{hasValue: true, value: false},
	externalConnected:   C.OptionalBool{hasValue: true, value: false},
}
