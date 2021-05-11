// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

// +build windows

package cpu

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
	"github.com/DataDog/gohai/cpu"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
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
	interruptsCounter *pdhutil.PdhMultiInstanceCounterSet
	idleCounter       *pdhutil.PdhMultiInstanceCounterSet
	userCounter       *pdhutil.PdhMultiInstanceCounterSet
	privilegedCounter *pdhutil.PdhMultiInstanceCounterSet
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	sender.Gauge("system.cpu.num_cores", c.nbCPU, "", nil)

	vals, err := c.interruptsCounter.GetAllValues()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
	} else {
		val := vals["_Total"]
		sender.Gauge("system.cpu.interrupt", float64(val), "", nil)
	}

	vals, err = c.idleCounter.GetAllValues()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
	} else {
		val := vals["_Total"]
		sender.Gauge("system.cpu.idle", float64(val), "", nil)
	}

	vals, err = c.userCounter.GetAllValues()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
	} else {
		val := vals["_Total"]
		sender.Gauge("system.cpu.user", float64(val), "", nil)
	}

	vals, err = c.privilegedCounter.GetAllValues()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
	} else {
		val := vals["_Total"]
		sender.Gauge("system.cpu.system", float64(val), "", nil)
	}

	sender.Gauge("system.cpu.iowait", 0.0, "", nil)
	sender.Gauge("system.cpu.stolen", 0.0, "", nil)
	sender.Gauge("system.cpu.guest", 0.0, "", nil)
	sender.Commit()

	return nil
}

// Configure the CPU check doesn't need configuration
func (c *Check) Configure(data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(data, source); err != nil {
		return err
	}

	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("cpu.Check: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU = cpucount

	// Note we use "processor information" instead of "processor" because on multi-processor machines the later only gives
	// you visibility about other applications running on the same processor as you
	c.interruptsCounter, err = pdhutil.GetMultiInstanceCounter("Processor Information", "% Interrupt Time", &[]string{"_Total"}, nil)
	if err != nil {
		return fmt.Errorf("cpu.Check could not establish interrupt time counter %v", err)
	}
	c.idleCounter, err = pdhutil.GetMultiInstanceCounter("Processor Information", "% Idle Time", &[]string{"_Total"}, nil)
	if err != nil {
		return fmt.Errorf("cpu.Check could not establish idle time counter %v", err)
	}
	c.userCounter, err = pdhutil.GetMultiInstanceCounter("Processor Information", "% User Time", &[]string{"_Total"}, nil)
	if err != nil {
		return fmt.Errorf("cpu.Check could not establish idle time counter %v", err)
	}
	c.privilegedCounter, err = pdhutil.GetMultiInstanceCounter("Processor Information", "% Privileged Time", &[]string{"_Total"}, nil)
	if err != nil {
		return fmt.Errorf("cpu.Check could not establish system time counter %v", err)
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
