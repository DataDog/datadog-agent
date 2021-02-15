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
	"unsafe"

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

// For testing purpose
var times = Times

// TimesStat contains the amounts of time the CPU has spent performing different
// kinds of work. Time units are in USER_HZ or Jiffies (typically hundredths of
// a second). It is based on linux /proc/stat file.
type TimesStat struct {
	CPU    string
	User   float64
	System float64
	Idle   float64
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU       float64
	lastNbCycle float64
	lastTimes   TimesStat
	counter     *pdhutil.PdhMultiInstanceCounterSet
}

// Total returns the total number of seconds in a CPUTimesStat
func (c TimesStat) Total() float64 {
	total := c.User + c.System + c.Idle
	return total
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	cpuTimes, err := times()
	if err != nil {
		log.Errorf("cpu.Check: could not retrieve cpu stats: %s", err)
		return err
	} else if len(cpuTimes) < 1 {
		errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
		log.Errorf("cpu.Check: %s", errEmpty)
		return errEmpty
	}
	t := cpuTimes[0]

	nbCycle := t.Total() / c.nbCPU

	sender.Gauge("system.cpu.num_cores", c.nbCPU, "", nil)
	if c.lastNbCycle != 0 {
		// gopsutil return the sum of every CPU
		toPercent := 100 / (nbCycle - c.lastNbCycle)

		user := ((t.User) - (c.lastTimes.User)) / c.nbCPU
		system := ((t.System) - (c.lastTimes.System)) / c.nbCPU
		iowait := float64(0)
		idle := (t.Idle - c.lastTimes.Idle) / c.nbCPU
		stolen := float64(0)
		guest := float64(0)

		sender.Gauge("system.cpu.user", user*toPercent, "", nil)
		sender.Gauge("system.cpu.system", system*toPercent, "", nil)
		sender.Gauge("system.cpu.iowait", iowait*toPercent, "", nil)
		sender.Gauge("system.cpu.idle", idle*toPercent, "", nil)
		sender.Gauge("system.cpu.stolen", stolen*toPercent, "", nil)
		sender.Gauge("system.cpu.guest", guest*toPercent, "", nil)
	}
	vals, err := c.counter.GetAllValues()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
	} else {
		val := vals["_Total"]
		sender.Gauge("system.cpu.interrupt", float64(val), "", nil)
	}
	sender.Commit()

	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *Check) Configure(data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(data, source); err != nil {
		return err
	}

	// do nothing
	info, err := cpu.GetCpuInfo()
	if err != nil {
		return fmt.Errorf("cpu.Check: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU = cpucount

	c.counter, err = pdhutil.GetMultiInstanceCounter("Processor", "% Interrupt Time", &[]string{"_Total"}, nil)
	if err != nil {
		return fmt.Errorf("cpu.Check could not establish interrupt time counter %v", err)
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

// FILETIME is a copy of the Windows FILETIME structure
type FILETIME struct {
	DwLowDateTime  uint32
	DwHighDateTime uint32
}

// Times returns times stat per cpu and combined for all CPUs
func Times() ([]TimesStat, error) {
	var ret []TimesStat
	var lpIdleTime FILETIME
	var lpKernelTime FILETIME
	var lpUserTime FILETIME
	r, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&lpIdleTime)),
		uintptr(unsafe.Pointer(&lpKernelTime)),
		uintptr(unsafe.Pointer(&lpUserTime)))
	if r == 0 {
		return ret, windows.GetLastError()
	}

	idle := uint64(uint64(lpIdleTime.DwHighDateTime)<<32) + uint64(lpIdleTime.DwLowDateTime)
	user := uint64(uint64(lpUserTime.DwHighDateTime)<<32) + uint64(lpUserTime.DwLowDateTime)
	kernel := uint64(uint64(lpKernelTime.DwHighDateTime)<<32) + uint64(lpKernelTime.DwLowDateTime)
	system := (kernel - idle)

	ret = append(ret, TimesStat{
		CPU:    "cpu-total",
		Idle:   float64(idle),
		User:   float64(user),
		System: float64(system),
	})
	return ret, nil
}
