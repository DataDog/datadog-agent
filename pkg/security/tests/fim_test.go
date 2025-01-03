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

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestFIMOpen(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_fim_rule",
			Expression: `fim.write.file.path == "{{.Root}}/test-open"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// open test
	test.WaitSignal(t, func() error {
		f, err := os.Create(testFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assertTriggeredRule(t, rule, "__fim_expanded_open__test_fim_rule")
		assert.Equal(t, rule.Def.ID, "test_fim_rule")
		assertInode(t, event.Open.File.Inode, getInode(t, testFile))
	})

	// chmod test
	test.WaitSignal(t, func() error {
		return os.Chmod(testFile, 0o777)
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "chmod", event.GetType(), "wrong event type")
		assertTriggeredRule(t, rule, "__fim_expanded_chmod__test_fim_rule")
		assert.Equal(t, rule.Def.ID, "test_fim_rule")
		assertInode(t, event.Chmod.File.Inode, getInode(t, testFile))
	})

	// open but read only
	_ = test.GetSignal(t, func() error {
		f, err := os.Open(testFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(_ *model.Event, _ *rules.Rule) {
		t.Error("Event received (rule is in write only mode, and the open is read only)")
	})
}
