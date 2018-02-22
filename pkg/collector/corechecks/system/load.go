// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package system

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/load"
)

const loadCheckName = "load"

// For testing purpose
var loadAvg = load.Avg

// LoadCheck doesn't need additional fields
type LoadCheck struct {
	core.CheckBase
	nbCPU int32
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

// Configure the CPU check doesn't need configuration
func (c *LoadCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	// NOTE: This check is disabled on windows - so the following doesn't apply
	//       currently:
	//
	//       This runs before the python checks, so we should be good, but cpuInfo()
	//       on windows initializes COM to the multithreaded model. Therefore,
	//       if a python check has run on this native windows thread prior and
	//       CoInitialized() the thread to a different model (ie. single-threaded)
	//       This will cause cpuInfo() to fail.
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.LoadCheck: could not query CPU info - %v", err)
	}
	for _, i := range info {
		c.nbCPU += i.Cores
	}
	return nil
}

func loadFactory() check.Check {
	return &LoadCheck{
		CheckBase: core.NewCheckBase(loadCheckName),
	}
}

func init() {
	core.RegisterCheck(loadCheckName, loadFactory)
}
