// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"
	"time"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestRace(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("openat", func(t *testing.T) {
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			_, _, _ = syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			time.Sleep(2 * time.Second)
			t.Skip("skip is part of the test")
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			t.Error("should not be called")
		})
	})
}
