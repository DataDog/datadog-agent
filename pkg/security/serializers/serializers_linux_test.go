// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package serializers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/schemas"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// validateProcessSchemaFields validates that the given JSON document's boolean fields
// conform to the process schema. It only reports errors for the fields under test,
// ignoring issues with other fields (executable, credentials) that require full event setup.
func validateProcessSchemaFields(t *testing.T, jsonStr string) {
	t.Helper()

	fs := http.FS(schemas.AssetFS)
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem("file:///process.schema.json", fs)
	documentLoader := gojsonschema.NewStringLoader(jsonStr)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	require.NoError(t, err)

	for _, desc := range result.Errors() {
		if desc.Type() == "additional_property_not_allowed" {
			continue
		}
		// Only fail on errors related to our fields under test.
		field := desc.Field()
		switch field {
		case "is_thread", "is_exec", "is_exec_child", "is_parent_missing":
			t.Errorf("schema validation error for %s: %s", field, desc)
		}
	}
}

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

func TestCustomEventVariables_AncestorVariables(t *testing.T) {
	// Build a process scoper that returns event.ProcessCacheEntry.
	scopedVars := eval.NewScopedVariables("process", func(ctx *eval.Context) eval.VariableScope {
		if pce := ctx.Event.(*model.Event).ProcessCacheEntry; pce != nil {
			return pce
		}
		return nil
	})

	// Create a scoped boolean variable named "process.is_suspicious".
	seclVar, err := scopedVars.NewSECLVariable("process.is_suspicious", false, "process", eval.VariableOpts{})
	require.NoError(t, err)

	store := &eval.VariableStore{}
	store.Add("process.is_suspicious", seclVar)

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{VariableStore: store},
	)

	// Create an event with a process and one ancestor.
	event := newAnomalyEvent()
	event.ProcessContext.Pid = 42

	ancestorPCE := &model.ProcessCacheEntry{}
	ancestorPCE.Process.Pid = 1
	event.ProcessContext.Ancestor = ancestorPCE

	mainPCE := &model.ProcessCacheEntry{}
	mainPCE.Process.Pid = 42
	event.ProcessCacheEntry = mainPCE

	// Set the variable value for the ancestor's scope.
	event.ProcessCacheEntry = ancestorPCE
	ctx := eval.NewContext(event)
	err = seclVar.(eval.MutableVariable).Set(ctx, true)
	require.NoError(t, err)

	// Restore the main process PCE.
	event.ProcessCacheEntry = mainPCE

	s := NewEventSerializer(event, rule, newTestScrubber(t))

	require.NotNil(t, s.ProcessContextSerializer)
	require.Len(t, s.ProcessContextSerializer.Ancestors, 1)

	// The ancestor should have the variable set to true.
	require.NotNil(t, s.ProcessContextSerializer.Ancestors[0].Variables)
	assert.Equal(t, true, s.ProcessContextSerializer.Ancestors[0].Variables["is_suspicious"])

	// The main process should have the variable's zero value (false), since
	// the scoped variable evaluator returns the zero value when not explicitly set.
	require.NotNil(t, s.ProcessContextSerializer.Variables)
	assert.Equal(t, false, s.ProcessContextSerializer.Variables["is_suspicious"])
}

func TestProcessSerializer_IsExecFields(t *testing.T) {
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)

	proc := &event.ProcessContext.Process
	proc.Pid = 1234
	proc.Tid = 1234
	proc.PPid = 1
	proc.Comm = "test"
	proc.FileEvent.PathnameStr = "/usr/bin/test"
	proc.FileEvent.BasenameStr = "test"
	proc.FileEvent.Inode = 12345
	proc.FileEvent.MountID = 1
	proc.FileEvent.FileFields.Mode = 0o755
	proc.IsThread = false
	proc.IsExec = true
	proc.IsExecExec = false
	proc.IsParentMissing = false

	ps := newProcessSerializer(proc, event)

	assert.False(t, ps.IsThread)
	assert.True(t, ps.IsExec)
	assert.False(t, ps.IsExecExec)
	assert.False(t, ps.IsParentMissing)

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	// Verify the JSON keys exist with expected values.
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Equal(t, true, raw["is_exec"], "is_exec should be true in JSON")
	// omitempty means false booleans are absent
	_, hasIsThread := raw["is_thread"]
	assert.False(t, hasIsThread, "is_thread should be omitted when false")
	_, hasIsExecExec := raw["is_exec_child"]
	assert.False(t, hasIsExecExec, "is_exec_child should be omitted when false")
	_, hasIsParentMissing := raw["is_parent_missing"]
	assert.False(t, hasIsParentMissing, "is_parent_missing should be omitted when false")

	// Validate against the JSON schema.
	validateProcessSchemaFields(t, string(data))
}

