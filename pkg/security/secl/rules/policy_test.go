// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func savePolicy(filename string, testPolicy *PolicyDef) error {
	yamlBytes, err := yaml.Marshal(testPolicy)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, yamlBytes, 0700)
}

func TestMacroMerge(t *testing.T) {
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(map[eval.EventType]bool{"*": true})

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test" && process.name == "/usr/bin/vim"`,
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

	tmpDir, err := os.MkdirTemp("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	rs.Evaluate(&testEvent{
		open: testOpen{
			filename: "/tmp/test",
		},
		process: testProcess{
			name: "/usr/bin/vi",
		},
	})

	provider, err := NewPoliciesDirProvider(tmpDir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	macro := rs.opts.Macros["test_macro"]
	if macro == nil {
		t.Fatalf("failed to find test_macro in ruleset: %+v", rs.opts.Macros)
	}

	testPolicy2.Macros[0].Combine = ""

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	if err := rs.LoadPolicies(loader); err == nil {
		t.Error("expected macro ID conflict")
	}
}

func TestRuleMerge(t *testing.T) {
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(map[eval.EventType]bool{"*": true})
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
		}},
	}

	testPolicy2 := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
			Combine:    OverridePolicy,
		}},
	}

	tmpDir, err := os.MkdirTemp("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test2.policy"), testPolicy2); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader); errs.ErrorOrNil() != nil {
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

	if err := rs.LoadPolicies(loader); err == nil {
		t.Error("expected rule ID conflict")
	}
}

type testVariableProvider struct {
	vars map[string]map[string]interface{}
}

func (t *testVariableProvider) GetVariable(name string, value interface{}) (eval.VariableValue, error) {
	switch value.(type) {
	case []int:
		intVar := eval.NewIntArrayVariable(func(ctx *eval.Context) []int {
			processName := (*testEvent)(ctx.Object).process.name
			processVars, found := t.vars[processName]
			if !found {
				return nil
			}

			v, found := processVars[name]
			if !found {
				return nil
			}

			i, _ := v.([]int)
			return i
		}, func(ctx *eval.Context, value interface{}) error {
			processName := (*testEvent)(ctx.Object).process.name
			if _, found := t.vars[processName]; !found {
				t.vars[processName] = map[string]interface{}{}
			}

			t.vars[processName][name] = value
			return nil
		})
		return intVar, nil
	default:
		return nil, fmt.Errorf("unsupported variable '%s'", name)
	}
}

func TestActionSetVariable(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}
	stateScopes := map[Scope]VariableProviderFactory{
		"process": func() VariableProvider {
			return &testVariableProvider{
				vars: map[string]map[string]interface{}{},
			}
		},
	}
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithVariables(make(map[string]eval.VariableValue)).
		WithStateScopes(stateScopes).
		WithMacros(make(map[eval.MacroID]*eval.Macro))
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
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
					Field: "open.filename",
				},
			}, {
				Set: &SetDefinition{
					Name:   "var10",
					Field:  "open.filename",
					Append: true,
				},
			}},
		}, {
			ID: "test_rule2",
			Expression: `open.filename == "/tmp/test2" && ` +
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

	tmpDir, err := os.MkdirTemp("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader); errs.ErrorOrNil() != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}

	event := &testEvent{
		process: testProcess{
			uid:  0,
			name: "myprocess",
		},
	}

	ev1 := *event
	ev1.kind = "open"
	ev1.open = testOpen{
		filename: "/tmp/test2",
		flags:    syscall.O_RDONLY,
	}

	if rs.Evaluate(event) {
		t.Errorf("Expected event to match no rule")
	}

	ev1.open.filename = "/tmp/test"

	if !rs.Evaluate(&ev1) {
		t.Errorf("Expected event to match rule")
	}

	ev1.open.filename = "/tmp/test2"
	if !rs.Evaluate(&ev1) {
		t.Errorf("Expected event to match rule")
	}
}

func TestActionSetVariableConflict(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithVariables(make(map[string]eval.VariableValue)).
		WithMacros(make(map[eval.MacroID]*eval.Macro))
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
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
			Expression: `open.filename == "/tmp/test2" && ` +
				`${var1} == true`,
		}},
	}

	tmpDir, err := os.MkdirTemp("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewPolicyLoader(provider)

	if errs := rs.LoadPolicies(loader); errs.ErrorOrNil() == nil {
		t.Error("expected policy to fail to load")
	}
}

