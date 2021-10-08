// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"unsafe"

	"github.com/avast/retry-go"
	"github.com/freddierice/go-losetup"
	"github.com/pkg/errors"
)

type testDrive struct {
	file       *os.File
	dev        *losetup.Device
	mountPoint string
}

func (td *testDrive) Root() string {
	return td.mountPoint
}

func (td *testDrive) Path(filename ...string) (string, unsafe.Pointer, error) {
	components := []string{td.mountPoint}
	components = append(components, filename...)
	path := path.Join(components...)
	filenamePtr, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", nil, err
	}
	return path, unsafe.Pointer(filenamePtr), nil
}

func newTestDrive(fsType string, mountOpts []string) (*testDrive, error) {
	return newTestDriveWithMountPoint(fsType, mountOpts, "")
}

func newTestDriveWithMountPoint(fsType string, mountOpts []string, mountPoint string) (*testDrive, error) {
	backingFile, err := ioutil.TempFile("", "secagent-testdrive-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create testdrive backing file")
	}

	if err := backingFile.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to close testdrive backing file")
	}

	if len(mountPoint) == 0 {
		mountPoint, err = ioutil.TempDir("", "secagent-testdrive-")
		if err != nil {
			os.Remove(backingFile.Name())
			return nil, errors.Wrap(err, "failed to create testdrive mount point")
		}
	}

	var loopback *losetup.Device
	var devPath string
	if fsType != "tmpfs" && fsType != "debugfs" && fsType != "tracefs" {
		if err := os.Truncate(backingFile.Name(), 20*1024*1024); err != nil {
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)

			return nil, errors.Wrap(err, "failed to truncate testdrive backing file")
		}

		dev, err := losetup.Attach(backingFile.Name(), 0, false)
		if err != nil {
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)
			return nil, errors.Wrap(err, "failed to create testdrive loop device")
		}

		if len(mountOpts) == 0 {
			mountOpts = append(mountOpts, "auto")
		}

		mkfsCmd := exec.Command("/sbin/mkfs."+fsType, dev.Path())
		if err := mkfsCmd.Run(); err != nil {
			_ = dev.Detach()
			os.Remove(backingFile.Name())
			os.RemoveAll(mountPoint)
			return nil, errors.Wrapf(err, "failed to create testdrive %s filesystem", fsType)
		}

		loopback = &dev
		devPath = dev.Path()

	} else {
		devPath = "none"
	}

	mountCmd := exec.Command("mount", "-t", fsType, "-o", strings.Join(mountOpts, ","), devPath, mountPoint)

	if err := mountCmd.Run(); err != nil {
		if loopback != nil {
			_ = loopback.Detach()
		}
		os.Remove(backingFile.Name())
		os.RemoveAll(mountPoint)
		return nil, errors.Wrap(err, "failed to mount testdrive")
	}

	return &testDrive{
		file:       backingFile,
		dev:        loopback,
		mountPoint: mountPoint,
	}, nil
}

func (td *testDrive) lsof() string {
	lsofCmd := exec.Command("lsof", td.mountPoint)
	output, _ := lsofCmd.CombinedOutput()
	return string(output)
}

func (td *testDrive) Unmount() error {
	unmountCmd := exec.Command("umount", "-f", td.mountPoint)
	return unmountCmd.Run()
}

func (td *testDrive) Close() {
	if err := td.Unmount(); err != nil {
		fmt.Printf("failed to unmount test drive: %s (lsof: %s)", err, td.lsof())
	}
	if td.dev != nil {
		if err := td.dev.Detach(); err != nil {
			fmt.Printf("failed to detach test drive: %s (lsof: %s)", err, td.lsof())
		}
		if err := retry.Do(td.dev.Remove); err != nil {
			fmt.Printf("failed to remove test drive: %s (lsof: %s)", err, td.lsof())
		}
	}
	os.Remove(td.file.Name())
	os.Remove(td.mountPoint)
}
