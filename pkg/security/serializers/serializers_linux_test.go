// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package serializers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func newTestScrubber(t *testing.T) *utils.Scrubber {
	t.Helper()
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	return scrubber
}

func newAnomalyEvent() *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.AddToFlags(model.EventFlagsAnomalyDetectionEvent)
	return event
}

func TestCustomEventVariables_WithVariableStore(t *testing.T) {
	// Create a variable store with a boolean variable, simulating what a rule's
	// "set" action would populate during evaluation.
	store := &eval.VariableStore{}
	boolVar := eval.NewBoolVariable(true, eval.VariableOpts{})
	boolVar.Set(nil, true)
	store.Add("my_flag", boolVar)

	// Create the custom rule the same way sendAnomalyDetection does, passing the
	// variable store through eval.Opts.
	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()
	event.Rules = append(event.Rules, &model.MatchedRule{
		RuleID:      "test_rule",
		RuleVersion: "1.0",
		PolicyName:  "test_policy",
	})

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	// The non-scoped variable "my_flag" should appear in the event context variables.
	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["my_flag"])
}

func TestCustomEventVariables_WithoutVariableStore(t *testing.T) {
	// Create a custom rule without a variable store (the old behavior).
	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		nil,
	)

	event := newAnomalyEvent()

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	// Without a variable store, no variables should be serialized.
	assert.Nil(t, s.EventContextSerializer.Variables)
}

func TestCustomEventVariables_NilRule(t *testing.T) {
	// Simulate the old EventMarshallerCtor path where rule was nil.
	event := newAnomalyEvent()

	s := NewEventSerializer(event, nil, newTestScrubber(t))

	assert.Nil(t, s.EventContextSerializer.Variables)
}

func TestCustomEventVariables_MultipleTypes(t *testing.T) {
	store := &eval.VariableStore{}

	boolVar := eval.NewBoolVariable(false, eval.VariableOpts{})
	boolVar.Set(nil, true)
	store.Add("is_suspicious", boolVar)

	strVar := eval.NewStringVariable("", eval.VariableOpts{})
	strVar.Set(nil, "malware.exe")
	store.Add("threat_name", strVar)

	intVar := eval.NewIntVariable(0, eval.VariableOpts{})
	intVar.Set(nil, 42)
	store.Add("hit_count", intVar)

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["is_suspicious"])
	assert.Equal(t, "malware.exe", s.EventContextSerializer.Variables["threat_name"])
	assert.Equal(t, 42, s.EventContextSerializer.Variables["hit_count"])
}

func TestCustomEventVariables_PrivateExcluded(t *testing.T) {
	store := &eval.VariableStore{}

	publicVar := eval.NewBoolVariable(true, eval.VariableOpts{})
	store.Add("visible", publicVar)

	privateVar := eval.NewBoolVariable(true, eval.VariableOpts{Private: true})
	store.Add("hidden", privateVar)

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["visible"])
	_, found := s.EventContextSerializer.Variables["hidden"]
	assert.False(t, found, "private variables should not be serialized")
}

func TestCustomEventVariables_ScopedProcessVariable(t *testing.T) {
	store := &eval.VariableStore{}

	// A non-scoped variable (no dot in name) should appear in event context.
	globalVar := eval.NewBoolVariable(true, eval.VariableOpts{})
	store.Add("global_flag", globalVar)

	// A process-scoped variable should appear in process context, not event context.
	// For global (non-scoped) variables in the store, a name with "process." prefix
	// is filtered by the prefix logic: prefix="" excludes names with dots,
	// prefix="process." includes only names starting with "process.".
	processVar := eval.NewStringVariable("", eval.VariableOpts{})
	processVar.Set(nil, "test_value")
	store.Add("process.my_var", processVar)

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()
	// Set a non-zero PID so that newProcessContextSerializer doesn't return nil.
	event.ProcessContext.Pid = 1234

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	// Event context (prefix="") should have global_flag but NOT process.my_var.
	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["global_flag"])
	_, found := s.EventContextSerializer.Variables["process.my_var"]
	assert.False(t, found, "scoped variable should not appear in event context")

	// Process context (prefix="process.") should have my_var (trimmed of prefix).
	require.NotNil(t, s.ProcessContextSerializer)
	require.NotNil(t, s.ProcessContextSerializer.Variables)
	assert.Equal(t, "test_value", s.ProcessContextSerializer.Variables["my_var"])
}

func TestCustomEventVariables_MatchedRulesSerialized(t *testing.T) {
	store := &eval.VariableStore{}
	boolVar := eval.NewBoolVariable(true, eval.VariableOpts{})
	store.Add("flagged", boolVar)

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()
	event.Rules = append(event.Rules,
		&model.MatchedRule{
			RuleID:        "rule_1",
			RuleVersion:   "1.0",
			RuleTags:      map[string]string{"severity": "high"},
			PolicyName:    "my_policy",
			PolicyVersion: "2.0",
		},
		&model.MatchedRule{
			RuleID:      "rule_2",
			RuleVersion: "1.1",
			PolicyName:  "my_policy",
		},
	)

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	// Variables should be present.
	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["flagged"])

	// Matched rules should also be serialized for anomaly detection events.
	require.Len(t, s.EventContextSerializer.MatchedRules, 2)
	assert.Equal(t, "rule_1", s.EventContextSerializer.MatchedRules[0].ID)
	assert.Equal(t, "rule_2", s.EventContextSerializer.MatchedRules[1].ID)
}

func TestCustomEventVariables_SECLVariablesExcluded(t *testing.T) {
	store := &eval.VariableStore{}

	// Add a user variable.
	userVar := eval.NewBoolVariable(true, eval.VariableOpts{})
	store.Add("user_flag", userVar)

	// Add a variable with the same name as a SECLVariable — these are hardcoded
	// variables like "process.pid" and should be excluded from serialization.
	for name := range model.SECLVariables {
		fakeVar := eval.NewIntVariable(999, eval.VariableOpts{})
		store.Add(name, fakeVar)
		break // just need one to test
	}

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	event := newAnomalyEvent()

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	require.NotNil(t, s.EventContextSerializer.Variables)
	assert.Equal(t, true, s.EventContextSerializer.Variables["user_flag"])

	// SECLVariable names should not appear in serialized variables.
	for name := range model.SECLVariables {
		_, found := s.EventContextSerializer.Variables[name]
		assert.False(t, found, "SECLVariable %q should not be serialized", name)
	}
}
