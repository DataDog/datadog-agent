// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	json "encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

const (
	policySource  = "self-test"                          //nolint: deadcode, unused
	policyVersion = "1.0.0"                              //nolint: deadcode, unused
	policyName    = "datadog-agent-cws-self-test-policy" //nolint: deadcode, unused
	ruleIDPrefix  = "datadog_agent_cws_self_test_rule"   //nolint: deadcode, unused

	// DefaultTimeout default timeout
	DefaultTimeout = 30 * time.Second

	// PolicyProviderType name of the self test policy provider
	PolicyProviderType = "selfTesterPolicyProvider"
)

// SelfTestEvent is used to report a self test result
type SelfTestEvent struct {
	events.CustomEventCommonFields
	Success    []eval.RuleID                                `json:"succeeded_tests"`
	Fails      []eval.RuleID                                `json:"failed_tests"`
	TestEvents map[eval.RuleID]*serializers.EventSerializer `json:"test_events"`
}

// ToJSON marshal using json format
func (t SelfTestEvent) ToJSON() ([]byte, error) {
	// cleanup the serialization of potentially nil slices
	if t.Success == nil {
		t.Success = []eval.RuleID{}
	}
	if t.Fails == nil {
		t.Fails = []eval.RuleID{}
	}

	return json.Marshal(t)
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []eval.RuleID, fails []eval.RuleID, testEvents map[eval.RuleID]*serializers.EventSerializer) (*rules.Rule, *events.CustomEvent) {
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
func (t *SelfTester) SetOnNewPoliciesReadyCb(_ func()) {
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
func (t *SelfTester) EventDiscarderFound(_ *rules.RuleSet, _ eval.Event, _ eval.Field, _ eval.EventType) {
}
