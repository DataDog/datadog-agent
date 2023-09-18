//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package event

import (
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	smodel "github.com/DataDog/datadog-agent/pkg/security/serializers/model"
)

const (
	policySource  = "self-test"                          // nolint: deadcode, unused
	policyVersion = "1.0.0"                              // nolint: deadcode, unused
	policyName    = "datadog-agent-cws-self-test-policy" // nolint: deadcode, unused
	ruleIDPrefix  = "datadog_agent_cws_self_test_rule"   // nolint: deadcode, unused

	// PolicyProviderType name of the self test policy provider
	PolicyProviderType = "selfTesterPolicyProvider"
)

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	events.CustomEventCommonFields
	Success    []string                           `json:"succeeded_tests"`
	Fails      []string                           `json:"failed_tests"`
	TestEvents map[string]*smodel.EventSerializer `json:"test_events"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string, testEvents map[string]*smodel.EventSerializer) (*rules.Rule, *events.CustomEvent) {
	evt := SelfTestEvent{
		Success:    success,
		Fails:      fails,
		TestEvents: testEvents,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.SelfTestRuleID, events.SelfTestRuleDesc),
		events.NewCustomEvent(model.CustomSelfTestEventType, evt)
}