func TestProcessSerializer_IsExecFields_AllTrue(t *testing.T) {
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)

	proc := &event.ProcessContext.Process
	proc.Pid = 1234
	proc.Tid = 1234
	proc.PPid = 1
	proc.Comm = "test"
	proc.FileEvent.PathnameStr = "/usr/bin/test"
	proc.FileEvent.BasenameStr = "test"
	proc.FileEvent.Inode = 12345
	proc.FileEvent.MountID = 1
	proc.FileEvent.FileFields.Mode = 0o755
	proc.IsThread = true
	proc.IsExec = true
	proc.IsExecExec = true
	proc.IsParentMissing = true

	ps := newProcessSerializer(proc, event)

	assert.True(t, ps.IsThread)
	assert.True(t, ps.IsExec)
	assert.True(t, ps.IsExecExec)
	assert.True(t, ps.IsParentMissing)

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Equal(t, true, raw["is_thread"])
	assert.Equal(t, true, raw["is_exec"])
	assert.Equal(t, true, raw["is_exec_child"])
	assert.Equal(t, true, raw["is_parent_missing"])

	// Validate against the JSON schema.
	validateProcessSchemaFields(t, string(data))
}

func TestProcessSerializer_IsExecFields_Ancestors(t *testing.T) {
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)

	// Set up main process.
	proc := &event.ProcessContext.Process
	proc.Pid = 1234
	proc.Tid = 1234
	proc.PPid = 100
	proc.Comm = "child"
	proc.FileEvent.PathnameStr = "/usr/bin/child"
	proc.FileEvent.BasenameStr = "child"
	proc.FileEvent.Inode = 12345
	proc.FileEvent.MountID = 1
	proc.FileEvent.FileFields.Mode = 0o755
	proc.IsExec = true
	proc.IsExecExec = true

	// Set up ancestor.
	ancestor := &model.ProcessCacheEntry{}
	ancestor.Process.Pid = 100
	ancestor.Process.Tid = 100
	ancestor.Process.PPid = 1
	ancestor.Process.Comm = "parent"
	ancestor.Process.FileEvent.PathnameStr = "/usr/bin/parent"
	ancestor.Process.FileEvent.BasenameStr = "parent"
	ancestor.Process.FileEvent.Inode = 67890
	ancestor.Process.FileEvent.MountID = 1
	ancestor.Process.FileEvent.FileFields.Mode = 0o755
	ancestor.Process.IsExec = true
	ancestor.Process.IsParentMissing = true
	event.ProcessContext.Ancestor = ancestor
	event.ProcessContext.Parent = &ancestor.ProcessContext.Process

	rule := events.NewCustomRule(
		events.AnomalyDetectionRuleID,
		events.AnomalyDetectionRuleDesc,
		&eval.Opts{},
	)
	pcs := newProcessContextSerializer(event.ProcessContext, event, rule)
	require.NotNil(t, pcs)

	// Verify main process fields.
	assert.True(t, pcs.IsExec)
	assert.True(t, pcs.IsExecExec)

	// Verify ancestor fields.
	require.NotNil(t, pcs.Parent)
	assert.True(t, pcs.Parent.IsExec)
	assert.True(t, pcs.Parent.IsParentMissing)

	require.NotEmpty(t, pcs.Ancestors)
	assert.True(t, pcs.Ancestors[0].IsExec)
	assert.True(t, pcs.Ancestors[0].IsParentMissing)

	// Validate ancestor serialization against the JSON schema.
	data, err := json.Marshal(pcs.Ancestors[0])
	require.NoError(t, err)
	validateProcessSchemaFields(t, string(data))
}
