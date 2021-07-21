// +build !windows

package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/api/plugin"
)

const cpuCheckName = "cpu_plugin"
const version = "0.0.1"

// For testing purpose
var times = cpu.Times
var cpuInfo = cpu.Info

// CPUCheck holds all the info about this plugin
type CPUCheck struct {
	// CheckBase
	checkName      string
	checkID        string
	latestWarnings []error
	checkInterval  time.Duration
	source         string
	telemetry      bool

	nbCPU       float64
	lastNbCycle float64
	lastTimes   cpu.TimesStat
}

// Run runs the check
func (c *CPUCheck) Run(sender plugin.Sender) error {
	if sender == nil {
		return errors.New("system.plugin.CPUCheck: Sender for the check invocation was empty")
	}

	cpuTimes, err := times(false)
	if err != nil {
		log.Printf("system.plugin.CPUCheck: could not retrieve cpu stats: %s", err)
		return err
	} else if len(cpuTimes) < 1 {
		errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
		log.Printf("system.plugin.CPUCheck: %s", errEmpty)
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

		sender.Gauge("system.cpu_plugin.user", user*toPercent, "", nil)
		sender.Gauge("system.cpu_plugin.system", system*toPercent, "", nil)
		sender.Gauge("system.cpu_plugin.iowait", iowait*toPercent, "", nil)
		sender.Gauge("system.cpu_plugin.idle", idle*toPercent, "", nil)
		sender.Gauge("system.cpu_plugin.stolen", stolen*toPercent, "", nil)
		sender.Gauge("system.cpu_plugin.guest", guest*toPercent, "", nil)
	}

	sender.Gauge("system.cpu_plugin.num_cores", c.nbCPU, "", nil)
	sender.Commit()

	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

// FIXME: Do something here with config stuff
func (c *CPUCheck) commonConfigure(data []byte, source string) error {
	c.source = source
	return nil
}

// Configure the CPU check
func (c *CPUCheck) Configure(data []byte, initConfig []byte, source string) error {
	err := c.commonConfigure(data, source)
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
		return fmt.Errorf("system.plugin.CPUCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += float64(i.Cores)
	}
	return nil
}

// Canned API below

// Cancel cancels the check. Cancel is called when the check is unscheduled:
// - unlike Stop, it is called even if the check is not running when it's unscheduled
// - if the check is running, Cancel is called after Stop and may be called before the call to Stop completes
func (c *CPUCheck) Cancel() {}

// ConfigSource returns the configuration source of the check
func (c *CPUCheck) ConfigSource() string { return c.source }

// GetMetricStats gets metric stats from the sender
func (c *CPUCheck) GetMetricStats() (map[string]int64, error) {
	return map[string]int64{}, nil
}

// GetWarnings returns the last warning registered by the check
func (c *CPUCheck) GetWarnings() []error { return []error{} }

// ID provides a unique identifier for every check instance
func (c *CPUCheck) ID() string { return c.checkID }

// Interval returns the interval time for the check
func (c *CPUCheck) Interval() time.Duration { return 5 * time.Second }

// IsTelemetryEnabled returns if telemetry is enabled for this check
func (c *CPUCheck) IsTelemetryEnabled() bool { return false }

// Stop stops the check if it's running
func (c *CPUCheck) Stop() {}

// String provides a printable version of the check name
func (c *CPUCheck) String() string { return fmt.Sprintf("plugin: %s", c.checkID) }

// Version returns the version of the check if available
func (c *CPUCheck) Version() string { return version }

// NewCheck creates a new instance of a CPUCheck
func NewCheck() plugin.Check {
	return &CPUCheck{
		checkID: cpuCheckName,
	}
}

// PluginInfo contains the plugin main metadata
func PluginInfo() map[string]string {
	return map[string]string{
		"id":      cpuCheckName,
		"version": version,
	}
}
