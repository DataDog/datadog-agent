// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSyscallBlockingSystem(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_open_and_block",
			Expression: `open.file.name == "foobar"`,
			Actions: []rules.ActionDefinition{
				{
					Block: &rules.BlockDefinition{
						Syscalls: true,
					},
				},
			},
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

	t.Run("syscallblock-open", func(t *testing.T) {
		testDir := t.TempDir()
		foobarPath := filepath.Join(testDir, "foobar")
		foobazPath := filepath.Join(testDir, "foobaz")
		args := []string{"open", foobarPath, ";", "sleep", "1", ";", "open", foobazPath}

		test.WaitSignal(t, func() error {
			cmd := exec.Command(syscallTester, args...)
			return cmd.Start()
		}, func(event *model.Event, r *rules.Rule) { // catching the first sig
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.Signal.Retval, "wrong retval")
		})
		_, err := os.Stat(foobarPath) // should exist
		assert.Equal(t, nil, err, "foobar should exist")

		time.Sleep(time.Second * 2)
		_, err = os.Stat(foobazPath) // should NOT exist
		assert.NotEqual(t, nil, err, "foobaz should not exist")
	})
}
