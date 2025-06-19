// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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

func mountPointFromFd(fd int) uint32 {
	data, _ := os.ReadFile("/proc/" + strconv.Itoa(os.Getpid()) + "/fdinfo/" + strconv.Itoa(int(fd)))

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "mnt_id:") {
			mountId, _ := strconv.Atoi(strings.Split(line, "\t")[1])
			return uint32(mountId)
		}
	}
	return 0
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
		p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)

		fsfd, err := unix.Fsopen("tmpfs", 0)
		if err != nil {
			t.Skip("This kernel doesn't have the new mount api")
			return
		}
		defer unix.Close(fsfd)

		_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
		_ = fsconfigStr(fsfd, unix.FSCONFIG_SET_STRING, "size", "50M", 0)
		_ = fsconfig(fsfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)

		mountfd, err := unix.Fsmount(fsfd, 0, 0)
		if err != nil {
			assert.Fail(t, "fsmount failed")
			return
		}
		defer unix.Close(mountfd)

		time.Sleep(500 * time.Millisecond)

		if err != nil {
			assert.Fail(t, "mount resolution failed")
		}

		mnt, _, _, err := p.Resolvers.MountResolver.ResolveMount(mountPointFromFd(mountfd), 0, 0, "")
		assert.Equal(t, mnt.Origin, model.MountOriginFsmount)
		assert.Equal(t, mnt.RootStr, "/")
	})
}
