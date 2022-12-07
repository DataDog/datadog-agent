// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

//go:build windows
// +build windows

package cpu

import (
	"fmt"
	"strconv"

	"github.com/DataDog/gohai/cpu"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

const cpuCheckName = "cpu"

// For testing purposes
var cpuInfo = cpu.GetCpuInfo

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU    float64
	pdhQuery *pdhutil.PdhQuery
	// maps metric to counter object
	counters map[string]pdhutil.PdhSingleInstanceCounter
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.cpu.num_cores", c.nbCPU, "", nil)

	// Fetch PDH query values
	err = c.pdhQuery.CollectQueryData()
	if err == nil {
		// Get values for PDH counters
		for metricname, cset := range c.counters {
			var val float64
			val, err = cset.GetValue()
			if err == nil {
				sender.Gauge(metricname, float64(val), "", nil)
			} else {
				c.Warnf("cpu.Check: Could not retrieve value for %v: %v", metricname, err)
			}
		}
	} else {
		c.Warnf("cpu.Check: Could not collect performance counter data: %v", err)
	}

	sender.Gauge("system.cpu.iowait", 0.0, "", nil)
	sender.Gauge("system.cpu.stolen", 0.0, "", nil)
	sender.Gauge("system.cpu.guest", 0.0, "", nil)
	sender.Commit()

	return nil
}

// Configure the CPU check doesn't need configuration
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("cpu.Check: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU = cpucount

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counters = map[string]pdhutil.PdhSingleInstanceCounter{
		"system.cpu.interrupt": c.pdhQuery.AddEnglishCounterInstance("Processor Information", "% Interrupt Time", "_Total"),
		"system.cpu.idle":      c.pdhQuery.AddEnglishCounterInstance("Processor Information", "% Idle Time", "_Total"),
		"system.cpu.user":      c.pdhQuery.AddEnglishCounterInstance("Processor Information", "% User Time", "_Total"),
		"system.cpu.system":    c.pdhQuery.AddEnglishCounterInstance("Processor Information", "% Privileged Time", "_Total"),
	}

	return nil
}

func cpuFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(cpuCheckName),
	}
}

func init() {
	core.RegisterCheck(cpuCheckName, cpuFactory)
}
