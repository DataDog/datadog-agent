// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

// Package thermal implements the thermal zone check for macOS.
package thermal

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation

#include "thermal_darwin.h"
*/
import "C"

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "thermal"

// thermalCheck collects Apple Silicon thermal sensor readings via the
// AppleSMC and IOHIDEventSystemClient utility in thermal_darwin.c.
type thermalCheck struct {
	core.CheckBase
}

// smcReading holds the AppleSMC temperatures, nil for any sensor that was
// unavailable or failed to read.
type smcReading struct {
	cpu     *float64
	gpu     *float64
	ssd     *float64
	battery *float64
}

// hidReading is the IOHIDEventSystemClient-sourced half of a thermal sensor
// snapshot, with nil for any sensor that was unavailable or failed to read.
// battery here is the "gas gauge battery" HID node, distinct from
// smcReading.battery (the TB0T-family SMC keys).
type hidReading struct {
	tdie    *float64
	nand    *float64
	battery *float64
	mtrGPU  *float64
	mtrPACC *float64
	mtrEACC *float64
	// The following are the hottest single node contributing to each
	// averaged field above, rather than the average across all matching
	// nodes.
	tdieMax    *float64
	nandMax    *float64
	batteryMax *float64
	mtrGPUMax  *float64
	mtrPACCMax *float64
	mtrEACCMax *float64
}

// thermalReading is the darwin thermal sensor snapshot.
type thermalReading struct {
	smc smcReading
	hid hidReading
	// thermalLevel is the raw macOS thermal pressure level (0=Nominal,
	// 1=Moderate, 2=Heavy, 3=Trapping, 4=Sleeping), nil if the lookup failed.
	thermalLevel *int
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &thermalCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure initializes the check
func (c *thermalCheck) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source, provider); err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	return nil
}

// optionalFloat converts a C.OptionalFloat into a *float64, nil when unset.
func optionalFloat(o C.OptionalFloat) *float64 {
	if !bool(o.hasValue) {
		return nil
	}
	v := float64(o.value)
	return &v
}

// optionalInt converts a C.OptionalInt into a *int, nil when unset.
func optionalInt(o C.OptionalInt) *int {
	if !bool(o.hasValue) {
		return nil
	}
	v := int(o.value)
	return &v
}

// thermalPressureLevelName maps a raw thermal pressure level to its name.
// "unknown" covers any value outside the 0-4 range, since the notification is
// a private API with no stability guarantee.
func thermalPressureLevelName(level int) string {
	switch level {
	case 0:
		return "nominal"
	case 1:
		return "moderate"
	case 2:
		return "heavy"
	case 3:
		return "trapping"
	case 4:
		return "sleeping"
	default:
		return "unknown"
	}
}

// getThermalReading calls into the cgo AppleSMC/IOHID utility and converts
// the result into a thermalReading.
func getThermalReading() thermalReading {
	info := C.getThermalInfo()
	return thermalReading{
		smc: smcReading{
			cpu:     optionalFloat(info.smc.cpu),
			gpu:     optionalFloat(info.smc.gpu),
			ssd:     optionalFloat(info.smc.ssd),
			battery: optionalFloat(info.smc.battery),
		},
		hid: hidReading{
			tdie:       optionalFloat(info.hid.tdie),
			nand:       optionalFloat(info.hid.nand),
			battery:    optionalFloat(info.hid.battery),
			mtrGPU:     optionalFloat(info.hid.mtrGpu),
			mtrPACC:    optionalFloat(info.hid.mtrPacc),
			mtrEACC:    optionalFloat(info.hid.mtrEacc),
			tdieMax:    optionalFloat(info.hid.tdieMax),
			nandMax:    optionalFloat(info.hid.nandMax),
			batteryMax: optionalFloat(info.hid.batteryMax),
			mtrGPUMax:  optionalFloat(info.hid.mtrGpuMax),
			mtrPACCMax: optionalFloat(info.hid.mtrPaccMax),
			mtrEACCMax: optionalFloat(info.hid.mtrEaccMax),
		},
		thermalLevel: optionalInt(info.thermalLevel),
	}
}

// Run executes the check
func (c *thermalCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	reading := getThermalReading()

	if v := reading.smc.cpu; v != nil {
		log.Infof("thermal: SMC CPU temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.cpu", *v, "", []string{"macos", "smc", "cpu"})
	}
	if v := reading.smc.gpu; v != nil {
		log.Infof("thermal: SMC GPU temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.gpu", *v, "", []string{"macos", "smc", "gpu"})
	}
	if v := reading.smc.ssd; v != nil {
		log.Infof("thermal: SMC SSD temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.ssd", *v, "", []string{"macos", "smc", "ssd"})
	}
	if v := reading.smc.battery; v != nil {
		log.Infof("thermal: SMC battery temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.battery", *v, "", []string{"macos", "smc", "battery"})
	}
	if v := reading.hid.tdie; v != nil {
		log.Infof("thermal: PMU tdie temperature: %.1f°C", *v)
	}
	if v := reading.hid.tdieMax; v != nil {
		log.Infof("thermal: PMU tdie max temperature: %.1f°C", *v)
	}
	if v := reading.hid.nand; v != nil {
		log.Infof("thermal: NAND temperature: %.1f°C", *v)
	}
	if v := reading.hid.nandMax; v != nil {
		log.Infof("thermal: NAND max temperature: %.1f°C", *v)
	}
	if v := reading.hid.battery; v != nil {
		log.Infof("thermal: gas gauge battery temperature: %.1f°C", *v)
	}
	if v := reading.hid.batteryMax; v != nil {
		log.Infof("thermal: gas gauge battery max temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrPACC; v != nil {
		log.Infof("thermal: pACC MTR temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrPACCMax; v != nil {
		log.Infof("thermal: pACC MTR max temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrEACC; v != nil {
		log.Infof("thermal: eACC MTR temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrEACCMax; v != nil {
		log.Infof("thermal: eACC MTR max temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrGPU; v != nil {
		log.Infof("thermal: GPU MTR temperature: %.1f°C", *v)
	}
	if v := reading.hid.mtrGPUMax; v != nil {
		log.Infof("thermal: GPU MTR max temperature: %.1f°C", *v)
	}

	if v := reading.thermalLevel; v != nil {
		log.Infof("thermal: thermal pressure level: %d", *v)
		tags := []string{"macos", "pressure_level:" + thermalPressureLevelName(*v)}
		sender.Gauge("system.thermal.pressure_level", float64(*v), "", tags)
	}

	return nil
}
