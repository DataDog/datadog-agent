// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	iouring "github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
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
		{
			ID:         "test_splice_io_uring",
			Expression: `splice.file.name == "splice_test" && process.file.name == "testsuite"`,
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
		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "splice")
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "splice", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.Splice.PipeEntryFlag, "wrong pipe entry flag")
			assert.Equal(t, uint32(0), event.Splice.PipeExitFlag, "wrong pipe exit flag")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSpliceSchema(t, event)
		}, "test_splice")
	})

	t.Run("io_uring", func(t *testing.T) {
		checkKernelCompatibility(t, "io_uring splice needs Linux 5.7", func(kv *kernel.Version) bool {
			return kv.Code < kernel.Kernel5_7
		})

		testFile, _, err := test.Path("splice_test")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(testFile, []byte("splice me\n"), 0600); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		fd, err := unix.Open(testFile, unix.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

		var p [2]int
		if err := unix.Pipe(p[:]); err != nil {
			t.Fatal(err)
		}
		defer unix.Close(p[0])
		defer unix.Close(p[1])

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		// splice from the file (in) to the pipe (out), mirroring the syscall_tester
		prepRequest := ioUringPrepSplice(fd, p[1], 1)
		ch := make(chan iouring.Result, 1)

		test.WaitSignalFromRule(t, func() error {
			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			ret, err := ioUringResult(result)
			if err != nil {
				return fmt.Errorf("io_uring error: %w", err)
			}

			if ret < 0 {
				// On a supported kernel a negative result is a real failure, not a skip:
				// a malformed SQE would also return a negative errno and hide the gap.
				return fmt.Errorf("failed to splice with io_uring: %d", ret)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_splice_io_uring")
			assert.Equal(t, "splice", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.Splice.PipeEntryFlag, "wrong pipe entry flag")
			assert.Equal(t, uint32(0), event.Splice.PipeExitFlag, "wrong pipe exit flag")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, true, value.(bool), "io_uring splice event should be async")

			test.validateSpliceSchema(t, event)
		}, "test_splice_io_uring")
	})
}
