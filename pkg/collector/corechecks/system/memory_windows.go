// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var virtualMemory = winutil.VirtualMemory
var swapMemory = winutil.SwapMemory
var runtimeOS = runtime.GOOS

// MemoryCheck doesn't need additional fields
type MemoryCheck struct {
	core.CheckBase
	cacheBytes     *pdhutil.PdhCounterSet
	committedBytes *pdhutil.PdhCounterSet
	pagedBytes     *pdhutil.PdhCounterSet
	nonpagedBytes  *pdhutil.PdhCounterSet
}

const mbSize float64 = 1024 * 1024

// Configure handles initial configuration/initialization of the check
func (c *MemoryCheck) Configure(data integration.Data, initConfig integration.Data) (err error) {
	c.cacheBytes, err = pdhutil.GetCounterSet("Memory", "Cache Bytes", "", nil)
	if err == nil {
		c.committedBytes, err = pdhutil.GetCounterSet("Memory", "Committed Bytes", "", nil)
		if err == nil {
			c.pagedBytes, err = pdhutil.GetCounterSet("Memory", "Pool Paged Bytes", "", nil)
			if err == nil {
				c.nonpagedBytes, err = pdhutil.GetCounterSet("Memory", "Pool Nonpaged Bytes", "", nil)
			}
		}
	}
	return err
}

// Run executes the check
func (c *MemoryCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	var val float64
	if c.cacheBytes != nil {
		val, err = c.cacheBytes.GetSingleValue()
		if err == nil {
			sender.Gauge("system.mem.cached", float64(val)/mbSize, "", nil)
		} else {
			log.Warnf("Could not retrieve value for system.mem.cached %v", err)
		}
	}

	if c.committedBytes != nil {
		val, err = c.committedBytes.GetSingleValue()
		if err == nil {
			sender.Gauge("system.mem.committed", float64(val)/mbSize, "", nil)
		} else {
			log.Warnf("Could not retrieve value for system.mem.committed %v", err)
		}
	}

	if c.pagedBytes != nil {
		val, err = c.pagedBytes.GetSingleValue()
		if err == nil {
			sender.Gauge("system.mem.paged", float64(val)/mbSize, "", nil)
		} else {
			log.Warnf("Could not retrieve value for system.mem.paged %v", err)
		}
	}

	if c.nonpagedBytes != nil {
		val, err = c.nonpagedBytes.GetSingleValue()
		if err == nil {
			sender.Gauge("system.mem.nonpaged", float64(val)/mbSize, "", nil)
		} else {
			log.Warnf("Could not retrieve value for system.mem.nonpaged %v", err)
		}
	}
	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(100-v.UsedPercent)/100, "", nil)
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