func loadPolicy(t *testing.T, testPolicy *PolicyDef, agentVersion *semver.Version) (*RuleSet, *multierror.Error) {
	enabled := map[eval.EventType]bool{"*": true}
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithVariables(make(map[string]eval.VariableValue)).
		WithMacros(make(map[eval.MacroID]*eval.Macro))
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	tmpDir, err := os.MkdirTemp("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false, agentVersion)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewPolicyLoader(provider)

	return rs, rs.LoadPolicies(loader)
}

func TestRuleAgentConstraint(t *testing.T) {

	testEntries := []struct {
		name           string
		agentVersion   string
		ruleConstraint string
		expectLoad     bool
	}{
		{
			name:           "basic",
			agentVersion:   "7.38",
			ruleConstraint: "< 7.37",
			expectLoad:     false,
		},
		{
			name:           "basic2",
			agentVersion:   "7.35",
			ruleConstraint: "< 7.37",
			expectLoad:     true,
		},
		{
			name:           "range",
			agentVersion:   "7.35",
			ruleConstraint: ">= 7.30, < 7.37",
			expectLoad:     true,
		},
		{
			name:           "range_not",
			agentVersion:   "7.35",
			ruleConstraint: ">= 7.30, < 7.37, != 7.35",
			expectLoad:     false,
		},
	}

	for _, entry := range testEntries {
		t.Run(entry.name, func(t *testing.T) {
			ruleID := fmt.Sprintf("test_rule_%s", entry.name)

			testPolicy := &PolicyDef{
				Rules: []*RuleDefinition{{
					ID:                     ruleID,
					Expression:             `open.filename == "/tmp/test"`,
					AgentVersionConstraint: entry.ruleConstraint,
				}},
			}

			agentVersion, err := semver.NewVersion(entry.agentVersion)
			assert.Nil(t, err)

			rs, err := loadPolicy(t, testPolicy, agentVersion)
			assert.Nil(t, err)

			if entry.expectLoad {
				assert.Contains(t, rs.rules, ruleID)
			} else {
				assert.NotContains(t, rs.rules, ruleID)
			}
		})
	}
}

func TestActionSetVariableInvalid(t *testing.T) {
	t.Run("both-field-and-value", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []string{"abc"},
						Field: "open.filename",
					},
				}},
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("policy should fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("bool-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []bool{true},
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.filename == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("heterogeneous-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: []interface{}{"string", true},
					},
				}},
			}, {
				ID: "test_rule2",
				Expression: `open.filename == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("nil-values", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Value: nil,
					},
				}},
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("append-array", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
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
				Expression: `open.filename == "/tmp/test2" && ` +
					`${var1} == true`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("conflicting-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:  "var1",
						Field: "open.filename",
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
				Expression: `open.filename == "/tmp/test2" && ` +
					`${var1} == "true"`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})

	t.Run("conflicting-field-type", func(t *testing.T) {
		testPolicy := &PolicyDef{
			Rules: []*RuleDefinition{{
				ID:         "test_rule",
				Expression: `open.filename == "/tmp/test"`,
				Actions: []ActionDefinition{{
					Set: &SetDefinition{
						Name:   "var1",
						Field:  "open.filename",
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
				Expression: `open.filename == "/tmp/test2" && ` +
					`${var1} == "true"`,
			}},
		}

		if _, err := loadPolicy(t, testPolicy, nil); err == nil {
			t.Error("expected policy to fail to load")
		} else {
			t.Log(err)
		}
	})
}
