// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
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
