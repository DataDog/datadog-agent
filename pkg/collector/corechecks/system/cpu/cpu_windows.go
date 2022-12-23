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
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

var (
	modkernel32 = windows.NewLazyDLL("kernel32.dll")

	procGetSystemTimes = modkernel32.NewProc("GetSystemTimes")
)

const cpuCheckName = "cpu"

// For testing purposes
var cpuInfo = cpu.GetCpuInfo

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU             float64
	interruptsCounter *pdhutil.PdhSingleInstanceCounterSet
	idleCounter       *pdhutil.PdhSingleInstanceCounterSet
	userCounter       *pdhutil.PdhSingleInstanceCounterSet
	privilegedCounter *pdhutil.PdhSingleInstanceCounterSet
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.cpu.num_cores", c.nbCPU, "", nil)

	var val float64

	// counter ("Processor Information", "% Interrupt Time")
	if c.interruptsCounter == nil {
		c.interruptsCounter, err = getProcessorPDHCounter("% Interrupt Time", "_Total")
	}
	if c.interruptsCounter != nil {
		val, err = c.interruptsCounter.GetValue()
	}
	if err != nil {
		c.Warnf("cpu.Check: could not establish interrupt time counter: %v", err)
	} else {
		sender.Gauge("system.cpu.interrupt", float64(val), "", nil)
	}

	// counter ("Processor Information", "% Idle Time")
	if c.idleCounter == nil {
		c.idleCounter, err = getProcessorPDHCounter("% Idle Time", "_Total")
	}
	if c.idleCounter != nil {
		val, err = c.idleCounter.GetValue()
	}
	if err != nil {
		c.Warnf("cpu.Check: could not establish idle time counter: %v", err)
	} else {
		sender.Gauge("system.cpu.idle", float64(val), "", nil)
	}

	// counter ("Processor Information", "% User Time")
	if c.userCounter == nil {
		c.userCounter, err = getProcessorPDHCounter("% User Time", "_Total")
	}
	if c.userCounter != nil{
		val, err = c.userCounter.GetValue()
	}
	if err != nil {
		c.Warnf("cpu.Check: could not establish user time counter: %v", err)
	} else {
		sender.Gauge("system.cpu.user", float64(val), "", nil)
	}

	// counter ("Processor Information", "% Privileged Time")
	if c.privilegedCounter == nil {
		c.privilegedCounter, err = getProcessorPDHCounter("% Privileged Time", "_Total")
	}
	if c.privilegedCounter != nil {
		val, err = c.privilegedCounter.GetValue()
	}
	if err != nil {
		c.Warnf("cpu.Check: could not establish system time counter: %v", err)
	} else {
		sender.Gauge("system.cpu.system", float64(val), "", nil)
	}

	sender.Gauge("system.cpu.iowait", 0.0, "", nil)
	sender.Gauge("system.cpu.stolen", 0.0, "", nil)
	sender.Gauge("system.cpu.guest", 0.0, "", nil)
	sender.Commit()

	return nil
}

// Configure the PDH counter according to the running environment.
// On machines with more than 1 NUMA node, it uses "Processor Information",
// otherwise it uses "Processor" (e.g. in containers).
// Note we use "processor information" instead of "processor" because on multi-processor machines the later only gives
// you visibility about other applications running on the same processor as you
func getProcessorPDHCounter(counterName, instance string) (*pdhutil.PdhSingleInstanceCounterSet, error) {
	counter, err := pdhutil.GetEnglishCounterInstance("Processor Information", counterName, instance)
	if err != nil {
		counter, err = pdhutil.GetEnglishCounterInstance("Processor", counterName, instance)
	}
	return counter, err
}

// Configure the CPU check doesn't need configuration
func (c *Check) Configure(data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(initConfig, data, source); err != nil {
		return err
	}

	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("cpu.Check: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU = cpucount

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
