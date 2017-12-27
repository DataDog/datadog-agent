// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package system

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var times = cpu.Times
var cpuInfo = cpu.Info

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	nbCPU        float64
	lastNbCycle  float64
	lastWarnings []error
	lastTimes    cpu.TimesStat
}

func (c *CPUCheck) String() string {
	return "cpu"
}

// Run executes the check
func (c *CPUCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	cpuTimes, err := times(false)
	if err != nil {
		log.Errorf("system.CPUCheck: could not retrieve cpu stats: %s", err)
		return err
	} else if len(cpuTimes) < 1 {
		errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
		log.Errorf("system.CPUCheck: %s", errEmpty)
		return errEmpty
	}
	t := cpuTimes[0]

	nbCycle := t.Total() / c.nbCPU

	if c.lastNbCycle != 0 {
		// gopsutil return the sum of every CPU
		toPercent := 100 / (nbCycle - c.lastNbCycle)

		user := ((t.User + t.Nice) - (c.lastTimes.User + c.lastTimes.Nice)) / c.nbCPU
		system := ((t.System + t.Irq + t.Softirq) - (c.lastTimes.System + c.lastTimes.Irq + c.lastTimes.Softirq)) / c.nbCPU
		iowait := (t.Iowait - c.lastTimes.Iowait) / c.nbCPU
		idle := (t.Idle - c.lastTimes.Idle) / c.nbCPU
		stolen := (t.Steal - c.lastTimes.Steal) / c.nbCPU
		guest := (t.Guest - c.lastTimes.Guest) / c.nbCPU

		sender.Gauge("system.cpu.user", user*toPercent, "", nil)
		sender.Gauge("system.cpu.system", system*toPercent, "", nil)
		sender.Gauge("system.cpu.iowait", iowait*toPercent, "", nil)
		sender.Gauge("system.cpu.idle", idle*toPercent, "", nil)
		sender.Gauge("system.cpu.stolen", stolen*toPercent, "", nil)
		sender.Gauge("system.cpu.guest", guest*toPercent, "", nil)
		sender.Commit()
	}

	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *CPUCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.CPUCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += float64(i.Cores)
	}
	return nil
}

// Interval returns the scheduling time for the check
func (c *CPUCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *CPUCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *CPUCheck) Stop() {}

// GetWarnings grabs the last warnings from the sender
func (c *CPUCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *CPUCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *CPUCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *CPUCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func cpuFactory() check.Check {
	return &CPUCheck{}
}

func init() {
	core.RegisterCheck("cpu", cpuFactory)
}
