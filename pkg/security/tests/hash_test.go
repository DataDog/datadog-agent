// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestHash(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_hash_exec",
			Expression: `exec.file.path == "{{.Root}}/test-hash-exec" && exec.file.hashes in ["sha1:da77c6e4b745629c0622d6927fbdf0ab49c1b0f5"]`,
		},
		{
			ID:         "test_rule_hash_fifo",
			Expression: `open.file.path == "{{.Root}}/test-hash-fifo" && open.file.hashes not in [r".*"]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("exec", func(t *testing.T) {
		testFile, _, err := test.Path("test-hash-exec")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(testFile, []byte("#!/bin/sh\necho malware\nsleep 3"), 0755); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testFile)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Logf("cmd.Run() failed with output: %s", string(out))
				return err
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assertTriggeredRule(t, r, "test_rule_hash_exec")
		})
	})

	t.Run("fifo", func(t *testing.T) {
		testFile, _, err := test.Path("test-hash-fifo")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			if err := syscall.Mknod(testFile, syscall.S_IFIFO|0666, 0); err != nil {
				return err
			}

			fd, err := syscall.Open(testFile, syscall.O_NONBLOCK, 0644)
			if err != nil {
				return err
			}
			syscall.Close(fd)

			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assertTriggeredRule(t, r, "test_rule_hash_fifo")
		})
	})
}
