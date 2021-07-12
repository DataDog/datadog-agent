// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func openTestFile(test *testModule, testFile string, flags int) (int, error) {
	testFilePtr, err := syscall.BytePtrFromString(testFile)
	if err != nil {
		return 0, err
	}

	if dir := filepath.Dir(testFile); dir != test.Root() {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return 0, errors.Wrap(err, "failed to create directory")
		}
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(unsafe.Pointer(testFilePtr)), uintptr(flags))
	if errno != 0 {
		return 0, error(errno)
	}

	return int(fd), nil
}

func TestOpenBasenameApproverFilterERPCDentryResolution(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/%s"`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true, disableMapDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd1, fd2 int
	var testFile1, testFile2 string

	testFile1, _, err = test.Path(basename)
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile1)

	if err := waitForOpenProbeEvent(test, func() error {
		fd1, err = openTestFile(test, testFile1, syscall.O_CREAT)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd1)
	}, testFile1); err != nil {
		t.Error(err)
	}

	defer os.Remove(testFile2)

	testFile2, _, err = test.Path("test-oba-2")
	if err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd2, err = openTestFile(test, testFile2, syscall.O_CREAT)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd2)
	}, testFile2); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestOpenBasenameApproverFilterMapDentryResolution(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/%s"`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true, disableERPCDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd1, fd2 int
	var testFile1, testFile2 string

	testFile1, _, err = test.Path(basename)
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile1)

	if err := waitForOpenProbeEvent(test, func() error {
		fd1, err = openTestFile(test, testFile1, syscall.O_CREAT)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd1)
	}, testFile1); err != nil {
		t.Fatal(err)
	}

	testFile2, _, err = test.Path("test-oba-2")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile2)

	if err := waitForOpenProbeEvent(test, func() error {
		fd2, err = openTestFile(test, testFile2, syscall.O_CREAT)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd2)
	}, testFile2); err == nil {
		t.Fatalf("shouldn't get an event")
	}
}

func TestOpenLeafDiscarderFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename =~ "{{.Root}}/test-obc-1" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// ensure that all the previous discarder are removed
	test.probe.FlushDiscarders()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-obc-2")
	if err != nil {
		t.Fatal(err)
	}

	fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	defer os.Remove(testFile)

	if err := waitForOpenDiscarder(test, testFile); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("not able to get the expected event inode: %d, parent inode: %d", inode, parentInode)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatalf("shouldn't get an event")
	}
}

func TestOpenParentDiscarderFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "/usr/local/test-obd-2" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// ensure that all the previous discarder are removed
	test.probe.FlushDiscarders()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-obd-2")
	if err != nil {
		t.Fatal(err)
	}

	fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	defer os.Remove(testFile)

	if err := waitForOpenDiscarder(test, testFile); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("not able to get the expected event inode: %d, parent inode: %d", inode, parentInode)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatalf("shouldn't get an event")
	}
}

func TestOpenFlagsApproverFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.flags & (O_SYNC | O_NOCTTY) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-ofa-1")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile)

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_NOCTTY)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err != nil {
		t.Error(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_SYNC)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err != nil {
		t.Error(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_RDONLY)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Error("shouldn't get an event")
	}
}

func TestOpenProcessPidDiscarder(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =="{{.Root}}/test-oba-1" && process.file.path == "/bin/cat"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// ensure that all the previous discarder are removed
	test.probe.FlushDiscarders()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-oba-1")
	if err != nil {
		t.Fatal(err)
	}

	fd, err = openTestFile(test, testFile, syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	defer os.Remove(testFile)

	if err := waitForOpenDiscarder(test, testFile); err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile)

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_TRUNC)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatalf("shouldn't get an event")
	}
}

func TestDiscarderRetentionFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/test-obc-1" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	testDrive, err := newTestDrive("xfs", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{testDir: testDrive.Root(), wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// ensure that all the previous discarder are removed
	test.probe.FlushDiscarders()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-obc-2")
	if err != nil {
		t.Fatal(err)
	}

	fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	defer os.Remove(testFile)

	if err := waitForOpenDiscarder(test, testFile); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("not able to get the expected event inode: %d, parent inode: %d", inode, parentInode)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			t.Fatal(err)
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatalf("shouldn't get an event")
	}

	// check the retention, we should have event during the retention period
	var discarded bool

	start := time.Now()
	end := start.Add(20 * time.Second)

	newFile, _, err := test.Path("test-obc-renamed")
	if err != nil {
		t.Fatal(err)
	}

	// rename to generate an invalidation of the discarder in kernel marking it as retained
	if err := os.Rename(testFile, newFile); err != nil {
		t.Fatal(err)
	}

	for time.Now().Before(end) {
		if err := waitForOpenProbeEvent(test, func() error {
			fd, err = openTestFile(test, newFile, syscall.O_CREAT|syscall.O_SYNC)
			if err != nil {
				t.Fatal(err)
			}
			return syscall.Close(fd)
		}, newFile); err != nil {
			discarded = true
			break
		}
	}

	if !discarded {
		t.Fatalf("should be discarded")
	}

	if diff := time.Now().Sub(start); uint64(diff) < uint64(probe.DiscardRetention)-uint64(time.Second) {
		t.Errorf("discarder retention (%s) not reached: %s", time.Duration(uint64(probe.DiscardRetention)-uint64(time.Second)), diff)
	}
}
