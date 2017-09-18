// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package system

import (
	"fmt"
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
	lastWarnings []error
}

func (c *UptimeCheck) String() string {
	return "uptime"
}

// Run executes the check
func (c *UptimeCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	t, err := uptime()
	if err != nil {
		log.Errorf("system.UptimeCheck: could not retrieve uptime: %s", err)
		return err
	}

	sender.Gauge("system.uptime", float64(t), "", nil)
	sender.Commit()

	return nil
}

// Configure the CPU check doesn't need configuration
func (c *UptimeCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
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

// GetWarnings grabs the last warnings from the sender
func (c *UptimeCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *UptimeCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *UptimeCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *UptimeCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func uptimeFactory() check.Check {
	return &UptimeCheck{}
}

func init() {
	core.RegisterCheck("uptime", uptimeFactory)
}
