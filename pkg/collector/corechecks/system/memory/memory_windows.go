// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package memory

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

// For testing purpose
var virtualMemory = winutil.VirtualMemory

var (
	swapMemory = winutil.SwapMemory
	pageMemory = winutil.PagefileMemory
)

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	pdhQuery *pdhutil.PdhQuery
	// maps metric to counter object
	counters map[string]pdhutil.PdhSingleInstanceCounter
}

const mbSize float64 = 1024 * 1024

// Configure handles initial configuration/initialization of the check
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counters = map[string]pdhutil.PdhSingleInstanceCounter{
		"system.mem.cached":    c.pdhQuery.AddEnglishSingleInstanceCounter("Memory", "Cache Bytes"),
		"system.mem.committed": c.pdhQuery.AddEnglishSingleInstanceCounter("Memory", "Committed Bytes"),
		"system.mem.paged":     c.pdhQuery.AddEnglishSingleInstanceCounter("Memory", "Pool Paged Bytes"),
		"system.mem.nonpaged":  c.pdhQuery.AddEnglishSingleInstanceCounter("Memory", "Pool Nonpaged Bytes"),
	}

	return err
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Fetch PDH query values
	err = c.pdhQuery.CollectQueryData()
	if err == nil {
		// Get values for PDH counters
		for metricname, counter := range c.counters {
			var val float64
			val, err = counter.GetValue()
			if err == nil {
				sender.Gauge(metricname, val/mbSize, "", nil)
			} else {
				c.Warnf("memory.Check: Could not retrieve value for %v: %v", metricname, err)
			}
		}
	} else {
		c.Warnf("memory.Check: Could not collect performance counter data: %v", err)
	}

	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(100-v.UsedPercent)/100, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve virtual memory stats: %s", errVirt)
	}

	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
		sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
		sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
		sender.Gauge("system.swap.pct_free", float64(100-s.UsedPercent)/100, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	p, errPage := pageMemory()
	if errPage == nil {
		sender.Gauge("system.mem.pagefile.pct_free", float64(100-p.UsedPercent)/100, "", nil)
		sender.Gauge("system.mem.pagefile.total", float64(p.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.pagefile.free", float64(p.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pagefile.used", float64(p.Used)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	sender.Commit()
	return nil
}
