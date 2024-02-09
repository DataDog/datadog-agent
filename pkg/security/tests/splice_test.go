// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSpliceEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_splice",
			Expression: `splice.file.name == "splice_test" && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("test_splice", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "splice")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "splice", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.Splice.PipeEntryFlag, "wrong pipe entry flag")
			assert.Equal(t, uint32(0), event.Splice.PipeExitFlag, "wrong pipe exit flag")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSpliceSchema(t, event)
		})
	})
}
