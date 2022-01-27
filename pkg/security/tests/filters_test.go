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

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
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
			return err
		}
		if err = syscall.Close(fd1); err != nil {
			return err
		}
		return nil
	}, testFile1); err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile2)

	testFile2, _, err = test.Path("test-oba-2")
	if err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd2, err = openTestFile(test, testFile2, syscall.O_CREAT)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd2); err != nil {
			return err
		}
		return nil
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

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
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
			return err
		}
		if err = syscall.Close(fd1); err != nil {
			return err
		}
		return nil
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
			return err
		}
		if err = syscall.Close(fd2); err != nil {
			return err
		}
		return nil
	}, testFile2); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestOpenLeafDiscarderFilter(t *testing.T) {
	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-obc-2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := test.GetEventDiscarder(t, func() error {
		// The policy file inode is likely to be reused by the kernel after deletion. On deletion, the inode discarder will
		// be marked as retained in kernel space and will therefore no longer discard events. By waiting for the discard
		// retention period to expire, we're making sure that a newly created discarder will properly take effect.
		time.Sleep(probe.DiscardRetention)

		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, func(d *testDiscarder) bool {
		e := d.event.(*probe.Event)
		if e == nil || (e != nil && e.GetEventType() != model.FileOpenEventType) {
			return false
		}
		v, _ := e.GetFieldValue("open.file.path")
		if v == testFile {
			return true
		}
		return false
	}); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("event inode: %d, parent inode: %d, error: %v", inode, parentInode, err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func testOpenParentDiscarderFilter(t *testing.T, parents ...string) {
	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path(append(parents, "test-obd-2")...)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := test.GetEventDiscarder(t, func() error {
		// The policy file inode is likely to be reused by the kernel after deletion. On deletion, the inode discarder will
		// be marked as retained in kernel space and will therefore no longer discard events. By waiting for the discard
		// retention period to expire, we're making sure that a newly created discarder will properly take effect.
		time.Sleep(probe.DiscardRetention)

		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, func(d *testDiscarder) bool {
		e := d.event.(*probe.Event)
		if e == nil || (e != nil && e.GetEventType() != model.FileOpenEventType) {
			return false
		}
		v, _ := e.GetFieldValue("open.file.path")
		if v == testFile {
			return true
		}
		return false
	}); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("event inode: %d, parent inode: %d, error: %v", inode, parentInode, err)
	}

	testFile, _, err = test.Path(append(parents, "test-obd-3")...)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestOpenParentDiscarderFilter(t *testing.T) {
	testOpenParentDiscarderFilter(t, "parent")
}

func TestOpenGrandParentDiscarderFilter(t *testing.T) {
	testOpenParentDiscarderFilter(t, "grandparent", "parent")
}

func TestDiscarderFilterMask(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_mask_open_rule",
			Expression: `open.file.path == "{{.Root}}/test-mask"`,
		},
		{
			ID:         "test_mask_utimes_rule",
			Expression: `utimes.file.path == "{{.Root}}/do_not_match/test-mask"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mask", ifSyscallSupported("SYS_UTIME", func(t *testing.T, syscallNB uintptr) {
		var testFile string
		var testFilePtr unsafe.Pointer

		defer os.Remove(testFile)

		// not check that we still have the open allowed
		test.WaitSignal(t, func() error {
			// The policy file inode is likely to be reused by the kernel after deletion. On deletion, the inode discarder will
			// be marked as retained in kernel space and will therefore no longer discard events. By waiting for the discard
			// retention period to expire, we're making sure that a newly created discarder will properly take effect.
			time.Sleep(probe.DiscardRetention)

			testFile, testFilePtr, err = test.CreateWithOptions("test-mask", 98, 99, 0o447)
			if err != nil {
				return err
			}
			return nil
		}, func(event *probe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mask_open_rule")
		})

		utimbuf := &syscall.Utimbuf{
			Actime:  123,
			Modtime: 456,
		}

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(error(errno))
		}
		if err := waitForProbeEvent(test, nil, "utimes.file.path", testFile, model.FileUtimesEventType); err != nil {
			t.Fatal("should get a utimes event")
		}

		// wait a bit and ensure utimes event has been discarded
		time.Sleep(2 * time.Second)

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(error(errno))
		}
		if err := waitForProbeEvent(test, nil, "utimes.file.path", testFile, model.FileUtimesEventType); err == nil {
			t.Fatal("shouldn't get a utimes event")
		}

		// not check that we still have the open allowed
		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0)
			if err != nil {
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			return nil
		}, func(event *probe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mask_open_rule")
		})
	}))
}

func TestOpenFlagsApproverFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.flags & (O_SYNC | O_NOCTTY) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
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
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_RDONLY)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestOpenProcessPidDiscarder(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-oba-1" && process.file.path == "/bin/cat"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("test-oba-1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := test.GetEventDiscarder(t, func() error {
		// The policy file inode is likely to be reused by the kernel after deletion. On deletion, the inode discarder will
		// be marked as retained in kernel space and will therefore no longer discard events. By waiting for the discard
		// retention period to expire, we're making sure that a newly created discarder will properly take effect.
		time.Sleep(probe.DiscardRetention)

		fd, err = openTestFile(test, testFile, syscall.O_CREAT)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, func(d *testDiscarder) bool {
		e := d.event.(*probe.Event)
		if e == nil || (e != nil && e.GetEventType() != model.FileOpenEventType) {
			return false
		}
		v, _ := e.GetFieldValue("open.file.path")
		if v == testFile {
			return true
		}
		return false
	}); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("event inode: %d, parent inode: %d, error: %v", inode, parentInode, err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_TRUNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err == nil {
		t.Fatalf("shouldn't get an event")
	}
}

func TestDiscarderRetentionFilter(t *testing.T) {
	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	testDrive, err := newTestDrive("xfs", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{testDir: testDrive.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("to_be_discarded/test-obc-2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := test.GetEventDiscarder(t, func() error {
		// The policy file inode is likely to be reused by the kernel after deletion. On deletion, the inode discarder will
		// be marked as retained in kernel space and will therefore no longer discard events. By waiting for the discard
		// retention period to expire, we're making sure that a newly created discarder will properly take effect.
		time.Sleep(probe.DiscardRetention)

		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil

	}, func(d *testDiscarder) bool {
		e := d.event.(*probe.Event)
		if e == nil || (e != nil && e.GetEventType() != model.FileOpenEventType) {
			return false
		}

		v, _ := e.GetFieldValue("open.file.path")
		if v == testFile {
			return true
		}

		return false

	}); err != nil {
		inode := getInode(t, testFile)
		parentInode := getInode(t, path.Dir(testFile))

		t.Fatalf("event inode: %d, parent inode: %d, error: %v", inode, parentInode, err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		if err = syscall.Close(fd); err != nil {
			return err
		}
		return nil
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
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
				return err
			}
			if err = syscall.Close(fd); err != nil {
				return err
			}
			return nil
		}, newFile); err != nil {
			discarded = true
			break
		}
	}

	if !discarded {
		t.Fatal("should be discarded")
	}

	if diff := time.Since(start); uint64(diff) < uint64(probe.DiscardRetention)-uint64(time.Second) {
		t.Fatalf("discarder retention (%s) not reached: %s", time.Duration(uint64(probe.DiscardRetention)-uint64(time.Second)), diff)
	}
}
