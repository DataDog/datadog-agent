// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
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
	enabled := map[eval.EventType]bool{"*": true}
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, NewOptsWithParams(testConstants, testSupportedDiscarders, enabled, nil, nil, nil))

	testPolicy := &Policy{
		Name: "test-policy",
		Macros: []*MacroDefinition{{
			ID:     "test_macro",
			Values: []string{"/usr/bin/vi"},
		}, {
			ID:     "test_macro",
			Values: []string{"/usr/bin/vim"},
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

	if err := LoadPolicies(tmpDir, rs); err != nil {
		t.Error(err)
	}

	macro := rs.opts.Macros["test_macro"]
	if macro == nil {
		t.Fatalf("failed to find test_macro in ruleset: %+v", rs.opts.Macros)
	}

	sort.Strings(macro.Definition.Values)
	assert.Equal(t, macro.Definition.Values, []string{"/usr/bin/vi", "/usr/bin/vim"})

	falseBool := false
	testPolicy.Macros[1].Merge = &falseBool

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := LoadPolicies(tmpDir, rs); err == nil {
		t.Error("expected macro ID conflict")
	}
}

func TestRuleMerge(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, NewOptsWithParams(testConstants, testSupportedDiscarders, enabled, nil, nil, nil))

	trueBool := true

	testPolicy := &Policy{
		Name: "test-policy",
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename =~ "/sbin/*"`,
		}, {
			ID:         "test_rule",
			Expression: `&& process.uid != 0`,
			Merge:      &trueBool,
		}},
	}

	tmpDir, err := ioutil.TempDir("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := LoadPolicies(tmpDir, rs); err != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}

	if expectedExpression := testPolicy.Rules[0].Expression + " " + testPolicy.Rules[1].Expression; rule.Expression != expectedExpression {
		t.Errorf("expected expression to be %s, got %s", expectedExpression, rule.Expression)
	}

	testPolicy.Rules[1].Merge = nil

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := LoadPolicies(tmpDir, rs); err == nil {
		t.Error("expected rule ID conflict")
	}
}

func TestMacroInRuleMerge(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}
	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, NewOptsWithParams(testConstants, testSupportedDiscarders, enabled, nil, nil, nil))

	testPolicy := &Policy{
		Name: "test-policy",
		Macros: []*MacroDefinition{{
			ID:     "test_macro",
			Values: []string{"/usr/bin/vi"},
		}},
		Rules: []*RuleDefinition{{
			ID:         "test_rule",
			Expression: `open.filename in test_macro`,
		}},
	}

	tmpDir, err := ioutil.TempDir("", "test-policy")
	if err != nil {
		t.Fatal(err)
	}

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	if err := LoadPolicies(tmpDir, rs); err != nil {
		t.Error(err)
	}

	rule := rs.GetRules()["test_rule"]
	if rule == nil {
		t.Fatal("failed to find test_rule in ruleset")
	}
}
