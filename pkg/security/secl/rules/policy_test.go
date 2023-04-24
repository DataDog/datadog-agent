// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package rules

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

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
	rs := newRuleSet()
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

	event := model.NewDefaultEvent()
	event.SetFieldValue("open.file.path", "/tmp/test")
	event.SetFieldValue("process.comm", "/usr/bin/vi")

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() != nil {
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

	if err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err == nil {
		t.Error("expected macro ID conflict")
	}
}

func TestRuleMerge(t *testing.T) {
	rs := newRuleSet()

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
		}},
	}

	testPolicy2 := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Combine:    OverridePolicy,
		}},
	}

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}

	testPolicy2.Rules[0].Combine = ""

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	if err := rs.LoadPolicies(loader, PolicyLoaderOpts{}); err == nil {
		t.Error("expected rule ID conflict")
	}
}

func TestActionSetVariable(t *testing.T) {
	rs := newRuleSet()

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []ActionDefinition{{
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

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}

	event := model.NewDefaultEvent()
	event.(*model.Event).Type = uint32(model.FileOpenEventType)
	processCacheEntry := &model.ProcessCacheEntry{}
	processCacheEntry.Retain()
	event.(*model.Event).ProcessCacheEntry = processCacheEntry
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
	event.(*model.Event).ProcessCacheEntry.Release()
	assert.Equal(t, scopedVariables.Len(), 0)
}

func TestActionSetVariableConflict(t *testing.T) {
	rs := newRuleSet()

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.file.path == "/tmp/test"`,
			Actions: []ActionDefinition{{
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

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader, PolicyLoaderOpts{}); errs.ErrorOrNil() == nil {
		t.Error("expected policy to fail to load")
	}
}

func loadPolicy(t *testing.T, testPolicy *PolicyDef, policyOpts PolicyLoaderOpts) (*RuleSet, *multierror.Error) {
	rs := newRuleSet()

	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewPolicyLoader(provider)

	return rs, rs.LoadPolicies(loader, policyOpts)
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
	assert.Len(t, err.Errors, 2)
	assert.ErrorContains(t, err.Errors[0], "rule `testA` error: multiple definition with the same ID")
	assert.ErrorContains(t, err.Errors[1], "rule `testB` error: syntax error `1:17: unexpected token \"-\" (expected \"~\")`")

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
	assert.Nil(t, err)

	agentVersionFilter, err := NewAgentVersionFilter(agentVersion)
	assert.Nil(t, err)

	policyOpts := PolicyLoaderOpts{
		MacroFilters: []MacroFilter{
			agentVersionFilter,
		},
		RuleFilters: []RuleFilter{
			agentVersionFilter,
		},
	}

	rs, err := loadPolicy(t, testPolicy, policyOpts)
	for _, err := range err.(*multierror.Error).Errors {
		if rerr, ok := err.(*ErrRuleLoad); ok {
			if rerr.Definition.ID != "basic" && rerr.Definition.ID != "range_not" {
				t.Errorf("unexpected error: %v", rerr)
			}
		}
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

func TestRuleIDFilter(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test1",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "test2",
				Expression: `open.file.path != "/tmp/test"`,
			},
		},
	}

	policyOpts := PolicyLoaderOpts{
		RuleFilters: []RuleFilter{
			&RuleIDFilter{
				ID: "test2",
			},
		},
	}

	rs, err := loadPolicy(t, testPolicy, policyOpts)
	assert.Nil(t, err)

	assert.NotContains(t, rs.rules, "test1")
	assert.Contains(t, rs.rules, "test2")
}

func TestActionSetVariableInvalid(t *testing.T) {
	t.Run("both-field-and-value", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.file.path == "/tmp/test"`,
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
				Actions: []ActionDefinition{{
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
