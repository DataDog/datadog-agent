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

	test.WaitSignal(t, func() error {
		f, err := os.Create(testFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assertInode(t, event.Open.File.Inode, getInode(t, testFile))
	})
}
