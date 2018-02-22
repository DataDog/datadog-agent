// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"fmt"
	"strconv"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/gohai/cpu"
	log "github.com/cihub/seelog"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

var (
	Modkernel32 = windows.NewLazyDLL("kernel32.dll")

	ProcGetSystemTimes = Modkernel32.NewProc("GetSystemTimes")
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

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	core.CheckBase
	nbCPU       float64
	lastNbCycle float64
	lastTimes   TimesStat
}

// Total returns the total number of seconds in a CPUTimesStat
func (c TimesStat) Total() float64 {
	total := c.User + c.System + c.Idle
	return total
}

// Run executes the check
func (c *CPUCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	cpuTimes, err := times()
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
		sender.Commit()
	}

	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *CPUCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	info, err := cpu.GetCpuInfo()
	if err != nil {
		return fmt.Errorf("system.CPUCheck: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU += cpucount
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
	r, _, _ := ProcGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&lpIdleTime)),
		uintptr(unsafe.Pointer(&lpKernelTime)),
		uintptr(unsafe.Pointer(&lpUserTime)))
	if r == 0 {
		return ret, windows.GetLastError()
	}

	LOT := float64(0.0000001)
	HIT := (LOT * 4294967296.0)
	idle := ((HIT * float64(lpIdleTime.DwHighDateTime)) + (LOT * float64(lpIdleTime.DwLowDateTime)))
	user := ((HIT * float64(lpUserTime.DwHighDateTime)) + (LOT * float64(lpUserTime.DwLowDateTime)))
	kernel := ((HIT * float64(lpKernelTime.DwHighDateTime)) + (LOT * float64(lpKernelTime.DwLowDateTime)))
	system := (kernel - idle)

	ret = append(ret, TimesStat{
		CPU:    "cpu-total",
		Idle:   float64(idle),
		User:   float64(user),
		System: float64(system),
	})
	return ret, nil
}
