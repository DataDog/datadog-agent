// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/xeipuuv/gojsonschema"

	"github.com/Masterminds/semver/v3"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	yamlk8s "sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func savePolicy(filename string, testPolicy *PolicyDef) error {
	yamlBytes, err := yaml.Marshal(testPolicy)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, yamlBytes, 0700)
}

func TestMacroMerge(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test" && process.name == "/usr/bin/vim"`,
		}},
		Macros: []*MacroDefinition{{
			ID:     "test_macro",
			Values: []string{"/usr/bin/vi"},
		}},
	}

	testPolicy2 := &PolicyDef{
		Macros: []*MacroDefinition{{
			ID:      "test_macro",
			Values:  []string{"/usr/bin/vim"},
			Combine: MergePolicy,
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	event := model.NewFakeEvent()
	event.SetFieldValue("open.file.path", "/tmp/test")
	event.SetFieldValue("process.comm", "/usr/bin/vi")

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	macro := rs.evalOpts.MacroStore.Get("test_macro")
	if macro == nil {
		t.Fatalf("failed to find test_macro in ruleset: %+v", rs.evalOpts.MacroStore.List())
	}

	testPolicy2.Macros[0].Combine = ""

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Fatal(err)
	}
}

func TestRuleMerge(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "test_rule_foo",
				Expression: `exec.file.name == "foo"`,
			},
			{
				ID:         "test_rule_bar",
				Expression: `exec.file.name == "bar"`,
				Disabled:   true,
			},
		},
	}

	testPolicy2 := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Combine:    OverridePolicy,
			},
			{
				ID:         "test_rule_foo",
				Expression: `exec.file.name == "foo"`,
				Disabled:   true,
			},
			{
				ID:         "test_rule_bar",
				Expression: `exec.file.name == "bar"`,
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	t.Run("override", func(t *testing.T) {
		rule := rs.GetRules()["test_rule"]
		if rule == nil {
			t.Fatal("failed to find test_rule in ruleset")
		}

		testPolicy2.Rules[0].Combine = ""

		if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
			t.Fatal(err)
		}

		if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("enabled-disabled", func(t *testing.T) {
		rule := rs.GetRules()["test_rule_foo"]
		if rule == nil {
			t.Fatal("expected test_rule_foo to be loaded now")
		}
	})

	t.Run("disabled-enabled", func(t *testing.T) {
		rule := rs.GetRules()["test_rule_bar"]
		if rule == nil {
			t.Fatal("expected test_rule_bar to be loaded")
		}
	})
}

func TestActionSetVariable(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{{
				Set: &SetDefinition{
					Name:  "var1",
					Value: true,
				},
			}, {
				Set: &SetDefinition{
					Name:  "var2",
					Value: "value",
				},
			}, {
				Set: &SetDefinition{
					Name:  "var3",
					Value: 123,
				},
			}, {
				Set: &SetDefinition{
					Name:  "var4",
					Value: 123,
					Scope: "process",
				},
			}, {
				Set: &SetDefinition{
					Name: "var5",
					Value: []string{
						"val1",
					},
				},
			}, {
				Set: &SetDefinition{
					Name: "var6",
					Value: []int{
						123,
					},
				},
			}, {
				Set: &SetDefinition{
					Name:   "var7",
					Append: true,
					Value: []string{
						"aaa",
					},
				},
			}, {
				Set: &SetDefinition{
					Name:   "var8",
					Append: true,
					Value: []int{
						123,
					},
				},
			}, {
				Set: &SetDefinition{
					Name:  "var9",
					Field: "open.file.path",
				},
			}, {
				Set: &SetDefinition{
					Name:   "var10",
					Field:  "open.file.path",
					Append: true,
				},
			}},
		}, {
			ID: "test_rule2",
			Expression: `open.file.path == "/tmp/test2" && ` +
				`${var1} == true && ` +
				`"${var2}" == "value" && ` +
				`${var2} == "value" && ` +
				`${var3} == 123 && ` +
				`${process.var4} == 123 && ` +
				`"val1" in ${var5} && ` +
				`123 in ${var6} && ` +
				`"aaa" in ${var7} && ` +
				`123 in ${var8} && ` +
				`${var9} == "/tmp/test" && ` +
				`"/tmp/test" in ${var10}`,
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test2")
	event.SetFieldValue("open.flags", syscall.O_RDONLY)

	if rs.Evaluate(event) {
		t.Errorf("Expected event to match no rule")
	}

	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	event.SetFieldValue("open.file.path", "/tmp/test2")
	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	scopedVariables := rs.scopedVariables["process"].(*eval.ScopedVariables)

	assert.Equal(t, scopedVariables.Len(), 1)
	event.ProcessCacheEntry.Release()
	assert.Equal(t, scopedVariables.Len(), 0)
}

func TestActionSetVariableTTL(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{
				{
					Set: &SetDefinition{
						Name:   "var1",
						Append: true,
						Value:  "foo",
						TTL: &HumanReadableDuration{
							Duration: 1 * time.Second,
						},
					},
				},
				{
					Set: &SetDefinition{
						Name:   "var2",
						Append: true,
						Value:  123,
						TTL: &HumanReadableDuration{
							Duration: 1 * time.Second,
						},
					},
				},
				{
					Set: &SetDefinition{
						Name:   "scopedvar1",
						Append: true,
						Value:  []string{"bar"},
						Scope:  "process",
						TTL: &HumanReadableDuration{
							Duration: 1 * time.Second,
						},
					},
				},
				{
					Set: &SetDefinition{
						Name:   "scopedvar2",
						Append: true,
						Value:  []int{123},
						Scope:  "process",
						TTL: &HumanReadableDuration{
							Duration: 1 * time.Second,
						},
					},
				},
				{
					Set: &SetDefinition{
						Name:  "simplevarwithttl",
						Value: 456,
						Scope: "container",
						TTL: &HumanReadableDuration{
							Duration: 1 * time.Second,
						},
					},
				},
			},
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ContainerContext = &model.ContainerContext{
		ContainerID: "0123456789abcdef",
	}
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	opts := rs.evalOpts

	existingVariable := opts.VariableStore.Get("var1")
	assert.NotNil(t, existingVariable)
	stringArrayVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, stringArrayVar)
	assert.True(t, ok)
	strValue, _ := stringArrayVar.GetValue()
	assert.NotNil(t, strValue)
	assert.Contains(t, strValue, "foo")
	assert.IsType(t, strValue, []string{})

	existingVariable = opts.VariableStore.Get("var2")
	assert.NotNil(t, existingVariable)
	intArrayVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, intArrayVar)
	assert.True(t, ok)
	value, _ := intArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.Contains(t, value, 123)
	assert.IsType(t, value, []int{})

	ctx := eval.NewContext(event)
	existingScopedVariable := opts.VariableStore.Get("process.scopedvar1")
	assert.NotNil(t, existingScopedVariable)
	stringArrayScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, stringArrayScopedVar)
	assert.True(t, ok)
	value, _ = stringArrayScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Contains(t, value, "bar")
	assert.IsType(t, value, []string{})

	existingScopedVariable = opts.VariableStore.Get("process.scopedvar2")
	assert.NotNil(t, existingScopedVariable)
	intArrayScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, intArrayScopedVar)
	assert.True(t, ok)
	value, _ = intArrayScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Contains(t, value, 123)
	assert.IsType(t, value, []int{})

	existingContainerScopedVariable := opts.VariableStore.Get("container.simplevarwithttl")
	assert.NotNil(t, existingContainerScopedVariable)
	intVarScopedVar, ok := existingContainerScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, intVarScopedVar)
	assert.True(t, ok)
	value, isSet := intVarScopedVar.GetValue(ctx)
	assert.True(t, isSet)
	assert.NotNil(t, value)
	assert.Equal(t, 456, value)
	assert.IsType(t, int(0), value)

	time.Sleep(time.Second + 100*time.Millisecond)

	value, _ = stringArrayVar.GetValue()
	assert.NotContains(t, value, "foo")
	assert.Len(t, value, 0)

	value, _ = intArrayVar.GetValue()
	assert.NotContains(t, value, 123)
	assert.Len(t, value, 0)

	value, _ = stringArrayScopedVar.GetValue(ctx)
	assert.NotContains(t, value, "foo")
	assert.Len(t, value, 0)

	value, _ = intArrayScopedVar.GetValue(ctx)
	assert.NotContains(t, value, 123)
	assert.Len(t, value, 0)

	value, isSet = intVarScopedVar.GetValue(ctx)
	assert.False(t, isSet)
	assert.Equal(t, 0, value)
}

