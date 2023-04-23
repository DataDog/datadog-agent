// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	// Register compliance resources
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/audit"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/command"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/docker"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/file"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/group"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/kubeapiserver"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/process"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/xccdf"
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

	checkable Checkable

	eventNotify eventNotify
}

func (c *complianceCheck) Stop() {
}

func (c *complianceCheck) Cancel() {
}

func (c *complianceCheck) String() string {
	return compliance.CheckName(c.ruleID, c.description)
}

func (c *complianceCheck) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	return nil
}

func (c *complianceCheck) Interval() time.Duration {
	return c.interval
}

func (c *complianceCheck) ID() check.ID {
	return check.ID(c.ruleID)
}

func (c *complianceCheck) InitConfig() string {
	return ""
}

func (c *complianceCheck) InstanceConfig() string {
	return ""
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

type resourceQuadID struct {
	AgentRuleID      string
	AgentFrameworkID string
	ResourceID       string
	ResourceType     string
}

func (c *complianceCheck) Run() error {
	if !c.IsLeader() {
		return nil
	}

	var err error

	reports := c.checkable.Check(c)
	sort.Stable(reports)

	resourceQuadIDs := make(map[resourceQuadID]bool)

	for _, report := range reports {
		ruleID := c.ruleID

		// NOTE(pierre): XCCDF checks report multiple rules in one batch. This does not really
		// cope with the current architecture where each check report for one agent rule. For now we
		// use the Data map to pass this information and rewrite the agent_rule_id in place.
		if xccdfRuleID, ok := report.Data["xccdf_rule_id"].(string); ok {
			ruleID = xccdfRuleID
		}

		if report.Error != nil {
			log.Debugf("%s: check run failed: %v", ruleID, report.Error)
			if !report.UserProvidedError {
				err = report.Error
			}
		}

		data, result := reportToEventData(report)

		resource := c.reportToResource(report)

		quadID := resourceQuadID{
			AgentRuleID:      ruleID,
			AgentFrameworkID: c.suiteMeta.Framework,
			ResourceID:       resource.ID,
			ResourceType:     resource.Type,
		}

		// skip if we already sent an event with this quad ID
		if _, present := resourceQuadIDs[quadID]; present {
			continue
		}
		resourceQuadIDs[quadID] = true

		evaluator := report.Evaluator
		if evaluator == "" {
			evaluator = "legacy"
		}

		e := &event.Event{
			AgentRuleID:      quadID.AgentRuleID,
			AgentFrameworkID: quadID.AgentFrameworkID,
			AgentVersion:     version.AgentVersion,
			ResourceID:       quadID.ResourceID,
			ResourceType:     quadID.ResourceType,
			Result:           result,
			Data:             data,
			Evaluator:        evaluator,
			ExpireAt:         c.computeExpireAt(),
		}

		log.Debugf("%s: reporting [%s] [%s] [%s]", ruleID, e.Result, e.ResourceID, e.ResourceType)

		c.Reporter().Report(e)
		if c.eventNotify != nil {
			c.eventNotify(c.ruleID, e)
		}

		if client := c.StatsdClient(); client != nil {
			tags := []string{
				"rule_id:" + e.AgentRuleID,
				"rule_result:" + e.Result,
				"agent_version:" + e.AgentVersion,
			}
			if err := client.Gauge(metrics.MetricChecksStatuses, 1, tags, 1.0); err != nil {
				log.Errorf("failed to send checks metric: %v", err)
			}
		}
	}

	return err
}

// ExpireAtIntervalFactor represents the amount of intervals between a check and its expiration
const ExpireAtIntervalFactor = 3

func (c *complianceCheck) computeExpireAt() time.Time {
	base := time.Now().Add(c.interval * ExpireAtIntervalFactor).UTC()
	// remove sub-second precision
	truncated := base.Truncate(1 * time.Second)
	return truncated
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

func (c *complianceCheck) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	return nil, nil
}
