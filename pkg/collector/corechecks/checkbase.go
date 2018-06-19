// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package corechecks

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// CheckBase provides default implementations for most of the check.Check
// interface to make it easier to bootstrap a new corecheck.
//
// To use it, you need to embed it in your check struct, by calling
// NewCheckBase() in your factory, plus:
// - long-running checks must override Stop() and Interval()
// - checks supporting multiple instances must call BuildID() from
// their Config() method
//
// Integration warnings are handled via the Warn and Warnf methods
// that forward the warning to the logger and send the warning to
// the collector for display in the status page and the web UI.
type CheckBase struct {
	checkName      string
	checkID        check.ID
	latestWarnings []error
}

// NewCheckBase returns a check base struct with a given check name
func NewCheckBase(name string) CheckBase {
	return CheckBase{
		checkName: name,
		checkID:   check.ID(name),
	}
}

// BuildID is to be called by the check's Config() method to generate
// the unique check ID.
func (c *CheckBase) BuildID(instance, initConfig integration.Data) {
	c.checkID = check.BuildID(c.checkName, instance, initConfig)
}

// Warn sends an integration warning to logs + agent status.
func (c *CheckBase) Warn(v ...interface{}) error {
	w := log.Warn(v)
	c.latestWarnings = append(c.latestWarnings, w)

	return w
}

// Warnf sends an integration warning to logs + agent status.
func (c *CheckBase) Warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.latestWarnings = append(c.latestWarnings, w)

	return w
}

// Stop does nothing by default, you need to implement it in
// long-running checks (persisting after Run() exits)
func (c *CheckBase) Stop() {}

// Interval returns the scheduling time for the check.
// Long-running checks should override to return 0.
func (c *CheckBase) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// String returns the name of the check, the same for every instance
func (c *CheckBase) String() string {
	return c.checkName
}

// Version returns an empty string as Go check can't be updated independently
// from the agent
func (c *CheckBase) Version() string {
	return ""
}

// ID returns a unique ID for that check instance
//
// For checks that only support one instance, the default value is
// the check name. Regular checks must call BuildID() from Config()
// to build their ID.
func (c *CheckBase) ID() check.ID {
	return c.checkID
}

// GetWarnings grabs the latest integration warnings for the check.
func (c *CheckBase) GetWarnings() []error {
	if len(c.latestWarnings) == 0 {
		return nil
	}
	w := c.latestWarnings
	c.latestWarnings = []error{}
	return w
}

// GetMetricStats returns the stats from the last run of the check.
func (c *CheckBase) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve a sender: %v", err)
	}
	return sender.GetMetricStats(), nil
}
