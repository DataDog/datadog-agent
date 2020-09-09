// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	"time"
	"unsafe"

	"github.com/freddierice/go-losetup"
	"github.com/pkg/errors"
)

type testDrive struct {
	file       *os.File
	dev        losetup.Device
	mountPoint string
}

func (td *testDrive) Root() string {
	return td.mountPoint
}

func (td *testDrive) Path(filename string) (string, unsafe.Pointer, error) {
	filename = path.Join(td.mountPoint, filename)
	filenamePtr, err := syscall.BytePtrFromString(filename)
	if err != nil {
		return "", nil, err
	}
	return filename, unsafe.Pointer(filenamePtr), nil
}

func newTestDrive(fsType string, mountOpts []string) (*testDrive, error) {
	backingFile, err := ioutil.TempFile("", "secagent-testdrive-")
	if err != nil {
		return nil, err
	}

	mountPoint, err := ioutil.TempDir("", "secagent-testdrive-")
	if err != nil {
		return nil, err
	}

	if err := os.Truncate(backingFile.Name(), 1*1024*1024); err != nil {
		return nil, err
	}

	dev, err := losetup.Attach(backingFile.Name(), 0, false)
	if err != nil {
		os.Remove(backingFile.Name())
		return nil, err
	}

	if len(mountOpts) == 0 {
		mountOpts = append(mountOpts, "auto")
	}

	mkfsCmd := exec.Command("mkfs."+fsType, dev.Path())
	if err := mkfsCmd.Run(); err != nil {
		_ = dev.Detach()
		os.Remove(backingFile.Name())
		return nil, errors.Wrap(err, "failed to create ext4 filesystem")
	}

	mountCmd := exec.Command("mount", "-o", strings.Join(mountOpts, ","), dev.Path(), mountPoint)
	fmt.Printf("CMD %s\n", mountCmd.String())

	if err := mountCmd.Run(); err != nil {
		_ = dev.Detach()
		os.Remove(backingFile.Name())
		return nil, errors.Wrap(err, "failed to mount filesystem")
	}

	return &testDrive{
		file:       backingFile,
		dev:        dev,
		mountPoint: mountPoint,
	}, nil
}

func (td *testDrive) Unmount() error {
	unmountCmd := exec.Command("umount", "-f", td.mountPoint)
	if err := unmountCmd.Run(); err != nil {
		return errors.Wrap(err, "failed to unmount filesystem")
	}

	return nil
}

func (td *testDrive) Close() {
	os.RemoveAll(td.mountPoint)
	if err := td.Unmount(); err != nil {
		fmt.Print(err)
	}
	os.Remove(td.file.Name())
	os.Remove(td.mountPoint)
	time.Sleep(time.Second)
	if err := td.dev.Detach(); err != nil {
		fmt.Print(err)
	}
	if err := td.dev.Remove(); err != nil {
		fmt.Print(err)
	}
}
