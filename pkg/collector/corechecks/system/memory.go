// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package system

import (
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var virtualMemory = mem.VirtualMemory
var swapMemory = mem.SwapMemory
var runtimeOS = runtime.GOOS

// MemoryCheck doesn't need additional fields
type MemoryCheck struct {
	lastWarnings []error
}

func (c *MemoryCheck) String() string {
	return "memory"
}

const mbSize float64 = 1024 * 1024

func (c *MemoryCheck) linuxSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	sender.Gauge("system.mem.shared", float64(v.Shared)/mbSize, "", nil)
	sender.Gauge("system.mem.slab", float64(v.Slab)/mbSize, "", nil)
	sender.Gauge("system.mem.page_tables", float64(v.PageTables)/mbSize, "", nil)
	sender.Gauge("system.swap.cached", float64(v.SwapCached)/mbSize, "", nil)
	return nil
}

func (c *MemoryCheck) freebsdSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	return nil
}

// Run executes the check
func (c *MemoryCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(100-v.UsedPercent)/100, "", nil)

		switch runtimeOS {
		case "linux":
			e := c.linuxSpecificVirtualMemoryCheck(v)
			if e != nil {
				return e
			}
		case "freebsd":
			e := c.freebsdSpecificVirtualMemoryCheck(v)
			if e != nil {
				return e
			}
		}
	} else {
		log.Errorf("system.MemoryCheck: could not retrieve virtual memory stats: %s", errVirt)
	}

	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
		sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
		sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
		sender.Gauge("system.swap.pct_free", float64(100-s.UsedPercent)/100, "", nil)
	} else {
		log.Errorf("system.MemoryCheck: could not retrieve swap memory stats: %s", errSwap)
	}

	if errVirt != nil && errSwap != nil {
		return fmt.Errorf("failed to gather any memory information")
	}

	sender.Commit()
	return nil
}

// [TODO] The troubleshoot command does nothing for the Memory check
func (c *MemoryCheck) Troubleshoot() error {
	return nil
}

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

// Interval returns the scheduling time for the check
func (c *MemoryCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *MemoryCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *MemoryCheck) Stop() {}

// GetWarnings grabs the last warnings from the sender
func (c *MemoryCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *MemoryCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *MemoryCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *MemoryCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func memFactory() check.Check {
	return &MemoryCheck{}
}
func init() {
	core.RegisterCheck("memory", memFactory)
}
