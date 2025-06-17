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
	"path"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
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

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_fsmount_tmpfs",
			Expression: fmt.Sprintf(`fsmount.fd != 0 && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("fsmount-tmpfs", func(t *testing.T) {
		test.WaitSignal(t, func() error {

			fsfd, err := unix.Fsopen("tmpfs", 0)
			if err != nil {
				return fmt.Errorf("fsopen failed: %w", err)
			}
			defer unix.Close(fsfd)

			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
			_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "size", "50M", 0)
			_ = fsconfig(fsfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)

			mountfd, err := unix.Fsmount(fsfd, 0, 0)
			if err != nil {
				return fmt.Errorf("fsmount failed: %w", err)
			}
			defer unix.Close(mountfd)

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_fsmount_tmpfs")
			assertFieldEqual(t, event, "process.file.path", executable)
			assert.Equal(t, "fsmount", event.GetType(), "wrong event type")

			test.validateFsmountSchema(t, event)

		})
	})

}
