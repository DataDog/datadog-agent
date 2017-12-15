// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package system

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/load"
)

// For testing purpose
var loadAvg = load.Avg

// LoadCheck doesn't need additional fields
type LoadCheck struct {
	lastWarnings []error
	nbCPU        int32
}

func (c *LoadCheck) String() string {
	return "load"
}

// Run executes the check
func (c *LoadCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	avg, err := loadAvg()
	if err != nil {
		log.Errorf("system.LoadCheck: could not retrieve load stats: %s", err)
		return err
	}

	sender.Gauge("system.load.1", avg.Load1, "", nil)
	sender.Gauge("system.load.5", avg.Load5, "", nil)
	sender.Gauge("system.load.15", avg.Load15, "", nil)
	cpus := float64(c.nbCPU)
	sender.Gauge("system.load.norm.1", avg.Load1/cpus, "", nil)
	sender.Gauge("system.load.norm.5", avg.Load5/cpus, "", nil)
	sender.Gauge("system.load.norm.15", avg.Load15/cpus, "", nil)
	sender.Commit()

	return nil
}

// [TODO] The troubleshoot command does nothing for the Load check
func (c *LoadCheck) Troubleshoot() error {
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *LoadCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.LoadCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += i.Cores
	}
	return nil
}

// Interval returns the scheduling time for the check
func (c *LoadCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *LoadCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *LoadCheck) Stop() {}

// GetWarnings grabs the last warnings from the sender
func (c *LoadCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *LoadCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *LoadCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *LoadCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func loadFactory() check.Check {
	return &LoadCheck{}
}

func init() {
	core.RegisterCheck("load", loadFactory)
}
