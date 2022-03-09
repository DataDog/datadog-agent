// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSpliceEvent(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ptrace",
			Expression: `splice.file.name == "splice_test" && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
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
			return runSyscallTesterFunc(t, syscallTester, "splice")
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "splice", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.Splice.PipeBufFlag, "wrong pipe buf flag")

			if !validateSpliceSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
