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
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type checkKind string

// baseCheck defines common behavior for all compliance checks
type baseCheck struct {
	name     string
	id       check.ID
	kind     checkKind
	interval time.Duration
	reporter compliance.Reporter

	framework    string
	suiteName    string
	version      string
	ruleID       string
	resourceType string
	resourceID   string
}

func (c *baseCheck) Stop() {
}

func (c *baseCheck) String() string {
	return c.name
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
	return c.version
}

func (c *baseCheck) ConfigSource() string {
	return fmt.Sprintf("%s: %s", c.framework, c.suiteName)
}

func (c *baseCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *baseCheck) setStaticKV(field compliance.ReportedField, kv compliance.KVMap) bool {
	key := field.As

	if field.Value != "" {
		if key == "" {
			log.Errorf("%s: value field without an alias key - %s", c.id, field.Value)

		} else {
			kv[key] = field.Value
		}
		return true
	}
	return false
}

func (c *baseCheck) report(tags []string, kv compliance.KVMap, logMsgAndArgs ...interface{}) {
	if len(kv) == 0 {
		return
	}

	log.Debugf("%s: reporting %s:[%s]", c.ruleID, c.kind, logFromMsgAndArgs(logMsgAndArgs))

	event := &compliance.RuleEvent{
		RuleID:       c.ruleID,
		Framework:    c.framework,
		Version:      c.version,
		ResourceID:   c.resourceID,
		ResourceType: c.resourceType,
		Tags:         []string{fmt.Sprintf("check_kind:%s", c.kind)},
		Data:         kv,
	}
	c.reporter.Report(event)
}

func logFromMsgAndArgs(msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 || msgAndArgs == nil {
		return ""
	}
	if len(msgAndArgs) == 1 {
		msg := msgAndArgs[0]
		if msgAsStr, ok := msg.(string); ok {
			return msgAsStr
		}
		return fmt.Sprintf("%+v", msg)
	}
	if len(msgAndArgs) > 1 {
		return fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
	}
	return ""
}
