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
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const (
	CheckName                    = "battery"
	defaultMinCollectionInterval = 300
)

// Check is the battery check
type Check struct {
	core.CheckBase
}

// BatteryInfo contains battery information
type BatteryInfo struct {
	DesignedCapacity    uint32
	FullChargedCapacity uint32
	CycleCount          uint32
	CurrentCharge       uint32
	HasData             bool
}

var QueryBatteryInfo = queryBatteryInfo

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
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}

	return err
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	info, err := QueryBatteryInfo()
	if err != nil {
		return err
	}

	sender.Gauge("system.battery.designed_capacity", float64(info.DesignedCapacity), "", nil)
	sender.Gauge("system.battery.maximum_capacity", float64(info.FullChargedCapacity), "", nil)
	sender.Gauge("system.battery.cycle_count", float64(info.CycleCount), "", nil)
	sender.Gauge("system.battery.current_charge", float64(info.CurrentCharge), "", nil)

	sender.Commit()
	return nil
}
