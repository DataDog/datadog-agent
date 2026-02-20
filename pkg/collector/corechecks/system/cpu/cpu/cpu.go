// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package cpu

import (
	"errors"

	"github.com/shirou/gopsutil/v4/cpu"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the cpu check.
	CheckName = "cpu"
)

// For testing purpose
var getCPUTimes = cpu.Times
var getCPUInfo = cpu.Info
var getContextSwitches = GetContextSwitches

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	instanceConfig    cpuInstanceConfig
	lastNbCycle       float64
	lastTimes         cpu.TimesStat
	systemCPUUserTags []string
}

type cpuInstanceConfig struct {
	ReportTotalPerCPU bool `yaml:"report_total_percpu"`
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	c.reportContextSwitches(sender)
	numCores, err := c.reportCPUInfo(sender)
	if err != nil {
		return err
	}
	err = c.reportCPUMetricsPercent(sender, numCores)
	if err != nil {
		return err
	}
	err = c.reportCPUMetricsTotal(sender)
	if err != nil {
		return err
	}
	sender.Commit()
	return nil
}

func (c *Check) reportContextSwitches(sender sender.Sender) {
	ctxSwitches, err := getContextSwitches()
	if err != nil {
		log.Debugf("could not read context switches: %s", err.Error())
		// Don't return error here, we still want to collect the CPU metrics even if we could not
		// read the context switches
	} else {
		log.Debugf("getContextSwitches: %d", ctxSwitches)
		sender.MonotonicCount("system.cpu.context_switches", float64(ctxSwitches), "", nil)
	}
}

func (c *Check) reportCPUInfo(sender sender.Sender) (numCores int32, err error) {
	cpuInfo, err := getCPUInfo()
	if err != nil {
		log.Errorf("could not retrieve cpu info: %s", err.Error())
		return 0, err
	}
	log.Debugf("getCPUInfo: %s", cpuInfo)
	numCores = 0
	for _, i := range cpuInfo {
		numCores += i.Cores
	}
	sender.Gauge("system.cpu.num_cores", float64(numCores), "", nil)
	return numCores, nil
}

func (c *Check) reportCPUMetricsPercent(sender sender.Sender, numCores int32) (err error) {
	cpuTimes, err := getCPUTimes(false)
	if err != nil {
		log.Errorf("could not retrieve cpu times: %s", err.Error())
		return err
	}
	log.Debugf("getCPUTimes(false): %s", cpuTimes)
	if len(cpuTimes) == 0 {
		err = errors.New("no cpu stats retrieve (empty results)")
		log.Errorf("%s", err.Error())
		return err
	}
	t := cpuTimes[0]
	total := t.User + t.System + t.Idle + t.Nice +
		t.Iowait + t.Irq + t.Softirq + t.Steal
	log.Debugf("total: %f", total)
	nbCycle := total / float64(numCores)
	log.Debugf("nbCycle: %f", nbCycle)
	if c.lastNbCycle != 0 {
		// gopsutil return the sum of every CPU
		log.Debugf("c.lastNbCycle: %f", c.lastNbCycle)
		toPercent := 100 / (nbCycle - c.lastNbCycle)
		log.Debugf("toPercent: %f", toPercent)

		user := ((t.User + t.Nice) - (c.lastTimes.User + c.lastTimes.Nice)) / float64(numCores)
		system := ((t.System + t.Irq + t.Softirq) - (c.lastTimes.System + c.lastTimes.Irq + c.lastTimes.Softirq)) / float64(numCores)
		interrupt := ((t.Irq + t.Softirq) - (c.lastTimes.Irq + c.lastTimes.Softirq)) / float64(numCores)
		iowait := (t.Iowait - c.lastTimes.Iowait) / float64(numCores)
		idle := (t.Idle - c.lastTimes.Idle) / float64(numCores)
		stolen := (t.Steal - c.lastTimes.Steal) / float64(numCores)
		guest := (t.Guest - c.lastTimes.Guest) / float64(numCores)

		sender.Gauge("system.cpu.user", user*toPercent, "", c.systemCPUUserTags)
		sender.Gauge("system.cpu.system", system*toPercent, "", nil)
		sender.Gauge("system.cpu.interrupt", interrupt*toPercent, "", nil)
		sender.Gauge("system.cpu.iowait", iowait*toPercent, "", nil)
		sender.Gauge("system.cpu.idle", idle*toPercent, "", nil)
		sender.Gauge("system.cpu.stolen", stolen*toPercent, "", nil)
		sender.Gauge("system.cpu.guest", guest*toPercent, "", nil)
	}
	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

func (c *Check) reportCPUMetricsTotal(sender sender.Sender) (err error) {
	cpuTimes, err := getCPUTimes(c.instanceConfig.ReportTotalPerCPU)
	if err != nil {
		log.Errorf("could not retrieve cpu times: %s", err.Error())
		return err
	}
	log.Debugf("getCPUTimes(%t): %s", c.instanceConfig.ReportTotalPerCPU, cpuTimes)
	for _, t := range cpuTimes {
		tags := []string{"core:" + t.CPU}
		sender.Gauge("system.cpu.user.total", t.User, "", tags)
		sender.Gauge("system.cpu.nice.total", t.Nice, "", tags)
		sender.Gauge("system.cpu.system.total", t.System, "", tags)
		sender.Gauge("system.cpu.idle.total", t.Idle, "", tags)
		sender.Gauge("system.cpu.iowait.total", t.Iowait, "", tags)
		sender.Gauge("system.cpu.irq.total", t.Irq, "", tags)
		sender.Gauge("system.cpu.softirq.total", t.Softirq, "", tags)
		sender.Gauge("system.cpu.steal.total", t.Steal, "", tags)
		sender.Gauge("system.cpu.guest.total", t.Guest, "", tags)
		sender.Gauge("system.cpu.guestnice.total", t.GuestNice, "", tags)
	}
	return nil
}

// Configure configures the network checks
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInstance, &c.instanceConfig)
	if err != nil {
		return err
	}
	return nil
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	var systemCPUUserTags []string
	infraMode := pkgconfigsetup.Datadog().GetString("infrastructure_mode")
	if infraMode != "full" {
		systemCPUUserTags = []string{"infra_mode:" + infraMode}
	}
	return &Check{
		CheckBase:         core.NewCheckBase(CheckName),
		systemCPUUserTags: systemCPUUserTags,
	}
}
