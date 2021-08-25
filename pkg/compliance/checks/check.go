// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventNotify is a callback invoked when a compliance check reported an event
type eventNotify func(ruleID string, event *event.Event)

type resourceReporter func(*compliance.Report) compliance.ReportResource

// complianceCheck implements a compliance check
type complianceCheck struct {
	env.Env

	ruleID      string
	description string
	interval    time.Duration

	suiteMeta *compliance.SuiteMeta

	scope           compliance.RuleScope
	resourceHandler resourceReporter

	checkable checkable

	eventNotify eventNotify
}

func (c *complianceCheck) Stop() {
}

func (c *complianceCheck) Cancel() {
}

func (c *complianceCheck) String() string {
	return compliance.CheckName(c.ruleID, c.description)
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

func (c *complianceCheck) GetSenderStats() (check.SenderStats, error) {
	return check.NewSenderStats(), nil
}

func (c *complianceCheck) Version() string {
	return c.suiteMeta.Version
}

func (c *complianceCheck) ConfigSource() string {
	return c.suiteMeta.Source
}

func (c *complianceCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *complianceCheck) reportToResource(report *compliance.Report) compliance.ReportResource {
	if c.resourceHandler != nil {
		return c.resourceHandler(report)
	}

	return compliance.ReportResource{
		Type: string(c.scope),
		ID:   c.Hostname(),
	}
}

func (c *complianceCheck) Run() error {
	if !c.IsLeader() {
		return nil
	}

	var err error

	reports := c.checkable.check(c)
	for _, report := range reports {
		if report.Error != nil {
			log.Debugf("%s: check run failed: %v", c.ruleID, report.Error)
			err = report.Error
		}

		data, result := reportToEventData(report)

		resource := c.reportToResource(report)

		e := &event.Event{
			AgentRuleID:      c.ruleID,
			AgentFrameworkID: c.suiteMeta.Framework,
			ResourceID:       resource.ID,
			ResourceType:     resource.Type,
			Result:           result,
			Data:             data,
		}

		log.Debugf("%s: reporting [%s] [%s] [%s]", c.ruleID, e.Result, e.ResourceID, e.ResourceType)

		c.Reporter().Report(e)
		if c.eventNotify != nil {
			c.eventNotify(c.ruleID, e)
		}
	}

	return err
}

func reportToEventData(report *compliance.Report) (event.Data, string) {
	data := report.Data
	passed := report.Passed

	if report.Error != nil {
		data = event.Data{
			"error": report.Error.Error(),
		}
	}

	if report.Aggregated {
		if data == nil {
			data = event.Data{}
		}
		data["aggregated"] = true
	}

	return data, eventResult(passed, report.Error)
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