func TestActionSetVariableSize(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{
				{
					Set: &SetDefinition{
						Name:   "var1",
						Append: true,
						Value:  "foo",
						Size:   1,
					},
				}, {
					Set: &SetDefinition{
						Name:   "var2",
						Append: true,
						Value:  1,
						Size:   2,
					},
				},
				{
					Set: &SetDefinition{
						Name:   "scopedvar1",
						Append: true,
						Value:  "bar",
						Size:   1,
						Scope:  "process",
					},
				}, {
					Set: &SetDefinition{
						Name:   "scopedvar2",
						Append: true,
						Value:  123,
						Size:   2,
						Scope:  "process",
					},
				},
			},
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	opts := rs.evalOpts

	existingVariable := opts.VariableStore.Get("var1")
	assert.NotNil(t, existingVariable)

	stringArrayVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, stringArrayVar)
	assert.True(t, ok)
	value, set := stringArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.False(t, set)

	existingVariable = opts.VariableStore.Get("var2")
	assert.NotNil(t, existingVariable)

	intArrayVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, intArrayVar)
	assert.True(t, ok)
	_, set = intArrayVar.GetValue()
	assert.False(t, set)

	existingScopedVariable := opts.VariableStore.Get("process.scopedvar1")
	assert.NotNil(t, existingScopedVariable)
	stringArrayScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, stringArrayScopedVar)
	assert.True(t, ok)

	existingScopedVariable = opts.VariableStore.Get("process.scopedvar2")
	assert.NotNil(t, existingScopedVariable)
	intArrayScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, intArrayScopedVar)
	assert.True(t, ok)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	ctx := eval.NewContext(event)

	_, set = stringArrayScopedVar.GetValue(ctx)
	assert.False(t, set)

	_, set = intArrayScopedVar.GetValue(ctx)
	assert.False(t, set)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}
	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set = stringArrayVar.GetValue()
	assert.Contains(t, value, "foo")
	assert.Len(t, value, 1)
	assert.IsType(t, value, []string{})
	assert.True(t, set)

	value, set = intArrayVar.GetValue()
	assert.IsType(t, value, []int{})
	assert.Contains(t, value, 1)
	assert.Len(t, value, 1)
	assert.True(t, set)

	value, set = stringArrayScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Contains(t, value, "bar")
	assert.IsType(t, value, []string{})
	assert.Len(t, value, 1)
	assert.True(t, set)

	value, set = intArrayScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Contains(t, value, 123)
	assert.IsType(t, value, []int{})
	assert.Len(t, value, 1)
	assert.True(t, set)
}

func TestActionSetEmptyScope(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{
				{
					Set: &SetDefinition{
						Name:   "scopedvar1",
						Append: true,
						Value:  "bar",
						Size:   1,
						Scope:  "process",
					},
				},
			},
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	opts := rs.evalOpts

	existingScopedVariable := opts.VariableStore.Get("process.scopedvar1")
	assert.NotNil(t, existingScopedVariable)
	stringArrayScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, stringArrayScopedVar)
	assert.True(t, ok)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.SetFieldValue("open.file.path", "/tmp/test")

	ctx := eval.NewContext(event)
	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set := stringArrayScopedVar.GetValue(ctx)
	assert.Nil(t, value)
	assert.False(t, set)
}

