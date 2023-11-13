// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/freddierice/go-losetup"
)

type testDrive struct {
	file  *os.File
	dev   *losetup.Device
	mount *testMount
}

func (td *testDrive) Root() string {
	return td.mount.target
}

func (td *testDrive) FSType() string {
	return td.mount.fstype
}

func (td *testDrive) Path(filename ...string) string {
	return td.mount.path(filename...)
}

func newTestDrive(tb testing.TB, fsType string, mountOpts []string, mountPoint string) (*testDrive, error) {
	return newTestDriveWithMountPoint(tb, fsType, mountOpts, mountPoint)
}

func newTestDriveWithMountPoint(tb testing.TB, fsType string, mountOpts []string, mountPoint string) (*testDrive, error) {
	backingFile, err := os.CreateTemp("", "secagent-testdrive-")
	if err != nil {
		return nil, fmt.Errorf("failed to create testdrive backing file: %w", err)
	}

	if err := backingFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close testdrive backing file: %w", err)
	}

	if mountPoint == "" {
		mountPoint = tb.TempDir()
	}

	var loopback *losetup.Device
	var devPath string
	if fsType != "tmpfs" && fsType != "debugfs" && fsType != "tracefs" {
		if err := os.Truncate(backingFile.Name(), 20*1024*1024); err != nil {
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)

			return nil, fmt.Errorf("failed to truncate testdrive backing file: %w", err)
		}

		dev, err := losetup.Attach(backingFile.Name(), 0, false)
		if err != nil {
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)
			return nil, fmt.Errorf("failed to create testdrive loop device: %w", err)
		}

		// if len(mountOpts) == 0 {
		// 	mountOpts = append(mountOpts, "auto")
		// }

		var env []string
		if fsType == "xfs" {
			// starting with v5.19, mkfs.xfs does not accept filesystem of size < 300MB
			// for more info see
			// https://lore.kernel.org/all/164738662491.3191861.15611882856331908607.stgit@magnolia/
			// https://patchwork.ozlabs.org/project/ltp/patch/20220817204015.31420-1-pvorel@suse.cz/#2950033
			// so we re-use the undocumented escape path used in fstests
			env = os.Environ()
			env = append(env, "TEST_DIR=1", "TEST_DEV=1", "QA_CHECK_FS=1")
		}

		mkfsCmd := exec.Command("/sbin/mkfs."+fsType, dev.Path())
		mkfsCmd.Env = env

		if out, err := mkfsCmd.CombinedOutput(); err != nil {
			tb.Error(string(out))
			_ = dev.Detach()
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)
			return nil, fmt.Errorf("failed to create testdrive %s filesystem: %w", fsType, err)
		}

		loopback = &dev
		devPath = dev.Path()

	} else {
		devPath = "none"
	}

	mount := newTestMount(
		mountPoint,
		withSource(devPath),
		withFSType(fsType),
		withMountOpts(mountOpts...),
	)

	if err := mount.mount(); err != nil {
		if loopback != nil {
			_ = loopback.Detach()
		}
		os.Remove(backingFile.Name())
		os.RemoveAll(mountPoint)
		return nil, fmt.Errorf("failed to mount testdrive: %w", err)
	}

	return &testDrive{
		file:  backingFile,
		dev:   loopback,
		mount: mount,
	}, nil
}

func (td *testDrive) lsof() string {
	lsofCmd := exec.Command("lsof", td.Root())
	output, _ := lsofCmd.CombinedOutput()
	return string(output)
}

func (td *testDrive) Unmount() error {
	return td.mount.unmount(syscall.MNT_FORCE)
}

func (td *testDrive) DetachDevice() error {
	if td.dev != nil {
		if err := td.dev.Detach(); err != nil {
			return err
		}
		if err := retry.Do(td.dev.Remove); err != nil {
			return err
		}
	}

	if err := os.Remove(td.file.Name()); err != nil {
		return err
	}

	if err := os.RemoveAll(td.Root()); err != nil {
		return err
	}
	return nil
}

func (td *testDrive) Close() {
	if err := td.Unmount(); err != nil {
		fmt.Printf("failed to unmount test drive: %s (lsof: %s)\n", err, td.lsof())
	}

	if err := td.DetachDevice(); err != nil {
		fmt.Printf("failed to detach test drive: %s (lsof: %s)\n", err, td.lsof())
	}
}
