// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package memory

import (
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v3/mem"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// For testing purpose
var virtualMemory = mem.VirtualMemory
var swapMemory = mem.SwapMemory
var runtimeOS = runtime.GOOS

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
}

const mbSize float64 = 1024 * 1024

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(v.Available)/float64(v.Total), "", nil)

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
		log.Errorf("memory.Check: could not retrieve virtual memory stats: %s", errVirt)
	}

	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
		sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
		sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
		sender.Gauge("system.swap.pct_free", (100-s.UsedPercent)/100, "", nil)
		sender.Rate("system.swap.swap_in", float64(s.Sin)/mbSize, "", nil)
		sender.Rate("system.swap.swap_out", float64(s.Sout)/mbSize, "", nil)
	} else {
		log.Errorf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	if errVirt != nil && errSwap != nil {
		return fmt.Errorf("failed to gather any memory information")
	}

	sender.Commit()
	return nil
}

func (c *Check) linuxSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	sender.Gauge("system.mem.buffered", float64(v.Buffers)/mbSize, "", nil)
	sender.Gauge("system.mem.shared", float64(v.Shared)/mbSize, "", nil)
	sender.Gauge("system.mem.slab", float64(v.Slab)/mbSize, "", nil)
	sender.Gauge("system.mem.slab_reclaimable", float64(v.Sreclaimable)/mbSize, "", nil)
	sender.Gauge("system.mem.page_tables", float64(v.PageTables)/mbSize, "", nil)
	sender.Gauge("system.mem.commit_limit", float64(v.CommitLimit)/mbSize, "", nil)
	sender.Gauge("system.mem.committed_as", float64(v.CommittedAS)/mbSize, "", nil)
	sender.Gauge("system.swap.cached", float64(v.SwapCached)/mbSize, "", nil)
	return nil
}

func (c *Check) freebsdSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	return nil
}