func TestActionSetVariableConflict(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{{
				Set: &SetDefinition{
					Name:  "var1",
					Value: true,
				},
			}, {
				Set: &SetDefinition{
					Name:  "var1",
					Value: "value",
				},
			}},
		}, {
			ID: "test_rule2",
			Expression: `open.file.path == "/tmp/test2" && ` +
				`${var1} == true`,
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err == nil {
		t.Error("expected policy to fail to load")
	}
}

func TestActionSetVariableInitialValue(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test" && ${var1} == 123`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:         "var1",
							DefaultValue: 123,
							Value:        456,
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	opts := rs.evalOpts

	existingVariable := opts.VariableStore.Get("var1")
	assert.NotNil(t, existingVariable)

	intVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, intVar)
	assert.True(t, ok)
	value, set := intVar.GetValue()
	assert.NotNil(t, value)
	assert.Equal(t, 123, value)
	assert.False(t, set)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set = intVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, 456, value)

	if rs.Evaluate(event) {
		t.Errorf("Expected event to not match rule")
	}
}

func TestActionSetVariableInherited(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "guess",
				Expression: `open.file.path == "/tmp/guess"`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:      "var1",
							Value:     456,
							Scope:     "process",
							Inherited: true,
						},
					},
				},
			},
			{
				ID:         "victory",
				Expression: `open.file.path == "/tmp/guess2" && ${process.var1} == 456`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:      "var1",
							Value:     1000,
							Scope:     "process",
							Inherited: true,
						},
					},
				},
			},
			{
				ID:         "game_over",
				Expression: `open.file.path == "/tmp/guess2" && ${process.var1} == 0`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:      "var1",
							Value:     0,
							Scope:     "process",
							Inherited: true,
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Fatal(err)
	}

	opts := rs.evalOpts

	existingScopedVariable := opts.VariableStore.Get("process.var1")
	assert.NotNil(t, existingScopedVariable)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{
					Pid: 1,
				},
			},
		},
	}
	event.ProcessCacheEntry.Retain()
	event.SetFieldValue("open.file.path", "/tmp/guess")

	ctx := eval.NewContext(event)

	assert.NotNil(t, existingScopedVariable)
	stringScopedVar, ok := existingScopedVariable.(eval.ScopedVariable)
	assert.NotNil(t, stringScopedVar)
	assert.True(t, ok)

	value, set := stringScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	// TODO(lebauce): should be 123. default_value are not properly handled
	assert.Equal(t, 0, value)
	assert.False(t, set)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set = stringScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Equal(t, 456, value)
	assert.True(t, set)

	event2 := model.NewFakeEvent()
	event2.Type = uint32(model.FileOpenEventType)
	event2.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{
					Pid: 2,
				},
			},
			Ancestor: event.ProcessCacheEntry,
		},
	}
	event2.ProcessCacheEntry.Retain()
	event2.SetFieldValue("open.file.path", "/tmp/guess2")

	ctx = eval.NewContext(event2)
	if !rs.Evaluate(event2) {
		t.Errorf("Expected event to match rule")
	}

	value, set = stringScopedVar.GetValue(ctx)
	assert.NotNil(t, value)
	assert.Equal(t, 1000, value)
	assert.True(t, set)
}

func stringPtr(input string) *string {
	return &input
}

func fakeOpenEvent(path string, pid uint32, ancestor *model.ProcessCacheEntry) *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{
					Pid: pid,
				},
			},
		},
	}
	if ancestor != nil {
		event.ProcessCacheEntry.ProcessContext.Ancestor = ancestor
	}
	event.ProcessCacheEntry.Retain()
	event.SetFieldValue("open.file.path", path)
	return event
}

func TestActionSetVariableInheritedFilter(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "first_execution_context",
				Expression: `open.file.path == "/tmp/first" && ${process.correlation_key} == ""`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"first_${builtins.uuid4}"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
			{
				ID:         "second_execution_context",
				Expression: `open.file.path == "/tmp/second" && ${process.correlation_key} in ["", ~"first_*"]`,
				Actions: []*ActionDefinition{
					{
						Filter: stringPtr(`${process.correlation_key} != ""`),
						Set: &SetDefinition{
							Name:         "parent_correlation_keys",
							DefaultValue: "",
							Expression:   "${process.correlation_key}",
							Scope:        "process",
							Append:       true,
							Inherited:    true,
						},
					},
					{
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"second_${builtins.uuid4}"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
			{
				ID:         "third_execution_context",
				Expression: `open.file.path == "/tmp/third" && ${process.correlation_key} in ["", ~"first_*", ~"second_*"]`,
				Actions: []*ActionDefinition{
					{
						Filter: stringPtr(`${process.correlation_key} != ""`),
						Set: &SetDefinition{
							Name:         "parent_correlation_keys",
							DefaultValue: "",
							Expression:   "${process.correlation_key}",
							Scope:        "process",
							Append:       true,
							Inherited:    true,
						},
					},
					{
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"third_${builtins.uuid4}"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Fatal(err)
	}

	opts := rs.evalOpts

	// Fetch process.correlation_key variable
	correlationKeySECLVariable := opts.VariableStore.Get("process.correlation_key")
	assert.NotNil(t, correlationKeySECLVariable)
	correlationKeyScopedVariable, ok := correlationKeySECLVariable.(eval.ScopedVariable)
	assert.NotNil(t, correlationKeyScopedVariable)
	assert.True(t, ok)

	// Fetch process.parent_correlation_keys variable
	parentCorrelationKeysSECLVariable := opts.VariableStore.Get("process.parent_correlation_keys")
	assert.NotNil(t, parentCorrelationKeysSECLVariable)
	parentCorrelationKeysScopedVariable, ok := parentCorrelationKeysSECLVariable.(eval.ScopedVariable)
	assert.NotNil(t, parentCorrelationKeysScopedVariable)
	assert.True(t, ok)

	event := fakeOpenEvent("/tmp/first", 1, nil)
	ctx := eval.NewContext(event)

	// test correlation key initial value
	correlationKeyValue, set := correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "", correlationKeyValue)
	assert.False(t, set)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.True(t, strings.HasPrefix(correlationKeyValue.(string), "first_"))
	assert.True(t, set)

	parentCorrelationKeysValue, _ := parentCorrelationKeysScopedVariable.GetValue(ctx)
	assert.Equal(t, len(parentCorrelationKeysValue.([]string)), 0)

	correlationKeyFromFirstRule := correlationKeyValue.(string)

	// trigger the first rule again, and make sure nothing changes
	event2 := fakeOpenEvent("/tmp/first", 2, event.ProcessCacheEntry)
	ctx = eval.NewContext(event2)

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, correlationKeyValue, correlationKeyFromFirstRule)
	assert.True(t, set)

	if rs.Evaluate(event2) {
		t.Errorf("Didn't expected event to match rule")
	}

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, correlationKeyValue, correlationKeyFromFirstRule)
	assert.True(t, set)

	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx)
	assert.Equal(t, len(parentCorrelationKeysValue.([]string)), 0)

	// jump to the third rule, check:
	//  - that the correlation key is updated with the pattern from the third rule
	//  - that the first correlation key is now in the "parent correlation keys" variable
	event3 := fakeOpenEvent("/tmp/third", 3, event2.ProcessCacheEntry)
	ctx = eval.NewContext(event3)

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, correlationKeyValue, correlationKeyFromFirstRule)
	assert.True(t, set)

	if !rs.Evaluate(event3) {
		t.Errorf("Expected event to match rule")
	}

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.True(t, strings.HasPrefix(correlationKeyValue.(string), "third_"))
	assert.True(t, set)

	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx)
	assert.True(t, len(parentCorrelationKeysValue.([]string)) == 1 && slices.Contains(parentCorrelationKeysValue.([]string), correlationKeyFromFirstRule))

	correlationKeyFromThirdRule := correlationKeyValue.(string)

	// trigger the second rule, make sure nothing changes
	event4 := fakeOpenEvent("/tmp/second", 4, event3.ProcessCacheEntry)
	ctx = eval.NewContext(event4)

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, correlationKeyValue, correlationKeyFromThirdRule)
	assert.True(t, set)

	if rs.Evaluate(event4) {
		t.Errorf("Didn't expected event to match rule")
	}

	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, correlationKeyValue, correlationKeyFromThirdRule)
	assert.True(t, set)

	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx)
	assert.True(t, len(parentCorrelationKeysValue.([]string)) == 1 && slices.Contains(parentCorrelationKeysValue.([]string), correlationKeyFromFirstRule))
}

