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

// thermalCheck collects thermal sensor readings on Apple Silicon and Intel
// Macs via the AppleSMC utility in thermal_darwin.c.
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

// thermalReading is the darwin thermal sensor snapshot.
type thermalReading struct {
	smc smcReading
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

// getThermalReading reads every sensor via cgo and converts the result into a
// thermalReading.
func getThermalReading() thermalReading {
	info := C.getThermalInfo()
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

// Run executes the check
func (c *thermalCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	reading := getThermalReading()

	if v := reading.smc.cpu; v != nil {
		log.Debugf("thermal: SMC CPU temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.cpu", *v, "", []string{"macos", "smc", "cpu"})
	}
	if v := reading.smc.gpu; v != nil {
		log.Debugf("thermal: SMC GPU temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.gpu", *v, "", []string{"macos", "smc", "gpu"})
	}
	if v := reading.smc.ssd; v != nil {
		log.Debugf("thermal: SMC SSD temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.ssd", *v, "", []string{"macos", "smc", "ssd"})
	}
	if v := reading.smc.battery; v != nil {
		log.Debugf("thermal: SMC battery temperature: %.1f°C", *v)
		sender.Gauge("system.thermal.temperature.battery", *v, "", []string{"macos", "smc", "battery"})
	}
	if v := reading.thermalLevel; v != nil {
		log.Debugf("thermal: thermal pressure level: %d", *v)
		tags := []string{"macos", "pressure_level:" + thermalPressureLevelName(*v)}
		sender.Gauge("system.thermal.pressure_level", float64(*v), "", tags)
	}

	return nil
}
