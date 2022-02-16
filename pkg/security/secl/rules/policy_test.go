// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func savePolicy(filename string, testPolicy *Policy) error {
	yamlBytes, err := yaml.Marshal(testPolicy)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, yamlBytes, 0700)
}

func TestMacroMerge(t *testing.T) {
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(map[eval.EventType]bool{"*": true})

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)
	testPolicy := &Policy{
		Name: "test-policy",
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test" && process.name == "/usr/bin/vim"`,
		}},
		Macros: []*MacroDefinition{{
			ID:     "test_macro",
			Values: []string{"/usr/bin/vi"},
		}},
	}

	testPolicy2 := &Policy{
		Name: "test-policy2",
		Macros: []*MacroDefinition{{
			ID:      "test_macro",
			Values:  []string{"/usr/bin/vim"},
			Combine: MergePolicy,
		}},
	}

	tmpDir, err := ioutil.TempDir("", "test-policy")
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

	if err := LoadPolicies(tmpDir, rs); err != nil {
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

	if err := LoadPolicies(tmpDir, rs); err == nil {
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

	testPolicy := &Policy{
		Name: "test-policy",
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
		}},
	}

	testPolicy2 := &Policy{
		Name: "test-policy2",
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename == "/tmp/test"`,
			Combine:    OverridePolicy,
		}},
	}

	tmpDir, err := ioutil.TempDir("", "test-policy")
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

	if err := LoadPolicies(tmpDir, rs); err != nil {
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

	if err := LoadPolicies(tmpDir, rs); err == nil {
		t.Error("expected rule ID conflict")
	}
}