func newFakeCGroupWrite(cgroupWritePID int, path string, pid uint32, ancestor *model.ProcessCacheEntry) *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.CgroupWriteEventType)
	event.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{
					Pid: pid,
				},
			},
		},
	}
	event.ProcessCacheEntry.Retain()
	if ancestor != nil {
		event.ProcessCacheEntry.ProcessContext.Ancestor = ancestor
	}
	event.SetFieldValue("cgroup_write.pid", cgroupWritePID)
	event.SetFieldValue("cgroup_write.file.path", path)
	return event
}

func TestActionSetVariableScopeField(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "first_execution_context",
				Expression: `cgroup_write.file.path == "/tmp/one" && ${process.correlation_key} == ""`,
				Actions: []*ActionDefinition{
					{
						// This action should set the value or the correlation_key of the target process of the cgroup_write event
						Filter: stringPtr(`${process.correlation_key} == ""`),
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"first"`,
							Scope:        "process",
							ScopeField:   "cgroup_write.pid",
							Inherited:    true,
						},
					},
					{
						// This action should set the value or the correlation_key of the process doing the cgroup_write
						Filter: stringPtr(`${process.correlation_key} == ""`),
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"cgroup_write_first"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
			{
				ID:         "second_execution_context",
				Expression: `cgroup_write.file.path == "/tmp/two" && ${process.correlation_key} == "cgroup_write_first"`,
				Actions: []*ActionDefinition{
					{
						// This action should set the value or the correlation_key of the target process of the cgroup_write event
						Filter: stringPtr(`${process.correlation_key} == "first"`),
						Set: &SetDefinition{
							Name:         "parent_correlation_keys",
							DefaultValue: "",
							ScopeField:   "cgroup_write.pid",
							Expression:   "${process.correlation_key}",
							Scope:        "process",
							Append:       true,
							Inherited:    true,
						},
					},
					{
						// This action should set the value or the correlation_key of the target process of the cgroup_write event
						Filter: stringPtr(`${process.correlation_key} == "first"`),
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"second"`,
							Scope:        "process",
							ScopeField:   "cgroup_write.pid",
							Inherited:    true,
						},
					},
					{
						// This action should set the value or the correlation_key of the target process of the cgroup_write event
						Filter: stringPtr(`${process.correlation_key} == "cgroup_write_first"`),
						Set: &SetDefinition{
							Name:         "parent_correlation_keys",
							DefaultValue: "",
							Expression:   "${process.correlation_key}",
							Scope:        "process",
							Append:       true,
							Inherited:    true,
						},
					},
					{
						// This action should set the value or the correlation_key of the process doing the cgroup_write
						Filter: stringPtr(`${process.correlation_key} == "cgroup_write_first"`),
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"cgroup_write_second"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
			{
				ID:         "third_execution_context",
				Expression: `open.file.path == "/tmp/third" && ${process.correlation_key} == "second"`,
				Actions: []*ActionDefinition{
					{
						// This action should set the value or the correlation_key of the target process of the cgroup_write event
						Filter: stringPtr(`${process.correlation_key} == "second"`),
						Set: &SetDefinition{
							Name:         "parent_correlation_keys",
							DefaultValue: "",
							Expression:   "${process.correlation_key}",
							Scope:        "process",
							Append:       true,
							Inherited:    true,
						},
					},
					{
						Set: &SetDefinition{
							Name:         "correlation_key",
							DefaultValue: "",
							Expression:   `"third"`,
							Scope:        "process",
							Inherited:    true,
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Fatal(err)
	}

	opts := rs.evalOpts

	// Fetch process.correlation_key variable
	correlationKeySECLVariable := opts.VariableStore.Get("process.correlation_key")
	assert.NotNil(t, correlationKeySECLVariable)
	correlationKeyScopedVariable, ok := correlationKeySECLVariable.(eval.ScopedVariable)
	assert.NotNil(t, correlationKeyScopedVariable)
	assert.True(t, ok)

	// Fetch process.parent_correlation_keys variable
	parentCorrelationKeysSECLVariable := opts.VariableStore.Get("process.parent_correlation_keys")
	assert.NotNil(t, parentCorrelationKeysSECLVariable)
	parentCorrelationKeysScopedVariable, ok := parentCorrelationKeysSECLVariable.(eval.ScopedVariable)
	assert.NotNil(t, parentCorrelationKeysScopedVariable)
	assert.True(t, ok)

	// create cgroup_write event
	event1 := newFakeCGroupWrite(2, "/tmp/one", 1, nil)
	event2 := newFakeCGroupWrite(2, "/tmp/two", 1, nil)
	eventPID2 := newFakeCGroupWrite(0, "", 2, nil)
	event3 := fakeOpenEvent("/tmp/third", 3, eventPID2.ProcessCacheEntry)
	ctx1 := eval.NewContext(event1)
	ctx3 := eval.NewContext(event3)

	if !rs.Evaluate(event1) {
		t.Errorf("Expected event1 to match a rule")
	}

	// check the correlation_key of the current process
	correlationKeyValue, set := correlationKeyScopedVariable.GetValue(ctx1)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "cgroup_write_first", correlationKeyValue.(string))
	assert.True(t, set)

	// check the correlation key of the PID from the cgroup_write
	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx3)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "first", correlationKeyValue.(string))
	assert.True(t, set)

	if !rs.Evaluate(event2) {
		t.Errorf("Expected event2 to match a rule")
	}

	// check the correlation_key of the current process
	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx1)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "cgroup_write_second", correlationKeyValue.(string))
	assert.True(t, set)

	// check the parent_correlation_keys of the current process
	parentCorrelationKeysValue, _ := parentCorrelationKeysScopedVariable.GetValue(ctx1)
	assert.Equal(t, []string{"cgroup_write_first"}, parentCorrelationKeysValue.([]string))

	// check the correlation key of the PID from the cgroup_write
	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx3)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "second", correlationKeyValue.(string))
	assert.True(t, set)

	// check the parent_correlation_keys of the PID from the cgroup_write
	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx3)
	assert.Equal(t, []string{"first"}, parentCorrelationKeysValue.([]string))

	if !rs.Evaluate(event3) {
		t.Errorf("Expected event3 to match a rule")
	}

	// check the correlation_key of the current process
	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx1)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "cgroup_write_second", correlationKeyValue.(string))
	assert.True(t, set)

	// check the parent_correlation_keys of the current process
	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx1)
	assert.Equal(t, []string{"cgroup_write_first"}, parentCorrelationKeysValue.([]string))

	// check the correlation key of the PID from the cgroup_write
	correlationKeyValue, set = correlationKeyScopedVariable.GetValue(ctx3)
	assert.NotNil(t, correlationKeyValue)
	assert.Equal(t, "third", correlationKeyValue.(string))
	assert.True(t, set)

	// check the parent_correlation_keys of the PID from the cgroup_write
	parentCorrelationKeysValue, _ = parentCorrelationKeysScopedVariable.GetValue(ctx3)
	assert.ElementsMatch(t, []string{"first", "second"}, parentCorrelationKeysValue.([]string))
}

