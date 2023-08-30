//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package selftests

import (
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

const (
	policySource  = "self-test"
	policyVersion = "1.0.0"
	policyName    = "datadog-agent-cws-self-test-policy"
	ruleIDPrefix  = "datadog_agent_cws_self_test_rule"

	// PolicyProviderType name of the self test policy provider
	PolicyProviderType = "selfTesterPolicyProvider"
)

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	events.CustomEventCommonFields
	Success    []string                                `json:"succeeded_tests"`
	Fails      []string                                `json:"failed_tests"`
	TestEvents map[string]*serializers.EventSerializer `json:"test_events"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string, testEvents map[string]*serializers.EventSerializer) (*rules.Rule, *events.CustomEvent) {
	evt := SelfTestEvent{
		Success:    success,
		Fails:      fails,
		TestEvents: testEvents,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.SelfTestRuleID, events.SelfTestRuleDesc),
		events.NewCustomEvent(model.CustomSelfTestEventType, evt)
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (t *SelfTester) SetOnNewPoliciesReadyCb(cb func()) {
}

// Type return the type of this policy provider
func (t *SelfTester) Type() string {
	return PolicyProviderType
}

// RuleMatch implement the rule engine interface
func (t *SelfTester) RuleMatch(rule *rules.Rule, event eval.Event) bool {
	// send if not selftest related events
	return !t.IsExpectedEvent(rule, event, t.probe)
}

// EventDiscarderFound implement the rule engine interface
func (t *SelfTester) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
}
