// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package thermal

/*
#include "thermal_darwin.h"
*/
import "C"

// Test fixtures for the cgo conversion helpers. Had to move these here
// because cgo is not supported in test files.

// testCThermalInfoFull has every sensor and the thermal level populated.
var testCThermalInfoFull = C.ThermalInfo{
	smc: C.SmcInfo{
		cpu:     C.OptionalFloat{hasValue: true, value: 61.5},
		gpu:     C.OptionalFloat{hasValue: true, value: 48.25},
		ssd:     C.OptionalFloat{hasValue: true, value: 39.0},
		battery: C.OptionalFloat{hasValue: true, value: 31.75},
	},
	thermalLevel: C.OptionalInt{hasValue: true, value: 2},
}

// testCThermalInfoEmpty has no sensor available, as on a machine where every
// SMC key read out of range and the notification lookup failed.
var testCThermalInfoEmpty = C.ThermalInfo{
	smc: C.SmcInfo{
		cpu:     C.OptionalFloat{hasValue: false, value: 0},
		gpu:     C.OptionalFloat{hasValue: false, value: 0},
		ssd:     C.OptionalFloat{hasValue: false, value: 0},
		battery: C.OptionalFloat{hasValue: false, value: 0},
	},
	thermalLevel: C.OptionalInt{hasValue: false, value: 0},
}

// testCOptionalFloatSet / testCOptionalFloatUnset exercise optionalFloat directly.
var (
	testCOptionalFloatSet   = C.OptionalFloat{hasValue: true, value: 42.5}
	testCOptionalFloatUnset = C.OptionalFloat{hasValue: false, value: 99.0}
)

// testCOptionalIntSet / testCOptionalIntUnset exercise optionalInt directly.
var (
	testCOptionalIntSet   = C.OptionalInt{hasValue: true, value: 3}
	testCOptionalIntUnset = C.OptionalInt{hasValue: false, value: 99}
)

// convertCThermalInfo mirrors getThermalReading's conversion without calling
// into the real hardware, so tests can drive it from a fixture.
func convertCThermalInfo(info C.ThermalInfo) thermalReading {
	return thermalReading{
		smc: smcReading{
			cpu:     optionalFloat(info.smc.cpu),
			gpu:     optionalFloat(info.smc.gpu),
			ssd:     optionalFloat(info.smc.ssd),
			battery: optionalFloat(info.smc.battery),
		},
		thermalLevel: optionalInt(info.thermalLevel),
	}
}
