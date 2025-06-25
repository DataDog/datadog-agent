// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func fsconfig(fd int, cmd uint, key *byte, value *byte, aux int) (err error) {
	_, _, e1 := unix.Syscall6(unix.SYS_FSCONFIG, uintptr(fd), uintptr(cmd), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(value)), uintptr(aux), 0)
	return e1
}

func fsconfigStr(fd int, cmd uint, key string, value string, aux int) (err error) {
	keyBytes := append([]byte(key), 0)
	valueBytes := append([]byte(value), 0)

	err = fsconfig(fd, cmd, &keyBytes[0], &valueBytes[0], aux)
	return err
}

func TestFsmount(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `open.file.name == "test-open"`,
		},
		{
			ID:         "test_rule_2",
			Expression: `mkdir.file.name == "test-mkdir"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("fsmount-tmpfs", func(t *testing.T) {
		var fsfd int
		err = test.GetProbeEvent(func() error {
			fsfd, err := unix.Fsopen("tmpfs", 0)
			if err != nil {
				t.Skip("This kernel doesn't have the new mount api")
				return nil
			}
			defer unix.Close(fsfd)

			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "size", "50M", 0)
			_ = fsconfig(fsfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)

			mountfd, err := unix.Fsmount(fsfd, unix.FSMOUNT_CLOEXEC, unix.MOUNT_ATTR_RDONLY)
			if err != nil {
				return err
			}
			defer unix.Close(mountfd)

			return nil
		}, func(event *model.Event) bool {
			assert.NotEqual(t, event.Fsmount.MountID, uint32(0), "Mount id is zero")
			assert.NotEqual(t, event.Fsmount.Flags, unix.FSOPEN_CLOEXEC, "Mount flags")
			assert.NotEqual(t, event.Fsmount.Fd, fsfd, "Wrong file descriptor")
			assert.NotEqual(t, event.Fsmount.MountAttrs, unix.FSMOUNT_CLOEXEC, "Wrong mount attributes")
			return true
		}, 3*time.Second, model.FileFsmountEventType)
	})

	t.Run("fsmount-resolve-open-file", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fsfd, err := unix.Fsopen("tmpfs", 0)
			if err != nil {
				t.Skip("This kernel doesn't have the new mount api")
				return nil
			}
			defer unix.Close(fsfd)

			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "size", "50M", 0)
			_ = fsconfig(fsfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)

			mountfd, err := unix.Fsmount(fsfd, unix.FSMOUNT_CLOEXEC, 0)
			if err != nil {
				return err
			}

			file := fmt.Sprintf("/proc/%d/fd/%d/test-open", os.Getpid(), mountfd)
			cmd := exec.Command("touch", file)
			err = cmd.Run()

			if err != nil {
				return err
			}
			defer unix.Close(mountfd)

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "/test-open", event.Open.File.PathnameStr, "Wrong pathname")
		})
	})

	t.Run("fsmount-resolve-mkdir", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fsfd, err := unix.Fsopen("tmpfs", 0)
			if err != nil {
				t.Skip("This kernel doesn't have the new mount api")
				return nil
			}
			defer unix.Close(fsfd)

			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "size", "50M", 0)
			_ = fsconfig(fsfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)

			mountfd, err := unix.Fsmount(fsfd, unix.FSMOUNT_CLOEXEC, 0)
			if err != nil {
				return err
			}

			file := fmt.Sprintf("/proc/%d/fd/%d/test-mkdir", os.Getpid(), mountfd)
			cmd := exec.Command("mkdir", file)
			err = cmd.Run()

			if err != nil {
				return err
			}
			defer unix.Close(mountfd)

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "/test-mkdir", event.Mkdir.File.PathnameStr, "Wrong path")
		})
	})

}
