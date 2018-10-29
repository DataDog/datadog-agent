// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"fmt"
	"runtime"

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
	cache_bytes *pdhutil.PdhCounterSet
	committed_bytes *pdhutil.PdhCounterSet
	paged_bytes *pdhutil.PdhCounterSet
	non_paged_bytes *pdhutil.PdhCounterSet
}

const mbSize float64 = 1024 * 1024

func (c *MemoryCheck) Configure(data integration.Data, initConfig integration.Data) (err error) {
	c.cache_bytes, err = pdhutil.GetCounterSet("Memory", "Cache Bytes", nil, nil)
	if err != nil {
		c.committed_bytes, err = pdhutil.GetCounterSet("Memory", "Committed Bytes", nil, nil)
		if err != nil {
			c.paged_bytes, err = pdhutil.GetCounterSet("Memory", "Pool Paged Bytes", nil, nil)
			if err != nil {
				c.non_paged_bytes, err = pdhutil.GetCounterSet("Memory", "Pool Nonpaged Bytes", nil, nil)
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

	val, err := c.cache_bytes.GetSingleValue()
	if err != nil {
		sender.Gauge("system.mem.cached", float64(val)/mbSize, "", nil)
	} else {
		log.Warnf("Could not retrieve value for system.mem.cached")
	}

	val, err := c.committed_bytes.GetSingleValue()
	if err != nil {
		sender.Gauge("system.mem.committed", float64(val)/mbSize, "", nil)
	} else {
		log.Warnf("Could not retrieve value for system.mem.committed")
	}

	val, err := c.paged_bytes.GetSingleValue()
	if err != nil {
		sender.Gauge("system.mem.paged", float64(val)/mbSize, "", nil)
	} else {
		log.Warnf("Could not retrieve value for system.mem.paged")
	}

	val, err := c.non_paged_bytes.GetSingleValue()
	if err != nil {
		sender.Gauge("system.mem.nonpaged", float64(val)/mbSize, "", nil)
	} else {
		log.Warnf("Could not retrieve value for system.mem.nonpaged")
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