func TestActionSetVariableExpression(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:         "var1",
							DefaultValue: 123,
							Expression:   "${var1} + ${var1} + 1",
						},
					},
					{
						Set: &SetDefinition{
							Name:         "var2",
							Value:        "foo",
							DefaultValue: "",
						},
					},
					{
						Set: &SetDefinition{
							Name:         "var3",
							Expression:   `"${var2}:${var2}"`,
							DefaultValue: "",
						},
					},
				},
			},
			{
				ID:         "test_rule_connect",
				Expression: `connect.addr.ip == 192.168.1.0/24`,
				Actions: []*ActionDefinition{
					{
						Set: &SetDefinition{
							Name:  "connected",
							Value: true,
						},
					},
					{
						Set: &SetDefinition{
							Name:  "connected_to",
							Field: "connect.addr.ip",
						},
					},
					{
						Set: &SetDefinition{
							Name:         "connected_to_check",
							Expression:   "${connected_to} == 192.168.1.1/32",
							DefaultValue: false,
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	opts := rs.evalOpts

	existingVariable := opts.VariableStore.Get("var1")
	assert.NotNil(t, existingVariable)

	existingVariable2 := opts.VariableStore.Get("var3")
	assert.NotNil(t, existingVariable2)

	existingVariable3 := opts.VariableStore.Get("connected")
	assert.NotNil(t, existingVariable3)

	existingVariable4 := opts.VariableStore.Get("connected_to")
	assert.NotNil(t, existingVariable4)

	intVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, intVar)
	assert.True(t, ok)
	value, set := intVar.GetValue()
	assert.NotNil(t, value)
	assert.False(t, set)

	strVar, ok := existingVariable2.(eval.Variable)
	assert.NotNil(t, strVar)
	assert.True(t, ok)
	value, set = strVar.GetValue()
	assert.NotNil(t, value)
	assert.False(t, set)

	connectedVar, ok := existingVariable3.(eval.Variable)
	assert.NotNil(t, connectedVar)
	assert.True(t, ok)
	value, set = connectedVar.GetValue()
	assert.NotNil(t, value)
	assert.False(t, set)

	connectedToVar, ok := existingVariable4.(eval.Variable)
	assert.NotNil(t, connectedToVar)
	assert.True(t, ok)
	value, set = connectedToVar.GetValue()
	assert.NotNil(t, value)
	assert.False(t, set)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set = intVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, 247, value)

	value, set = strVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, "foo:foo", value)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	value, set = intVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, 495, value)

	value, set = strVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, "foo:foo", value)

	event2 := model.NewFakeEvent()
	event2.Type = uint32(model.ConnectEventType)
	processCacheEntry = &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event2.ProcessCacheEntry = processCacheEntry
	connectIP := net.IPNet{
		IP:   net.IPv4(192, 168, 1, 1),
		Mask: net.IPv4Mask(255, 255, 255, 0),
	}
	event2.SetFieldValue("connect.addr.ip", connectIP)

	if !rs.Evaluate(event2) {
		t.Errorf("Expected event to match rule")
	}

	value, set = connectedVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, true, value)

	value, set = connectedToVar.GetValue()
	assert.True(t, set)
	assert.Equal(t, []net.IPNet{{
		IP:   net.IPv4(192, 168, 1, 0).To4(),
		Mask: connectIP.Mask,
	}}, value)
}

