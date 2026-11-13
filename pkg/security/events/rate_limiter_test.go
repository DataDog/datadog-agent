// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func newTestRateLimiter() *RateLimiter {
	return NewRateLimiter(&config.RuntimeSecurityConfig{
		AnomalyDetectionRateLimiterNumKeys:          10,
		AnomalyDetectionRateLimiterNumEventsAllowed: 1,
		AnomalyDetectionRateLimiterPeriod:           time.Minute,
	}, nil)
}

func newTestRuleSet(t *testing.T, ruleIDs ...eval.RuleID) *rules.RuleSet {
	t.Helper()

	ruleOpts, evalOpts := rules.NewBothOpts(map[eval.EventType]bool{"*": true})
	rs := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)
	for _, id := range ruleIDs {
		_, err := rs.AddRule(&rules.PolicyRule{
			Def: &rules.RuleDefinition{ID: id, Expression: `exec.file.path == "/bin/sh"`},
		})
		require.NoError(t, err)
	}
	return rs
}

// TestRateLimiterAllowsCustomRulesBeforeRuleSet checks that custom events sent
// before the first rule set is applied are not dropped.
func TestRateLimiterAllowsCustomRulesBeforeRuleSet(t *testing.T) {
	rl := newTestRateLimiter()
	event := NewCustomEvent(model.CustomEventType, nil)

	for _, id := range append(AllCustomRuleIDs(), HeartbeatRuleID, EBPFLessHelloMessageRuleID) {
		assert.True(t, rl.Allow(id, event), "custom rule %s should be allowed", id)
	}
}

func TestRateLimiterApply(t *testing.T) {
	rl := newTestRateLimiter()
	event := NewCustomEvent(model.CustomEventType, nil)

	assert.False(t, rl.Allow("rule_a", event), "rule_a should be unknown before its rule set is applied")

	rl.Apply(newTestRuleSet(t, "rule_a"), AllCustomRuleIDs())
	assert.True(t, rl.Allow("rule_a", event))
	assert.True(t, rl.Allow(AnomalyDetectionRuleID, event))

	rl.Apply(newTestRuleSet(t, "rule_b"), AllCustomRuleIDs())
	assert.False(t, rl.Allow("rule_a", event), "rule_a should be unknown once removed from the rule set")
	assert.True(t, rl.Allow("rule_b", event))
}
