// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// complianceCheck implements a compliance check
type complianceCheck struct {
	env.Env

	name     string
	ruleID   string
	interval time.Duration

	framework    string
	suiteName    string
	version      string
	resourceType string
	resourceID   string

	configError error

	checkable checkable
}

func (c *complianceCheck) Stop() {
}

func (c *complianceCheck) String() string {
	return c.name
}

func (c *complianceCheck) Configure(config, initConfig integration.Data, source string) error {
	return nil
}

func (c *complianceCheck) Interval() time.Duration {
	return c.interval
}

func (c *complianceCheck) ID() check.ID {
	return check.ID(c.ruleID)
}

func (c *complianceCheck) GetWarnings() []error {
	return nil
}

func (c *complianceCheck) GetMetricStats() (map[string]int64, error) {
	return nil, nil
}

func (c *complianceCheck) Version() string {
	return c.version
}

func (c *complianceCheck) ConfigSource() string {
	return fmt.Sprintf("%s: %s", c.framework, c.suiteName)
}

func (c *complianceCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *complianceCheck) Run() error {
	if c.configError != nil {
		return c.configError
	}

	var (
		passed bool
		data   event.Data
	)

	report, err := c.checkable.check(c)
	if err == nil {
		data = report.data
		passed = report.passed
	} else {
		log.Warnf("%s: check run failed: %v", c.ruleID, err)
	}

	e := &event.Event{
		AgentRuleID:  c.ruleID,
		ResourceID:   c.resourceID,
		ResourceType: c.resourceType,
		Result:       eventResult(passed, err),
		Data:         data,
	}

	log.Debugf("%s: reporting [%s]", c.ruleID, e.Result)

	c.Reporter().Report(e)

	return err
}

func eventResult(passed bool, err error) string {
	if err != nil {
		return event.Error
	}
	if passed {
		return event.Passed
	}
	return event.Failed
}