func loadPolicy(t *testing.T, testPolicy *PolicyDef, policyOpts PolicyLoaderOpts) (*RuleSet, *multierror.Error) {
	rs := newRuleSet()

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewPolicyLoader(provider)

	_, errs := rs.LoadPolicies(loader, policyOpts)

	return rs, errs
}

func TestRuleErrorLoading(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "testA",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "testB",
				Expression: `open.file.path =-= "/tmp/test"`,
			},
			{
				ID:         "testA",
				Expression: `open.file.path == "/tmp/toto"`,
			},
		},
	}

	rs, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{})
	assert.NotNil(t, err)
	assert.Len(t, err.Errors, 1)
	assert.ErrorContains(t, err.Errors[0], "rule `testB` error: syntax error `1:17: unexpected token \"-\" (expected \"~\")`")

	assert.Contains(t, rs.rules, "testA")
	assert.NotContains(t, rs.rules, "testB")
}

func TestRuleAgentConstraint(t *testing.T) {
	testPolicy := &PolicyDef{
		Macros: []*MacroDefinition{
			{
				ID:         "macro1",
				Expression: `[1, 2]`,
			},
			{
				ID:                     "macro2",
				Expression:             `[3, 4]`,
				AgentVersionConstraint: ">= 7.37, < 7.38",
			},
			{
				ID:                     "macro2",
				Expression:             `[3, 4, 5]`,
				AgentVersionConstraint: ">= 7.38",
			},
		},
		Rules: []*RuleDefinition{
			{
				ID:         "no_constraint",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:                     "conflict",
				Expression:             `open.file.path == "/tmp/test1"`,
				AgentVersionConstraint: "< 7.37",
			},
			{
				ID:                     "conflict",
				Expression:             `open.file.path == "/tmp/test2"`,
				AgentVersionConstraint: ">= 7.37",
			},
			{
				ID:                     "basic",
				Expression:             `open.file.path == "/tmp/test"`,
				AgentVersionConstraint: "< 7.37",
			},
			{
				ID:                     "basic2",
				Expression:             `open.file.path == "/tmp/test"`,
				AgentVersionConstraint: "> 7.37",
			},
			{
				ID:                     "range",
				Expression:             `open.file.path == "/tmp/test"`,
				AgentVersionConstraint: ">= 7.30, < 7.39",
			},
			{
				ID:                     "range_not",
				Expression:             `open.file.path == "/tmp/test"`,
				AgentVersionConstraint: ">= 7.30, < 7.39, != 7.38",
			},
			{
				ID:                     "rc_prerelease",
				Expression:             `open.file.path == "/tmp/test"`,
				AgentVersionConstraint: ">= 7.38",
			},
			{
				ID:                     "with_macro1",
				Expression:             `open.file.path == "/tmp/test" && open.mode in macro1`,
				AgentVersionConstraint: ">= 7.38",
			},
			{
				ID:                     "with_macro2",
				Expression:             `open.file.path == "/tmp/test" && open.mode in macro2`,
				AgentVersionConstraint: ">= 7.38",
			},
		},
	}

	expected := []struct {
		ruleID       string
		expectedLoad bool
	}{
		{
			ruleID:       "no_constraint",
			expectedLoad: true,
		},
		{
			ruleID:       "conflict",
			expectedLoad: true,
		},
		{
			ruleID:       "basic",
			expectedLoad: false,
		},
		{
			ruleID:       "basic2",
			expectedLoad: true,
		},
		{
			ruleID:       "range",
			expectedLoad: true,
		},
		{
			ruleID:       "range_not",
			expectedLoad: false,
		},
		{
			ruleID:       "rc_prerelease",
			expectedLoad: true,
		},
		{
			ruleID:       "with_macro1",
			expectedLoad: true,
		},
		{
			ruleID:       "with_macro2",
			expectedLoad: true,
		},
	}

	agentVersion, err := semver.NewVersion("7.38")
	assert.NoError(t, err)

	agentVersionFilter, err := NewAgentVersionFilter(agentVersion)
	assert.NoError(t, err)

	policyOpts := PolicyLoaderOpts{
		MacroFilters: []MacroFilter{
			agentVersionFilter,
		},
		RuleFilters: []RuleFilter{
			agentVersionFilter,
		},
	}

	rs, rsErr := loadPolicy(t, testPolicy, policyOpts)
	if rsErr != nil {
		t.Fatalf("unexpected error: %v\n", rsErr)
	}

	for _, exp := range expected {
		t.Run(exp.ruleID, func(t *testing.T) {
			if exp.expectedLoad {
				assert.Contains(t, rs.rules, exp.ruleID)
			} else {
				assert.NotContains(t, rs.rules, exp.ruleID)
			}
		})
	}
}

