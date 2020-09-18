// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMacros(t *testing.T) {
	macros := []*rules.MacroDefinition{
		{
			ID:         "testmacro",
			Expression: `"{{.Root}}/test-macro"`,
		},
		{
			ID:         "testmacro2",
			Expression: `["{{.Root}}/test-macro"]`,
		},
	}

	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `testmacro in testmacro2 && mkdir.filename in testmacro2`,
		},
	}

	test, err := newTestModule(macros, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-macro")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(testFile, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "mkdir" {
			t.Errorf("expected mkdir event, got %s", event.GetType())
		}
	}
}
