//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	Timestamp time.Time `json:"date"`
	Success   []string  `json:"succeeded_tests"`
	Fails     []string  `json:"failed_tests"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string) (*rules.Rule, *events.CustomEvent) {
	return events.NewCustomRule(events.SelfTestRuleID), events.NewCustomEvent(model.CustomSelfTestEventType, SelfTestEvent{
		Timestamp: time.Now(),
		Success:   success,
		Fails:     fails,
	})
}

// ReportSelfTest reports to Datadog that a self test was performed
func ReportSelfTest(sender EventSender, statsdClient statsd.ClientInterface, success []string, fails []string) {
	// send metric with number of success and fails
	tags := []string{
		fmt.Sprintf("success:%d", len(success)),
		fmt.Sprintf("fails:%d", len(fails)),
	}
	if err := statsdClient.Count(metrics.MetricSelfTest, 1, tags, 1.0); err != nil {
		log.Error(fmt.Errorf("failed to send self_test metric: %w", err))
	}

	// send the custom event with the list of succeed and failed self tests
	rule, event := NewSelfTestEvent(success, fails)
	sender.SendEvent(rule, event, func() []string { return nil }, "")
}