func TestActionSetVariableInvalid(t *testing.T) {
	t.Run("both-field-and-value", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []string{"abc"},
						Field: "open.file.path",
					},
				}},
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("policy should fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("bool-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []bool{true},
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.file.path == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("heterogeneous-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []interface{}{"string", true},
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.file.path == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("nil-values", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: nil,
					},
				}},
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("append-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:   "var1",
						Value:  []string{"abc"},
						Append: true,
					},
				}, {
					Set: &SetDefinition{
						Name:   "var1",
						Value:  true,
						Append: true,
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.file.path == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("conflicting-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Field: "open.file.path",
					},
				}, {
					Set: &SetDefinition{
						Name:   "var1",
						Value:  true,
						Append: true,
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.file.path == "/tmp/test2" && ` +
					`${var1} == "true"`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("conflicting-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:   "var1",
						Field:  "open.file.path",
						Append: true,
					},
				}, {
					Set: &SetDefinition{
						Name:   "var1",
						Field:  "process.is_root",
						Append: true,
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.file.path == "/tmp/test2" && ` +
					`${var1} == "true"`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})
}

func TestActionSetVariableLength(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []*ActionDefinition{
				{
					Set: &SetDefinition{
						Name:  "var1",
						Value: "foo",
					},
				},
				{
					Set: &SetDefinition{
						Name:         "var2",
						Expression:   "${var1.length}",
						DefaultValue: 0,
					},
				},
				{
					Set: &SetDefinition{
						Name:   "var3",
						Append: true,
						Value:  1,
					},
				},
				{
					Set: &SetDefinition{
						Name:   "var3",
						Append: true,
						Value:  2,
					},
				},
				{
					Set: &SetDefinition{
						Name:         "var4",
						Expression:   "${var3.length}",
						DefaultValue: 0,
					},
				},
			},
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	rs := newRuleSet()
	if _, err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err != nil {
		t.Error(err)
	}

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ContainerContext = &model.ContainerContext{
		ContainerID: "0123456789abcdef",
	}
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	opts := rs.evalOpts

	existingVariable := opts.VariableStore.Get("var2")
	assert.NotNil(t, existingVariable)
	intArrayVar, ok := existingVariable.(eval.Variable)
	assert.NotNil(t, intArrayVar)
	assert.True(t, ok)
	intValue, _ := intArrayVar.GetValue()
	assert.NotNil(t, intValue)
	assert.Equal(t, 3, intValue)

	existingVariable = opts.VariableStore.Get("var4")
	assert.NotNil(t, existingVariable)
	intArrayVar, ok = existingVariable.(eval.Variable)
	assert.NotNil(t, intArrayVar)
	assert.True(t, ok)
	intValue, _ = intArrayVar.GetValue()
	assert.NotNil(t, intValue)
	assert.Equal(t, 2, intValue)
}

// go test -v github.com/DataDog/datadog-agent/pkg/security/secl/rules --run="TestLoadPolicy"
func TestLoadPolicy(t *testing.T) {
	type args struct {
		name         string
		policyType   PolicyType
		source       string
		fileContent  string
		macroFilters []MacroFilter
		ruleFilters  []RuleFilter
	}
	tests := []struct {
		name    string
		args    args
		want    *Policy
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "empty yaml file",
			args: args{
				name:         "myLocal.policy",
				source:       PolicyProviderTypeRC,
				policyType:   DefaultPolicyType,
				fileContent:  ``,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: nil,
			wantErr: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.EqualError(t, err, ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`EOF`)}.Error())
			},
		},
		{
			name: "empty yaml file with new line char",
			args: args{
				name:       "myLocal.policy",
				source:     PolicyProviderTypeRC,
				policyType: CustomPolicyType,
				fileContent: `
`,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: nil,
			wantErr: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.EqualError(t, err, ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`EOF`)}.Error())
			},
		},
		{
			name: "no rules in yaml file",
			args: args{
				name:       "myLocal.policy",
				source:     PolicyProviderTypeRC,
				policyType: CustomPolicyType,
				fileContent: `
rules:
`,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: &Policy{
				Info: PolicyInfo{
					Name:   "myLocal.policy",
					Source: PolicyProviderTypeRC,
					Type:   CustomPolicyType,
				},
				rules:  map[string][]*PolicyRule{},
				macros: map[string][]*PolicyMacro{},
			},
			wantErr: assert.NoError,
		},
		{
			name: "broken yaml file",
			args: args{
				name:       "myLocal.policy",
				source:     PolicyProviderTypeRC,
				policyType: CustomPolicyType,
				fileContent: `
broken
`,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: nil,
			wantErr: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.ErrorContains(t, err, ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`yaml: unmarshal error`)}.Error())
			},
		},
		{
			name: "disabled tag",
			args: args{
				name:       "myLocal.policy",
				source:     PolicyProviderTypeRC,
				policyType: CustomPolicyType,
				fileContent: `rules:
 - id: rule_test
   disabled: true
`,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: &Policy{
				Info: PolicyInfo{
					Name:   "myLocal.policy",
					Source: PolicyProviderTypeRC,
					Type:   CustomPolicyType,
				},
				rules: map[string][]*PolicyRule{
					"rule_test": {
						{
							Def: &RuleDefinition{
								ID:         "rule_test",
								Expression: "",
								Disabled:   true,
							},
							Policy: PolicyInfo{
								Name:   "myLocal.policy",
								Source: PolicyProviderTypeRC,
								Type:   CustomPolicyType,
							},
							Accepted: true,
						},
					},
				},
				macros: map[string][]*PolicyMacro{},
			},
			wantErr: assert.NoError,
		},
		{
			name: "combine:override tag",
			args: args{
				name:       "myLocal.policy",
				source:     PolicyProviderTypeRC,
				policyType: CustomPolicyType,
				fileContent: `rules:
 - id: rule_test
   expression: open.file.path == "/etc/gshadow"
   combine: override
`,
				macroFilters: nil,
				ruleFilters:  nil,
			},
			want: &Policy{
				Info: PolicyInfo{
					Name:   "myLocal.policy",
					Source: PolicyProviderTypeRC,
					Type:   CustomPolicyType,
				},
				rules: map[string][]*PolicyRule{
					"rule_test": {
						{
							Def: &RuleDefinition{
								ID:         "rule_test",
								Expression: "open.file.path == \"/etc/gshadow\"",
								Combine:    OverridePolicy,
							},
							Policy: PolicyInfo{
								Name:   "myLocal.policy",
								Source: PolicyProviderTypeRC,
								Type:   CustomPolicyType,
							},
							Accepted: true,
						},
					},
				},
				macros: map[string][]*PolicyMacro{},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.args.fileContent)

			info := &PolicyInfo{
				Name:   tt.args.name,
				Source: tt.args.source,
				Type:   tt.args.policyType,
			}

			got, err := LoadPolicy(info, r, tt.args.macroFilters, tt.args.ruleFilters)

			if !tt.wantErr(t, err, fmt.Sprintf("LoadPolicy(%v, %v, %v, %v, %v)", tt.args.name, tt.args.source, r, tt.args.macroFilters, tt.args.ruleFilters)) {
				return
			}

			if !cmp.Equal(tt.want, got, policyCmpOpts...) {
				t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(tt.want, got, policyCmpOpts...))
			}
		})
	}
}

