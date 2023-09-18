//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	policySource  = "self-test"                          // nolint: deadcode, unused
	policyVersion = "1.0.0"                              // nolint: deadcode, unused
	policyName    = "datadog-agent-cws-self-test-policy" // nolint: deadcode, unused
	ruleIDPrefix  = "datadog_agent_cws_self_test_rule"   // nolint: deadcode, unused

	// PolicyProviderType name of the self test policy provider
	PolicyProviderType = "selfTesterPolicyProvider"
)

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
