// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package battery implements the battery check.
package battery

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const (
	CheckName                    = "battery"
	defaultMinCollectionInterval = 300
)

// getBatteryInfoFunc is a mockable function variable for retrieving battery information
var getBatteryInfoFunc = getBatteryInfo

// hasBatteryAvailableFunc is a mockable function variable for checking if a battery is available
var hasBatteryAvailableFunc = hasBatteryAvailable

// batteryInfo contains normalized battery information across platforms
type batteryInfo struct {
	cycleCount         option.Option[float64] // battery cycle count
	designedCapacity   option.Option[float64] // mWh
	maximumCapacity    option.Option[float64] // mWh
	maximumCapacityPct option.Option[float64] // percentage (0-100)
	currentChargePct   option.Option[float64] // percentage (0-100)
	voltage            option.Option[float64] // mV
	chargeRate         option.Option[float64] // mW (positive = charging, negative = discharging)
	powerState         []string               // power state tags
}

// Check is the battery check
type Check struct {
	core.CheckBase
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}

// Configure handles initial configuration/initialization of the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}

	// Check if battery is available before enabling the check
	log.Debugf("Checking if battery is available")
	hasBattery, err := hasBatteryAvailableFunc()
	if err != nil {
		return err
	}
	if !hasBattery {
		log.Infof("No battery available, skipping check")
		return check.ErrSkipCheckInstance
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	info, err := getBatteryInfoFunc()
	if err != nil {
		return err
	}

	if v, ok := info.designedCapacity.Get(); ok {
		sender.Gauge("system.battery.designed_capacity", v, "", nil)
	}
	if v, ok := info.maximumCapacity.Get(); ok {
		sender.Gauge("system.battery.maximum_capacity", v, "", nil)
	}
	if v, ok := info.maximumCapacityPct.Get(); ok {
		sender.Gauge("system.battery.maximum_capacity_pct", v, "", nil)
	}
	if v, ok := info.cycleCount.Get(); ok {
		sender.Gauge("system.battery.cycle_count", v, "", nil)
	}
	if v, ok := info.currentChargePct.Get(); ok {
		sender.Gauge("system.battery.current_charge_pct", v, "", nil)
	}
	if v, ok := info.voltage.Get(); ok {
		sender.Gauge("system.battery.voltage", v, "", nil)
	}
	if v, ok := info.chargeRate.Get(); ok {
		sender.Gauge("system.battery.charge_rate", v, "", nil)
	}

	if len(info.powerState) > 0 {
		sender.Gauge("system.battery.power_state", 1, "", info.powerState)
	} else {
		sender.Gauge("system.battery.power_state", 0, "", []string{"power_state:unknown"})
	}

	sender.Commit()
	return nil
}
