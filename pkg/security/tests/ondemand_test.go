// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestOnDemandOpen(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_open",
			Expression: `ondemand.name == "do_sys_openat2" && ondemand.arg2.str =~ ~"*/test-open" && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{disableOnDemandRateLimiter: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint64(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-open", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	test.WaitSignalFromRule(t, func() error {
		openHow := unix.OpenHow{
			Flags: unix.O_RDONLY,
			Mode:  expectedMode,
		}
		fd, _, errno := syscall.Syscall6(unix.SYS_OPENAT2, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&openHow)), unix.SizeofOpenHow, 0, 0)
		if errno != 0 {
			if errno == unix.ENOSYS {
				return ErrSkipTest{"openat2 is not supported"}
			}
			return err
		}
		return syscall.Close(int(fd))
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")

		value, _ := event.GetFieldValue("ondemand.arg2.str")
		assert.Equal(t, testFile, value.(string))
	}, "test_rule_open")
}

func TestOnDemandChdir(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_chdir",
			Expression: `ondemand.name == "syscall:chdir" && ondemand.arg1.str != "" && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFolder, _, err := test.Path("test-chdir")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(testFolder, 0777); err != nil {
		t.Fatalf("failed to create directory: %s", err)
	}
	defer os.RemoveAll(testFolder)

	test.WaitSignalFromRule(t, func() error {
		return os.Chdir(testFolder)
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")

		value, _ := event.GetFieldValue("ondemand.arg1.str")
		assert.Equal(t, testFolder, value.(string))
	}, "test_rule_chdir")
}

func TestOnDemandMprotect(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mprotect",
			Expression: `ondemand.name == "security_file_mprotect" && (ondemand.arg3.uint & (PROT_READ|PROT_WRITE)) == (PROT_READ|PROT_WRITE) && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignalFromRule(t, func() error {
		var data []byte
		data, err = unix.Mmap(0, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANON)
		if err != nil {
			return fmt.Errorf("couldn't memory segment: %w", err)
		}

		if err = unix.Mprotect(data, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
			return fmt.Errorf("couldn't mprotect segment: %w", err)
		}

		if err := unix.Munmap(data); err != nil {
			return fmt.Errorf("couldn't unmap segment: %w", err)
		}

		return nil
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")
	}, "test_rule_mprotect")
}

func TestOnDemandCopyFileRange(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_copy_file_range",
			Expression: `ondemand.name == "syscall:copy_file_range" && ondemand.arg5.uint == 42 && ondemand.arg6.uint == 0 && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	f, err := os.CreateTemp("", "test-copy_file_range")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	test.WaitSignalFromRule(t, func() error {
		_, err := unix.CopyFileRange(int(f.Fd()), nil, int(f.Fd()), nil, 42, 0)
		if errors.Is(err, unix.ENOSYS) {
			return ErrSkipTest{"openat2 is not supported"}
		}
		return err
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")
	}, "test_rule_copy_file_range")
}
