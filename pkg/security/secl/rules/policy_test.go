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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/xeipuuv/gojsonschema"

	semver "github.com/Masterminds/semver/v3"
	multierror "github.com/hashicorp/go-multierror"
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

	var4Definition, ok := rs.evalOpts.NewStore.GetDefinition(eval.GetVariableName("process", "var4"))
	require.True(t, ok)
	require.NotNil(t, var4Definition)

	assert.Equal(t, var4Definition.GetInstancesCount(), 1)
	event.ProcessCacheEntry.Release()
	assert.Equal(t, var4Definition.GetInstancesCount(), 0)
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

	vStore := rs.evalOpts.NewStore

	definition, ok := vStore.GetDefinition("var1")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	stringArrayVar, err := definition.GetInstance(eval.NewContext(model.NewFakeEvent()))
	assert.NoError(t, err)
	assert.NotNil(t, stringArrayVar)
	value := stringArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []string{})
	assert.Contains(t, value, "foo")

	definition, ok = vStore.GetDefinition("var2")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	intArrayVar, err := definition.GetInstance(eval.NewContext(model.NewFakeEvent()))
	assert.NoError(t, err)
	assert.NotNil(t, intArrayVar)
	value = intArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []int{})
	assert.Contains(t, value, 123)

	ctx := eval.NewContext(event)

	definition, ok = vStore.GetDefinition("process.scopedvar1")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	stringArrayScopedVar, err := definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, stringArrayScopedVar)
	value = stringArrayScopedVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []string{})
	assert.Contains(t, value, "bar")

	definition, ok = vStore.GetDefinition("process.scopedvar2")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	intArrayScopedVar, err := definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, intArrayScopedVar)
	value = intArrayScopedVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []int{})
	assert.Contains(t, value, 123)

	definition, ok = vStore.GetDefinition("container.simplevarwithttl")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	intScopedVar, err := definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, intScopedVar)
	assert.Equal(t, 456, intScopedVar.GetValue())

	time.Sleep(time.Second + 100*time.Millisecond)

	value = stringArrayVar.GetValue()
	assert.NotContains(t, value, "foo")
	assert.Len(t, value, 0)

	value = intArrayVar.GetValue()
	assert.NotContains(t, value, 123)
	assert.Len(t, value, 0)

	value = stringArrayScopedVar.GetValue()
	assert.NotContains(t, value, "foo")
	assert.Len(t, value, 0)

	value = intArrayScopedVar.GetValue()
	assert.NotContains(t, value, 123)
	assert.Len(t, value, 0)

	assert.Equal(t, 0, intScopedVar.GetValue())
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

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	ctx := eval.NewContext(event)

	vStore := rs.evalOpts.NewStore

	var1Definition, ok := vStore.GetDefinition("var1")
	assert.True(t, ok)
	assert.NotNil(t, var1Definition)
	stringArrayVar, err := var1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, stringArrayVar)

	var2Definition, ok := vStore.GetDefinition("var2")
	assert.True(t, ok)
	assert.NotNil(t, var2Definition)
	intArrayVar, err := var2Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, intArrayVar)

	scopedVar1Definition, ok := vStore.GetDefinition("process.scopedvar1")
	assert.True(t, ok)
	assert.NotNil(t, scopedVar1Definition)
	stringArrayScopedVar, err := scopedVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, stringArrayScopedVar)

	scopedVar2Definition, ok := vStore.GetDefinition("process.scopedvar2")
	assert.True(t, ok)
	assert.NotNil(t, scopedVar2Definition)
	intArrayScopedVar, err := scopedVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, intArrayScopedVar)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}
	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	stringArrayVar, err = var1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, stringArrayVar)
	value := stringArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []string{})
	assert.Contains(t, value, "foo")
	assert.Len(t, value, 1)

	intArrayVar, err = var2Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, intArrayVar)
	value = intArrayVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []int{})
	assert.Contains(t, value, 1)
	assert.Len(t, value, 1)

	stringArrayScopedVar, err = scopedVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, stringArrayScopedVar)
	value = stringArrayScopedVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []string{})
	assert.Contains(t, value, "bar")
	assert.Len(t, value, 1)

	intArrayScopedVar, err = scopedVar2Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, intArrayScopedVar)
	value = intArrayScopedVar.GetValue()
	assert.NotNil(t, value)
	assert.IsType(t, value, []int{})
	assert.Contains(t, value, 123)
	assert.Len(t, value, 1)
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

	vStore := rs.evalOpts.NewStore

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.SetFieldValue("open.file.path", "/tmp/test")

	ctx := eval.NewContext(event)

	definition, ok := vStore.GetDefinition("process.scopedvar1")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	stringArrayScopedVar, err := definition.GetInstance(ctx)
	var expectedErr *eval.ErrScopeFailure
	assert.ErrorAs(t, err, &expectedErr)
	assert.Equal(t, expectedErr.VarName, "scopedvar1")
	assert.Equal(t, expectedErr.ScoperType, eval.ProcessScoperType)
	assert.Equal(t, expectedErr.ScoperErr.Error(), "failed to get process scope")
	assert.Nil(t, stringArrayScopedVar)
	assert.Equal(t, definition.GetInstancesCount(), 0)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	stringArrayScopedVar, err = definition.GetInstance(ctx)
	assert.ErrorAs(t, err, &expectedErr)
	assert.Equal(t, expectedErr.VarName, "scopedvar1")
	assert.Equal(t, expectedErr.ScoperType, eval.ProcessScoperType)
	assert.Equal(t, expectedErr.ScoperErr.Error(), "failed to get process scope")
	assert.Nil(t, stringArrayScopedVar)
	assert.Equal(t, definition.GetInstancesCount(), 0)
	assert.Nil(t, stringArrayScopedVar)
	assert.Equal(t, definition.GetInstancesCount(), 0)
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

	vStore := rs.evalOpts.NewStore

	evaluator, err := vStore.GetEvaluator("var1")
	assert.NoError(t, err)
	assert.NotNil(t, evaluator)
	intEvaluator, ok := evaluator.(*eval.IntEvaluator)
	assert.True(t, ok)
	if ok {
		value := intEvaluator.EvalFnc(eval.NewContext(model.NewFakeEvent()))
		assert.Equal(t, value, 123)
	}

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	definition, ok := vStore.GetDefinition("var1")
	assert.True(t, ok)
	assert.NotNil(t, definition)
	variable, err := definition.GetInstance(eval.NewContext(event))
	assert.NoError(t, err)
	assert.NotNil(t, variable)
	assert.Equal(t, 456, variable.GetValue())

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

	vStore := rs.evalOpts.NewStore

	processVar1Definition, ok := vStore.GetDefinition("process.var1")
	assert.True(t, ok)
	assert.NotNil(t, processVar1Definition)

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

	variable, err := processVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	// TODO(lebauce): should be 123. default_value are not properly handled
	assert.Nil(t, variable)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	variable, err = processVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, variable)
	assert.Equal(t, 456, variable.GetValue())

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

	variable, err = processVar1Definition.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, variable)
	assert.Equal(t, 1000, variable.GetValue())
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

	vStore := rs.evalOpts.NewStore

	correlationKeySECLVariableDef, ok := vStore.GetDefinition("process.correlation_key")
	assert.True(t, ok)
	assert.NotNil(t, correlationKeySECLVariableDef)

	parentCorrelationKeysSECLVariableDef, ok := vStore.GetDefinition("process.parent_correlation_keys")
	assert.True(t, ok)
	assert.NotNil(t, parentCorrelationKeysSECLVariableDef)

	event := fakeOpenEvent("/tmp/first", 1, nil)
	ctx := eval.NewContext(event)

	correlationKeyVariable, err := correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, correlationKeyVariable)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	correlationKeyFromFirstRule := correlationKeyVariable.GetValue().(string)
	assert.True(t, strings.HasPrefix(correlationKeyFromFirstRule, "first_"))

	assert.Equal(t, parentCorrelationKeysSECLVariableDef.GetInstancesCount(), 0)

	// trigger the first rule again, and make sure nothing changes
	event2 := fakeOpenEvent("/tmp/first", 2, event.ProcessCacheEntry)
	ctx = eval.NewContext(event2)

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, correlationKeyFromFirstRule, correlationKeyVariable.GetValue().(string))

	if rs.Evaluate(event2) {
		t.Errorf("Didn't expected event to match rule")
	}

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, correlationKeyFromFirstRule, correlationKeyVariable.GetValue().(string))

	assert.Equal(t, parentCorrelationKeysSECLVariableDef.GetInstancesCount(), 0)

	// jump to the third rule, check:
	//  - that the correlation key is updated with the pattern from the third rule
	//  - that the first correlation key is now in the "parent correlation keys" variable
	event3 := fakeOpenEvent("/tmp/third", 3, event2.ProcessCacheEntry)
	ctx = eval.NewContext(event3)

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, correlationKeyFromFirstRule, correlationKeyVariable.GetValue().(string))

	if !rs.Evaluate(event3) {
		t.Errorf("Expected event to match rule")
	}

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	correlationKeyFromThirdRule := correlationKeyVariable.GetValue().(string)
	assert.True(t, strings.HasPrefix(correlationKeyFromThirdRule, "third_"))

	parentCorrelationKeysSECLVariable, err := parentCorrelationKeysSECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, parentCorrelationKeysSECLVariable)
	parentCorrelationKeysValue := parentCorrelationKeysSECLVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 1)
	assert.Contains(t, parentCorrelationKeysValue, correlationKeyFromFirstRule)

	// trigger the second rule, make sure nothing changes
	event4 := fakeOpenEvent("/tmp/second", 4, event3.ProcessCacheEntry)
	ctx = eval.NewContext(event4)

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, correlationKeyFromThirdRule, correlationKeyVariable.GetValue().(string))

	if rs.Evaluate(event4) {
		t.Errorf("Didn't expected event to match rule")
	}

	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, correlationKeyFromThirdRule, correlationKeyVariable.GetValue().(string))

	parentCorrelationKeysSECLVariable, err = parentCorrelationKeysSECLVariableDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	parentCorrelationKeysValue = parentCorrelationKeysSECLVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 1)
	assert.Contains(t, parentCorrelationKeysValue, correlationKeyFromFirstRule)
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

	vStore := rs.evalOpts.NewStore

	// Fetch process.correlation_key variable
	correlationKeySECLVariableDef, ok := vStore.GetDefinition("process.correlation_key")
	assert.True(t, ok)
	assert.NotNil(t, correlationKeySECLVariableDef)

	// Fetch process.parent_correlation_keys variable
	parentCorrelationKeysSECLVariableDef, ok := vStore.GetDefinition("process.parent_correlation_keys")
	assert.True(t, ok)
	assert.NotNil(t, parentCorrelationKeysSECLVariableDef)

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
	correlationKeyVariable, err := correlationKeySECLVariableDef.GetInstance(ctx1)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "cgroup_write_first", correlationKeyVariable.GetValue().(string))

	// check the correlation key of the PID from the cgroup_write
	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx3)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "first", correlationKeyVariable.GetValue().(string))

	if !rs.Evaluate(event2) {
		t.Errorf("Expected event2 to match a rule")
	}

	// check the correlation_key of the current process
	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx1)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "cgroup_write_second", correlationKeyVariable.GetValue().(string))

	// check the parent_correlation_keys of the current process
	parentCorrelationKeysVariable, err := parentCorrelationKeysSECLVariableDef.GetInstance(ctx1)
	assert.NoError(t, err)
	assert.NotNil(t, parentCorrelationKeysVariable)
	parentCorrelationKeysValue := parentCorrelationKeysVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 1)
	assert.Contains(t, parentCorrelationKeysValue, "cgroup_write_first")

	// check the correlation key of the PID from the cgroup_write
	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx3)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "second", correlationKeyVariable.GetValue().(string))

	// check the parent_correlation_keys of the PID from the cgroup_write
	parentCorrelationKeysVariable, err = parentCorrelationKeysSECLVariableDef.GetInstance(ctx3)
	assert.NoError(t, err)
	assert.NotNil(t, parentCorrelationKeysVariable)
	parentCorrelationKeysValue = parentCorrelationKeysVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 1)
	assert.Contains(t, parentCorrelationKeysValue, "first")

	if !rs.Evaluate(event3) {
		t.Errorf("Expected event3 to match a rule")
	}

	// check the correlation_key of the current process
	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx1)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "cgroup_write_second", correlationKeyVariable.GetValue().(string))

	// check the parent_correlation_keys of the current process
	parentCorrelationKeysVariable, err = parentCorrelationKeysSECLVariableDef.GetInstance(ctx1)
	assert.NoError(t, err)
	assert.NotNil(t, parentCorrelationKeysVariable)
	parentCorrelationKeysValue = parentCorrelationKeysVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 1)
	assert.Contains(t, parentCorrelationKeysValue, "cgroup_write_first")

	// check the correlation key of the PID from the cgroup_write
	correlationKeyVariable, err = correlationKeySECLVariableDef.GetInstance(ctx3)
	assert.NoError(t, err)
	assert.NotNil(t, correlationKeyVariable)
	assert.Equal(t, "third", correlationKeyVariable.GetValue().(string))

	// check the parent_correlation_keys of the PID from the cgroup_write
	parentCorrelationKeysVariable, err = parentCorrelationKeysSECLVariableDef.GetInstance(ctx3)
	assert.NoError(t, err)
	assert.NotNil(t, parentCorrelationKeysVariable)
	parentCorrelationKeysValue = parentCorrelationKeysVariable.GetValue()
	assert.NotNil(t, parentCorrelationKeysValue)
	assert.IsType(t, parentCorrelationKeysValue, []string{})
	assert.Len(t, parentCorrelationKeysValue, 2)
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

	vStore := rs.evalOpts.NewStore
	ctx := eval.NewContext(model.NewFakeEvent())

	var1Def, ok := vStore.GetDefinition("var1")
	assert.True(t, ok)
	assert.NotNil(t, var1Def)

	var3Def, ok := vStore.GetDefinition("var3")
	assert.True(t, ok)
	assert.NotNil(t, var3Def)

	connectedDef, ok := vStore.GetDefinition("connected")
	assert.True(t, ok)
	assert.NotNil(t, connectedDef)

	connectedToDef, ok := vStore.GetDefinition("connected_to")
	assert.True(t, ok)
	assert.NotNil(t, connectedToDef)

	var1Var, err := var1Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, var1Var)

	var3Var, err := var3Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, var3Var)

	connectedVar, err := connectedDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, connectedVar)

	connectedToVar, err := connectedToDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.Nil(t, connectedToVar)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.ProcessCacheEntry = processCacheEntry
	event.SetFieldValue("open.file.path", "/tmp/test")

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	var1Var, err = var1Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var1Var)
	var1Val := var1Var.GetValue().(int)
	assert.Equal(t, 247, var1Val)

	var3Var, err = var3Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var3Var)
	var3Val := var3Var.GetValue().(string)
	assert.Equal(t, "foo:foo", var3Val)

	if !rs.Evaluate(event) {
		t.Errorf("Expected event to match rule")
	}

	var1Var, err = var1Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var1Var)
	var1Val = var1Var.GetValue().(int)
	assert.Equal(t, 495, var1Val)

	var3Var, err = var3Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var3Var)
	var3Val = var3Var.GetValue().(string)
	assert.Equal(t, "foo:foo", var3Val)

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

	connectedVar, err = connectedDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, connectedVar)
	connectedVal := connectedVar.GetValue().(bool)
	assert.Equal(t, true, connectedVal)

	connectedToVar, err = connectedToDef.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, connectedToVar)
	connectedToVal := connectedToVar.GetValue().([]net.IPNet)
	assert.Equal(t, connectedToVal, []net.IPNet{{
		IP:   net.IPv4(192, 168, 1, 0).To4(),
		Mask: connectIP.Mask,
	}})
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

