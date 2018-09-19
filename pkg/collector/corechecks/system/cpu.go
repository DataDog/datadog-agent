// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build !windows

package system

import (
	"fmt"

	"github.com/shirou/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const cpuCheckName = "cpu"

// For testing purpose
var times = cpu.Times
var cpuInfo = cpu.Info

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	core.CheckBase
	nbCPU       float64
	lastNbCycle float64
	lastTimes   cpu.TimesStat
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

// Configure the CPU check
func (c *CPUCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.CommonConfigure(data)
	if err != nil {
		return err
	}
	// NOTE: This runs before the python checks, so we should be good, but cpuInfo()
	//       on windows initializes COM to the multithreaded model. Therefore,
	//       if a python check has run on this native windows thread prior and
	//       CoInitialized() the thread to a different model (ie. single-threaded)
	//       This will cause cpuInfo() to fail.
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.CPUCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += float64(i.Cores)
	}
	return nil
}

func cpuFactory() check.Check {
	return &CPUCheck{
		CheckBase: core.NewCheckBase(cpuCheckName),
	}
}

func init() {
	core.RegisterCheck(cpuCheckName, cpuFactory)
}
