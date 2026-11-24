// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/rules/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

const neverExecutedExpression = `exec.file.path == "/tmp/cws-ruleset-loaded-actions-never-executed"`

// the ruleset loaded schema requires `monitored_files`, which is only reported when a loaded rule monitors a file
var monitoredFileRule = &rules.RuleDefinition{
	ID:         "test_ruleset_loaded_actions_monitored_file",
	Expression: `open.file.path == "/tmp/cws-ruleset-loaded-actions-never-opened" && open.flags & O_CREAT != 0`,
}

// reloadAndGetRuleState writes ruleDefs as the test policy, reloads it and returns the state of ruleID
// reported by the resulting ruleset loaded event
func reloadAndGetRuleState(t *testing.T, test *testModule, ruleDefs []*rules.RuleDefinition, ruleID string) *monitor.RuleState {
	t.Helper()

	if err := setTestPolicy(commonCfgDir, nil, append(ruleDefs, monitoredFileRule)); err != nil {
		t.Fatal(err)
	}

	var ruleState *monitor.RuleState
	reloadErr := make(chan error, 1)
	err := test.GetCustomEventSent(t, func() error {
		// the ruleset loaded event is sent synchronously during the reload, and its handler only
		// processes it once this function has returned
		go func() {
			reloadErr <- test.reloadPolicies()
		}()
		return nil
	}, func(_ *rules.Rule, customEvent *events.CustomEvent) bool {
		if !validateRuleSetLoadedSchema(t, customEvent) {
			return false
		}

		data, err := serializers.MarshalCustomEvent(customEvent)
		if err != nil {
			t.Error(err)
			return false
		}

		var event struct {
			Policies []*monitor.PolicyState `json:"policies"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Error(err)
			return false
		}

		for _, policy := range event.Policies {
			for _, rule := range policy.Rules {
				if rule.ID == ruleID {
					ruleState = rule
					return true
				}
			}
		}
		return false
	}, 20*time.Second, model.CustomEventType, events.RulesetLoadedRuleID)
	require.NoError(t, err)
	require.NoError(t, <-reloadErr)
	require.NotNil(t, ruleState, "rule %s not found in the ruleset loaded event", ruleID)

	return ruleState
}

func TestRulesetLoadedRejectedActions(t *testing.T) {
	SkipIfNotAvailable(t)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{{
		ID:         "test_ruleset_loaded_actions_initial",
		Expression: neverExecutedExpression,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("invalid-set-scope", func(t *testing.T) {
		ruleState := reloadAndGetRuleState(t, test, []*rules.RuleDefinition{{
			ID:         "test_rejected_set_scope",
			Expression: neverExecutedExpression,
			Actions: []*rules.ActionDefinition{
				{
					Log: &rules.LogDefinition{
						Level: "info",
					},
				},
				{
					Set: &rules.SetDefinition{
						Name:  "my_var",
						Value: "foo",
						Scope: "invalid_scope",
					},
				},
			},
		}}, "test_rejected_set_scope")

		assert.Equal(t, "loaded", ruleState.Status)
		assert.Equal(t, []monitor.RuleAction{
			{
				Status: monitor.ActionStatusLoaded,
				Log: &monitor.LogAction{
					Level: "info",
				},
			},
			{
				Status:  monitor.ActionStatusRejected,
				Message: "invalid scope 'invalid_scope'",
				Set: &monitor.RuleSetAction{
					Name:  "my_var",
					Value: "foo",
					Scope: "invalid_scope",
				},
			},
		}, ruleState.Actions)
	})

	t.Run("multiple-action-types-in-one-entry", func(t *testing.T) {
		ruleState := reloadAndGetRuleState(t, test, []*rules.RuleDefinition{{
			ID:         "test_rejected_multiple_types",
			Expression: neverExecutedExpression,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
					Hash: &rules.HashDefinition{},
				},
			},
		}}, "test_rejected_multiple_types")

		assert.Equal(t, "loaded", ruleState.Status)
		assert.Equal(t, []monitor.RuleAction{
			{
				Status:  monitor.ActionStatusRejected,
				Message: "only one action can be specified",
				Kill: &monitor.RuleKillAction{
					Signal: "SIGKILL",
					Scope:  "process",
				},
				Hash: &monitor.HashAction{
					Enabled: true,
				},
			},
		}, ruleState.Actions)
	})

	t.Run("unserializable-set-values", func(t *testing.T) {
		// serialized as `.nan` and as a mapping with an integer key in the policy YAML
		ruleState := reloadAndGetRuleState(t, test, []*rules.RuleDefinition{{
			ID:         "test_rejected_unserializable_values",
			Expression: neverExecutedExpression,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:  "nan_value",
						Value: math.NaN(),
					},
				},
				{
					Set: &rules.SetDefinition{
						Name:  "map_value",
						Value: map[interface{}]interface{}{1: "a"},
					},
				},
			},
		}}, "test_rejected_unserializable_values")

		assert.Equal(t, "loaded", ruleState.Status)
		require.Len(t, ruleState.Actions, 2)
		for _, action := range ruleState.Actions {
			assert.Equal(t, monitor.ActionStatusRejected, action.Status)
			assert.NotEmpty(t, action.Message)
		}
		assert.Equal(t, "NaN", ruleState.Actions[0].Set.Value)
		assert.Equal(t, "map[1:a]", ruleState.Actions[1].Set.Value)
	})

	t.Run("null-action", func(t *testing.T) {
		// the nil definition is serialized as a `null` entry in the policy YAML, and is placed first
		// so that every iteration over the rule actions has to go through it
		ruleState := reloadAndGetRuleState(t, test, []*rules.RuleDefinition{{
			ID:         "test_null_action",
			Expression: neverExecutedExpression,
			Actions: []*rules.ActionDefinition{
				nil,
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		}}, "test_null_action")

		assert.Equal(t, "loaded", ruleState.Status)
		assert.Equal(t, []monitor.RuleAction{
			{
				Status: monitor.ActionStatusLoaded,
				Kill: &monitor.RuleKillAction{
					Signal: "SIGKILL",
				},
			},
		}, ruleState.Actions)
	})

	t.Run("filtered-rule-with-null-action", func(t *testing.T) {
		// the nil definition is serialized as a `null` entry in the policy YAML
		ruleState := reloadAndGetRuleState(t, test, []*rules.RuleDefinition{{
			ID:                     "test_filtered_null_action",
			Expression:             neverExecutedExpression,
			AgentVersionConstraint: "< 0.0.1",
			Actions: []*rules.ActionDefinition{
				nil,
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		}}, "test_filtered_null_action")

		assert.Equal(t, "filtered", ruleState.Status)
		assert.Equal(t, []monitor.RuleAction{
			{
				Status: monitor.ActionStatusRejected,
				Kill: &monitor.RuleKillAction{
					Signal: "SIGKILL",
				},
			},
		}, ruleState.Actions)
	})
}