func TestActionHashField(t *testing.T) {
	entries := []struct {
		name        string
		expr        string
		field       string
		errExpected bool
	}{
		{"valid", `open.file.path == "/tmp/test"`, "open.file", false},
		{"wrong field", `open.file.path == "/tmp/test"`, "open.file.path", true},
		{"incompatible field", `open.file.path == "/tmp/test"`, "chmod.file", true},
		{"wrong and incompatible", `open.file.path == "/tmp/test"`, "chmod.file.path", true},
		{"common field", `open.file.path == "/tmp/test"`, "process.file", false},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			testPolicy := &PolicyDef{
				Rules: []*RuleDefinition{{
					ID:         "test_rule",
					Expression: entry.expr,
					Actions: []*ActionDefinition{{
						Hash: &HashDefinition{
							Field: entry.field,
						},
					}},
				}},
			}

			if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); (err != nil) != entry.errExpected {
				t.Errorf("expected error: %v, got: %v", entry.errExpected, err)
			}
		})
	}
}

func TestActionSetVariableValidation(t *testing.T) {
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

	t.Run("incompatible-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:   "var1",
						Field:  "exec.file.path",
						Append: true,
					},
				},
				}},
			},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("compatible-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:   "var1",
						Field:  "process.file.path",
						Append: true,
					},
				},
				}},
			},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err != nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("incompatible-expression-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:         "var1",
						Expression:   "exec.file.path",
						Append:       true,
						DefaultValue: "",
					},
				},
				}},
			},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("compatible-expression-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []*ActionDefinition{{
					Set: &SetDefinition{
						Name:         "var1",
						Expression:   `"ssh_$${builtins.uuid4}_$${process.pid}"`,
						Append:       true,
						DefaultValue: "",
					},
				},
				}},
			},
		}

		if _, err := loadPolicy(t, testPolicy, PolicyLoaderOpts{}); err != nil {
			t.Errorf("expected policy to fail to load: %s", err)
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

	vStore := rs.evalOpts.NewStore
	ctx := eval.NewContext(model.NewFakeEvent())

	var2Def, ok := vStore.GetDefinition("var2")
	assert.True(t, ok)
	assert.NotNil(t, var2Def)
	var2Var, err := var2Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var2Var)
	var2Val := var2Var.GetValue().(int)
	assert.Equal(t, 3, var2Val)

	var4Def, ok := vStore.GetDefinition("var4")
	assert.True(t, ok)
	assert.NotNil(t, var4Def)
	var4Var, err := var4Def.GetInstance(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, var4Var)
	var4Val := var4Var.GetValue().(int)
	assert.Equal(t, 2, var4Val)
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
				return assert.Error(t, err, &ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`EOF`)})
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
				return assert.Error(t, err, &ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`EOF`)})
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
				return assert.ErrorContains(t, err, (&ErrPolicyLoad{Name: "myLocal.policy", Source: PolicyProviderTypeRC, Err: fmt.Errorf(`yaml: unmarshal error`)}).Error())
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
