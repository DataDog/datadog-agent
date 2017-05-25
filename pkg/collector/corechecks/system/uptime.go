package system

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var uptime = host.Uptime

// UptimeCheck doesn't need additional fields
type UptimeCheck struct {
	sender aggregator.Sender
}

func (c *UptimeCheck) String() string {
	return "UptimeCheck"
}

// Run executes the check
func (c *UptimeCheck) Run() error {
	t, err := uptime()
	if err != nil {
		log.Errorf("system.UptimeCheck: could not retrieve uptime: %s", err)
		return err
	}

	c.sender.Gauge("system.uptime", float64(t), "", nil)
	c.sender.Commit()

	return nil
}

// Configure the CPU check doesn't need configuration
func (c *UptimeCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

// InitSender initializes a sender
func (c *UptimeCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *UptimeCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *UptimeCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *UptimeCheck) Stop() {}

func uptimeFactory() check.Check {
	return &UptimeCheck{}
}

func init() {
	core.RegisterCheck("uptime", uptimeFactory)
}
