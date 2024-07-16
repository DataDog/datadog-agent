// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
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

	onDemands := []rules.OnDemandHookPoint{
		{
			Name: "do_sys_openat2",
			Args: []rules.HookPointArg{
				{
					N:    1,
					Kind: "uint",
				},
				{
					N:    2,
					Kind: "null-terminated-string",
				},
			},
		},
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_open",
			Expression: `ondemand.name == "do_sys_openat2" && ondemand.arg2.str =~ ~"*/test-open" && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModuleWithOnDemandProbes(t, onDemands, nil, ruleDefs, withStaticOpts(testOpts{disableOnDemandRateLimiter: true}))
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

	test.WaitSignal(t, func() error {
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
	}, func(event *model.Event, r *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")

		value, _ := event.GetFieldValue("ondemand.arg2.str")
		assert.Equal(t, testFile, value.(string))
	})
}

func TestOnDemandChdir(t *testing.T) {
	SkipIfNotAvailable(t)

	onDemands := []rules.OnDemandHookPoint{
		{
			Name:      "chdir",
			IsSyscall: true,
			Args: []rules.HookPointArg{
				{
					N:    1,
					Kind: "null-terminated-string",
				},
			},
		},
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_chdir",
			Expression: `ondemand.name == "chdir" && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModuleWithOnDemandProbes(t, onDemands, nil, ruleDefs)
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

	test.WaitSignal(t, func() error {
		return os.Chdir(testFolder)
	}, func(event *model.Event, r *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")

		value, _ := event.GetFieldValue("ondemand.arg1.str")
		assert.Equal(t, testFolder, value.(string))
	})
}

func TestOnDemandMprotect(t *testing.T) {
	SkipIfNotAvailable(t)

	onDemands := []rules.OnDemandHookPoint{
		{
			Name: "security_file_mprotect",
			Args: []rules.HookPointArg{
				{
					N:    2,
					Kind: "uint",
				},
				{
					N:    3,
					Kind: "uint",
				},
			},
		},
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mprotect",
			Expression: `ondemand.name == "security_file_mprotect" && (ondemand.arg3.uint & (VM_READ|VM_WRITE)) == (VM_READ|VM_WRITE) && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModuleWithOnDemandProbes(t, onDemands, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		var data []byte
		data, err = unix.Mmap(0, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANON)
		if err != nil {
			return fmt.Errorf("couldn't memory segment: %w", err)
		}

		if err = unix.Mprotect(data, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
			return fmt.Errorf("couldn't mprotect segment: %w", err)
		}
		return nil
	}, func(event *model.Event, r *rules.Rule) {
		assert.Equal(t, "ondemand", event.GetType(), "wrong event type")
	})
}