// go test -v github.com/DataDog/datadog-agent/pkg/security/secl/rules --run="TestPolicySchema"
func TestPolicySchema(t *testing.T) {
	tests := []struct {
		name           string
		policy         string
		schemaResultCb func(*testing.T, *gojsonschema.Result)
	}{
		{
			name:   "valid",
			policy: policyValid,
			schemaResultCb: func(t *testing.T, result *gojsonschema.Result) {
				if !assert.True(t, result.Valid(), "schema validation failed") {
					for _, err := range result.Errors() {
						t.Errorf("%s", err)
					}
				}
			},
		},
		{
			name:   "missing required rule ID",
			policy: policyWithMissingRequiredRuleID,
			schemaResultCb: func(t *testing.T, result *gojsonschema.Result) {
				require.False(t, result.Valid(), "schema validation should fail")
				require.Len(t, result.Errors(), 1)
				assert.Contains(t, result.Errors()[0].String(), "id is required")
			},
		},
		{
			name:   "unknown field",
			policy: policyWithUnknownField,
			schemaResultCb: func(t *testing.T, result *gojsonschema.Result) {
				require.False(t, result.Valid(), "schema validation should fail")
				require.Len(t, result.Errors(), 1)
				assert.Contains(t, result.Errors()[0].String(), "Additional property unknown_field is not allowed")
			},
		},
		{
			name:   "invalid field type",
			policy: policyWithInvalidFieldType,
			schemaResultCb: func(t *testing.T, result *gojsonschema.Result) {
				require.False(t, result.Valid(), "schema validation should fail")
				require.Len(t, result.Errors(), 1)
				assert.Contains(t, result.Errors()[0].String(), "Invalid type")

			},
		},
		{
			name:   "multiple actions",
			policy: policyWithMultipleActions,
			schemaResultCb: func(t *testing.T, result *gojsonschema.Result) {
				require.False(t, result.Valid(), "schema validation should fail")
				require.Len(t, result.Errors(), 1)
				assert.Contains(t, result.Errors()[0].String(), "Must validate one and only one schema")
			},
		},
	}

	fs := os.DirFS("../../../../pkg/security/secl/schemas")
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem("file:///policy.schema.json", http.FS(fs))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			json, err := yamlk8s.YAMLToJSON([]byte(test.policy))
			require.NoErrorf(t, err, "failed to convert yaml to json: %v", err)
			documentLoader := gojsonschema.NewBytesLoader(json)
			result, err := gojsonschema.Validate(schemaLoader, documentLoader)
			require.NoErrorf(t, err, "failed to validate schema: %v", err)
			test.schemaResultCb(t, result)
		})
	}
}

const policyValid = `
version: 1.2.3
rules:
  - id: basic
    expression: exec.file.name == "foo"
  - id: with_tags
    description: Rule with tags
    expression: exec.file.name == "foo"
    tags:
      tagA: a
      tagB: b
  - id: disabled
    description: Disabled rule
    expression: exec.file.name == "foo"
    disabled: true
  - id: with_tags
    description: Rule with combine
    expression: exec.file.name == "bar"
    combine: override
    override_options:
      fields:
        - expression
  - id: with_filters
    description: Rule with a filter and agent_version field
    expression: exec.file.name == "foo"
    agent_version: ">= 7.38"
    filters:
      - os == "linux"
  - id: with_every_silent_group_id
    description: Rule with a silent/every/group_id field
    expression: exec.file.name == "foo"
    silent: true
    every: 10s
    group_id: "baz_group"
  - id: with_set_action_with_field
    description: Rule with a set action using an event field
    expression: exec.file.name == "foo"
    actions:
      - set:
          name: process_names
          field: process.file.name
          append: true
          size: 10
          ttl: 10s
  - id: with_set_action_with_value
    description: Rule with a set action using a value
    expression: exec.file.name == "foo"
    actions:
      - set:
          name: global_var_set
          value: true
  - id: with_set_action_use
    description: Rule using a variable set by a previous action
    expression: open.file.path == "/tmp/bar" && ${global_var_set}
  - id: with_kill_action
    description: Rule with a kill action
    expression: exec.file.name == "foo"
    actions:
      - kill:
          signal: SIGKILL
          scope: process
  - id: with_coredump_action
    description: Rule with a coredump action
    expression: exec.file.name == "foo"
    actions:
      - coredump:
          process: true
          dentry: true
          mount: true
          no_compression: true
  - id: with_hash_action
    description: Rule with a hash action
    expression: exec.file.name == "foo"
    actions:
      - hash: {}
`
const policyWithMissingRequiredRuleID = `
version: 1.2.3
rules:
  - description: Rule with missing ID
    expression: exec.file.name == "foo"
`

const policyWithUnknownField = `
version: 1.2.3
rules:
  - id: rule with unknown field
    expression: exec.file.name == "foo"
    unknown_field: "bar"
`

const policyWithInvalidFieldType = `
version: 1.2.3
rules:
  - id: 2
    expression: exec.file.name == "foo"
`

const policyWithMultipleActions = `
version: 1.2.3
rules:
  - id: rule with missing action
    expression: exec.file.name == "foo"
    actions:
      - set:
          name: global_var_set
          value: true
        kill:
          signal: SIGKILL
          scope: process
`
