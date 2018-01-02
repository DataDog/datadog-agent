// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package corechecks

import (
    "fmt"
    "time"

    "github.com/DataDog/datadog-agent/pkg/collector/check"
    "github.com/DataDog/datadog-agent/pkg/aggregator"
    log "github.com/cihub/seelog"
)

// CheckBase provides default for most methods required by
// the check.Check interface to make it easier to bootstrap a
// new corecheck.
type CheckBase struct {
    checkId      string
    lastWarnings []error
}

func NewCheckBase(id string) CheckBase {
    return CheckBase{
        checkId: id,
    }
}

func (c *CheckBase) String() string {
    return c.checkId
}

// ID returns the name of the check since there should be only one instance running
func (c *CheckBase) ID() check.ID {
    return check.ID(c.String())
}

// Interval returns the scheduling time for the check
func (c *CheckBase) Interval() time.Duration {
    return check.DefaultCheckInterval
}

// GetWarnings grabs the last warnings from the sender
func (c *CheckBase) GetWarnings() []error {
    if len(c.lastWarnings) == 0 {
        return nil
    }
    w := c.lastWarnings
    c.lastWarnings = []error{}
    return w
}

// Warn will log a warning and add it to the warnings
func (c *CheckBase) warn(v ...interface{}) error {
    w := log.Warn(v)
    c.lastWarnings = append(c.lastWarnings, w)

    return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *CheckBase) warnf(format string, params ...interface{}) error {
    w := log.Warnf(format, params)
    c.lastWarnings = append(c.lastWarnings, w)

    return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *CheckBase) GetMetricStats() (map[string]int64, error) {
    sender, err := aggregator.GetSender(c.ID())
    if err != nil {
        return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
    }
    return sender.GetMetricStats(), nil
}
