package system

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var times = cpu.Times
var cpuInfo = cpu.Info

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	sender      aggregator.Sender
	nbCPU       float64
	lastNbCycle float64
	lastTimes   cpu.TimesStat
}

func (c *CPUCheck) String() string {
	return "CPUCheck"
}

// Run executes the check
func (c *CPUCheck) Run() error {
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
		stolen := (t.Stolen - c.lastTimes.Stolen) / c.nbCPU
		guest := (t.Guest - c.lastTimes.Guest) / c.nbCPU

		c.sender.Gauge("system.cpu.user", user*toPercent, "", nil)
		c.sender.Gauge("system.cpu.system", system*toPercent, "", nil)
		c.sender.Gauge("system.cpu.iowait", iowait*toPercent, "", nil)
		c.sender.Gauge("system.cpu.idle", idle*toPercent, "", nil)
		c.sender.Gauge("system.cpu.stolen", stolen*toPercent, "", nil)
		c.sender.Gauge("system.cpu.guest", guest*toPercent, "", nil)
		c.sender.Commit()
	}

	c.lastNbCycle = nbCycle
	c.lastTimes = t
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *CPUCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.CPUCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += float64(i.Cores)
	}
	return nil
}

// InitSender initializes a sender
func (c *CPUCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *CPUCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *CPUCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *CPUCheck) Stop() {}

func cpuFactory() check.Check {
	return &CPUCheck{}
}

func init() {
	core.RegisterCheck("cpu", cpuFactory)
}
