// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"os"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestVariableAnyField(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID: "test_rule_field_variable",
		// TODO(lebauce): should infer event type from variable usage
		Expression: `open.file.path != "" && "%{open.file.path}:foo" == "{{.Root}}/test-open:foo"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename1 string

	test.WaitSignalFromRule(t, func() error {
		filename1, _, err = test.Create("test-open")
		return err
	}, func(_ *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_field_variable", rule.ID, "wrong rule triggered")
	}, "test_rule_field_variable")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename1)
}

func TestVariablePrivateField(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule_private_variable",
		Expression: `open.file.path == "{{.Root}}/test-private-var"`,
		Actions: []*rules.ActionDefinition{
			{
				Set: &rules.SetDefinition{
					Name:  "public_var",
					Value: true,
				},
			},
			{
				Set: &rules.SetDefinition{
					Name:    "private_var",
					Value:   true,
					Private: true,
				},
			},
		},
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename string

	test.WaitSignalFromRule(t, func() error {
		filename, _, err = test.Create("test-private-var")
		return err
	}, func(_ *model.Event, _ *rules.Rule) {}, "test_rule_private_variable")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename)

	err = retry.Do(func() error {
		msg := test.msgSender.getMsg("test_rule_private_variable")
		if msg == nil {
			return errors.New("message not found")
		}

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			if _, err := jsonpath.JsonPathLookup(obj, `$.evt.variables.public_var`); err != nil {
				t.Errorf("public variable should be present in serialized event: %v", err)
			}
			if _, err := jsonpath.JsonPathLookup(obj, `$.evt.variables.private_var`); err == nil {
				t.Errorf("private variable should not be present in serialized event")
			}
		})

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
