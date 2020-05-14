// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
)

// baseCheck defines common behavior for all compliance checks
type baseCheck struct {
	id       check.ID
	interval time.Duration
	reporter compliance.Reporter

	framework    string
	version      string
	ruleID       string
	resourceType string
	resourceID   string
}

func (c *baseCheck) Stop() {
}

func (c *baseCheck) String() string {
	return ""
}

func (c *baseCheck) Configure(config, initConfig integration.Data, source string) error {
	return nil
}

func (c *baseCheck) Interval() time.Duration {
	return c.interval
}

func (c *baseCheck) ID() check.ID {
	return c.id
}

func (c *baseCheck) GetWarnings() []error {
	return nil
}

func (c *baseCheck) GetMetricStats() (map[string]int64, error) {
	return nil, nil
}

func (c *baseCheck) Version() string {
	return ""
}

func (c *baseCheck) ConfigSource() string {
	return ""
}

func (c *baseCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *baseCheck) report(tags []string, kv compliance.KV) {
	event := &compliance.RuleEvent{
		RuleID:       c.ruleID,
		Framework:    c.framework,
		Version:      c.version,
		ResourceID:   c.resourceID,
		ResourceType: c.resourceType,
		Tags:         tags,
		Data:         kv,
	}
	c.reporter.Report(event)
}
