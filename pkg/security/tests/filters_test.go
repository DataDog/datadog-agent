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
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/avast/retry-go/v4"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
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
			return 0, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(unsafe.Pointer(testFilePtr)), uintptr(flags))
	if errno != 0 {
		return 0, error(errno)
	}

	return int(fd), nil
}

func TestFilterOpenBasenameApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/%s"`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withDynamicOpts(dynamicTestOpts{disableBundledRules: true}))
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
		return syscall.Close(fd1)
	}, testFile1); err != nil {
		t.Fatal(err)
	}

	testFile2, _, err = test.Path("test-oba-2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile2)

	// stats
	err = retry.Do(func() error {
		test.eventMonitor.SendStats()
		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":approver_type:basename"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":event_type:open"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		return nil
	}, retry.Delay(1*time.Second), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	if err := waitForOpenProbeEvent(test, func() error {
		fd2, err = openTestFile(test, testFile2, syscall.O_CREAT)
		if err != nil {
			return err
		}
		return syscall.Close(fd2)
	}, testFile2); err == nil {
		t.Fatal("shouldn't get an event")
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd2, err = openTestFile(test, testFile2, syscall.O_RDONLY)
		if err != nil {
			return err
		}
		return syscall.Close(fd2)
	}, testFile2); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestFilterOpenLeafDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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
		return syscall.Close(fd)
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		if event == nil || (eventType != "open") {
			return false
		}
		v, _ := event.GetFieldValue("open.file.path")
		return v == testFile
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
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

// This test is basically the same as TestFilterOpenLeafDiscarder but activity dumps are enabled.
// This means that the event is actually forwarded to user space, but the rule should not be evaluated
func TestFilterOpenLeafDiscarderActivityDump(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename =~ "/tmp/no-approver-*"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{enableActivityDump: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if err := test.StopAllActivityDumps(); err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}
	// dockerInstance, err := test.StartACustomDocker("ubuntu")
	dockerInstance, _, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	cmd := dockerInstance.Command("mkdir", []string{"/tmp/test"}, []string{})
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err)
	}

	testFile := "/tmp/test/test-obc-2"

	if err := test.GetEventDiscarder(t, func() error {
		cmd := dockerInstance.Command("touch", []string{testFile}, []string{})
		if _, err = cmd.CombinedOutput(); err != nil {
			t.Fatal(err)
		}
		return nil
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		e := event.(*model.Event)
		if e == nil || e.GetEventType() != model.FileOpenEventType {
			return false
		}
		v, _ := e.GetFieldValue("open.file.path")
		return v == testFile
	}); err != nil {
		t.Fatal(err)
	}

	// Check that we get a probe event "saved by activity dumps"
	if err := test.GetProbeEvent(func() error {
		cmd := dockerInstance.Command("cat", []string{testFile}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
			return err
		}
		return nil
	}, func(event *model.Event) bool {
		return event.GetType() == "open" && event.IsSavedByActivityDumps()
	}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
}

func testFilterOpenParentDiscarder(t *testing.T, parents ...string) {
	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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
		return syscall.Close(fd)
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		if event == nil || (eventType != "open") {
			return false
		}
		v, _ := event.GetFieldValue("open.file.path")
		return v == testFile
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
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestFilterOpenParentDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	testFilterOpenParentDiscarder(t, "parent")
}

func TestFilterOpenGrandParentDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	testFilterOpenParentDiscarder(t, "grandparent", "parent")
}

func runAUIDTest(t *testing.T, test *testModule, goSyscallTester string, eventType model.EventType, field eval.Field, path string, auidOK, auidKO string) {
	var cmdWrapper *dockerCmdWrapper
	cmdWrapper, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer cmdWrapper.stop()

	// reset stats
	test.statsdClient.Flush()

	if err := waitForProbeEvent(test, func() error {
		args := []string{
			"-login-uid-test",
			"-login-uid-event-type", eventType.String(),
			"-login-uid-path", "/tmp/test-auid",
			"-login-uid-value", auidOK,
		}

		cmd := cmdWrapper.Command(goSyscallTester, args, []string{})
		return cmd.Run()
	}, eventType, eventKeyValueFilter{
		key:   field,
		value: path,
	}); err != nil {
		t.Fatal(err)
	}

	// stats
	err = retry.Do(func() error {
		test.eventMonitor.SendStats()
		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":approver_type:auid"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":event_type:" + eventType.String()); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		return nil
	}, retry.Delay(1*time.Second), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	if err := waitForProbeEvent(test, func() error {
		args := []string{
			"-login-uid-test",
			"-login-uid-event-type", eventType.String(),
			"-login-uid-path", "/tmp/test-auid",
			"-login-uid-value", auidKO,
		}

		cmd := cmdWrapper.Command(goSyscallTester, args, []string{})
		return cmd.Run()
	}, eventType, eventKeyValueFilter{
		key:   field,
		value: path,
	}); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestFilterOpenAUIDEqualApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_equal_1",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid == 1005`,
		},
		{
			ID:         "test_equal_2",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid == 0`,
		},
		{
			ID:         "test_equal_3",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid == AUDIT_AUID_UNSET`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("equal-fixed-value", func(t *testing.T) {
		runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "1005", "6000")
	})

	t.Run("equal-zero", func(t *testing.T) {
		runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "0", "6000")
	})

	t.Run("equal-unset", func(t *testing.T) {
		runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "-1", "6000")
	})
}

func TestFilterOpenAUIDLesserApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_range_lesser",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid < 500`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "450", "605")
}

func TestFilterOpenAUIDGreaterApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_range_greater",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid > 1000`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "1500", "605")
}

func TestFilterOpenAUIDNotEqualUnsetApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_equal_4",
			Expression: `open.file.path =~ "/tmp/test-auid" && process.auid != AUDIT_AUID_UNSET`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	runAUIDTest(t, test, goSyscallTester, model.FileOpenEventType, "open.file.path", "/tmp/test-auid", "6000", "-1")
}

func TestFilterUnlinkAUIDEqualApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_equal_1",
			Expression: `unlink.file.path =~ "/tmp/test-auid" && process.auid == 1009`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("equal-fixed-value", func(t *testing.T) {
		runAUIDTest(t, test, goSyscallTester, model.FileUnlinkEventType, "unlink.file.path", "/tmp/test-auid", "1009", "6000")
	})
}

func TestFilterDiscarderMask(t *testing.T) {
	SkipIfNotAvailable(t)

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

	test, err := newTestModule(t, nil, ruleDefs)
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
			return err
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mask_open_rule")
		})

		utimbuf := &syscall.Utimbuf{
			Actime:  123,
			Modtime: 456,
		}

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(error(errno))
		}
		if err := waitForProbeEvent(test, nil, model.FileUtimesEventType, eventKeyValueFilter{
			key:   "utimes.file.path",
			value: testFile,
		}); err != nil {
			t.Fatal("should get a utimes event")
		}

		// wait a bit and ensure utimes event has been discarded
		time.Sleep(2 * time.Second)

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(error(errno))
		}
		if err := waitForProbeEvent(test, nil, model.FileUtimesEventType, eventKeyValueFilter{
			key:   "utimes.file.path",
			value: testFile,
		}); err == nil {
			t.Fatal("shouldn't get a utimes event")
		}

		// not check that we still have the open allowed
		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mask_open_rule")
		})
	}))
}

func TestFilterRenameFileDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/a*/test"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("b123/test")
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
		return syscall.Close(fd)
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		if event == nil || (eventType != "open") {
			return false
		}
		v, _ := event.GetFieldValue("open.file.path")
		return v == testFile
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
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}

	// rename the parent folder so that the discarder inode remains the same but now the rule should match
	newFile, _, err := test.Path("a123/test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(newFile)

	if err := os.MkdirAll(filepath.Dir(newFile), 0777); err != nil {
		t.Fatal(err)
	}

	// the next event on the file should now match the rule thus we should get an event, the inode should not anymore be discarded
	if err := os.Rename(testFile, newFile); err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, newFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, newFile); err != nil {
		t.Fatal("should get an event")
	}
}

func TestFilterRenameFolderDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/a*/test"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var fd int
	var testFile string

	testFile, _, err = test.Path("b123/test")
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
		return syscall.Close(fd)
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		if event == nil || (eventType != "open") {
			return false
		}
		v, _ := event.GetFieldValue("open.file.path")
		return v == testFile
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
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}

	// rename the parent folder so that the discarder inode remains the same but now the rule should match
	newFile, _, err := test.Path("a123/test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(newFile)

	// the next event on the file should now match the rule thus we should get an event, the inode should not anymore be discarded
	if err := os.Rename(filepath.Dir(testFile), filepath.Dir(newFile)); err != nil {
		t.Fatal(err)
	}

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, newFile, syscall.O_CREAT|syscall.O_SYNC)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, newFile); err != nil {
		t.Fatal("should get an event")
	}
}

func TestFilterOpenFlagsApprover(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.flags & (O_SYNC | O_NOCTTY) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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
		return syscall.Close(fd)
	}, testFile); err != nil {
		t.Fatal(err)
	}

	// stats
	err = retry.Do(func() error {
		test.eventMonitor.SendStats()
		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":approver_type:flag"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":event_type:open"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		return nil
	}, retry.Delay(1*time.Second), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_SYNC)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, testFile); err != nil {
		t.Fatal(err)
	}

	err = retry.Do(func() error {
		test.eventMonitor.SendStats()
		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":approver_type:flag"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		if count := test.statsdClient.Get(metrics.MetricEventApproved + ":event_type:open"); count == 0 {
			return fmt.Errorf("expected metrics not found: %+v", test.statsdClient.GetByPrefix(metrics.MetricEventApproved))
		}

		return nil
	}, retry.Delay(1*time.Second), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	if err := waitForOpenProbeEvent(test, func() error {
		fd, err = openTestFile(test, testFile, syscall.O_RDONLY)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}
}

func TestFilterDiscarderRetention(t *testing.T) {
	SkipIfNotAvailable(t)

	// We need to write a rule with no approver on the file path, and that won't match the real opened file (so that
	// a discarder is created).
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path =~ "{{.Root}}/no-approver-*" && open.flags & (O_CREAT | O_SYNC) > 0`,
	}

	testDrive, err := newTestDrive(t, "xfs", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
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
		return syscall.Close(fd)
	}, func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		e := event.(*model.Event)
		if e == nil || (e != nil && e.GetEventType() != model.FileOpenEventType) {
			return false
		}

		v, _ := e.GetFieldValue("open.file.path")
		return v == testFile
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
		return syscall.Close(fd)
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
			return syscall.Close(fd)
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

func TestFilterBpfCmd(t *testing.T) {
	SkipIfNotAvailable(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	rule := &rules.RuleDefinition{
		ID:         "test_bpf_map_create",
		Expression: fmt.Sprintf(`bpf.cmd == BPF_MAP_CREATE && process.file.name == "%s"`, path.Base(executable)),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var m *ebpf.Map
	defer func() {
		if m != nil {
			m.Close()
		}
	}()

	test.WaitSignal(t, func() error {
		m, err = ebpf.NewMap(&ebpf.MapSpec{Name: "test_bpf_map", Type: ebpf.Array, KeySize: 4, ValueSize: 4, MaxEntries: 1})
		if err != nil {
			return err
		}
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_bpf_map_create")
	})

	err = test.GetProbeEvent(func() error {
		if m.Update(uint32(0), uint32(1), ebpf.UpdateAny) != nil {
			return err
		}
		return nil
	}, func(event *model.Event) bool {
		cmdIntf, err := event.GetFieldValue("bpf.cmd")
		if !assert.NoError(t, err) {
			return false
		}
		cmdInt, ok := cmdIntf.(int)
		if !assert.True(t, ok) {
			return false
		}
		cmd := model.BPFCmd(uint64(cmdInt))
		if assert.Equal(t, model.BpfMapCreateCmd, cmd, "should not get a bpf event with cmd other than BPF_MAP_CREATE") {
			return false
		}
		return true
	}, 1*time.Second, model.BPFEventType)
	if err != nil {
		if otherErr, ok := err.(ErrTimeout); !ok {
			t.Fatal(otherErr)
		}
	}
}

func TestFilterRuntimeDiscarded(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_open",
			Expression: `open.file.path == "{{.Root}}/no-event"`,
		},
		{
			ID:         "test_unlink",
			Expression: `unlink.file.path == "{{.Root}}/no-event"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{discardRuntime: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("no-event")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// test that we don't receive event from the kernel
	if err := waitForOpenProbeEvent(test, func() error {
		fd, err := openTestFile(test, testFile, syscall.O_CREAT)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, testFile); err == nil {
		t.Fatal("shouldn't get an event")
	}

	// unlink aren't discarded kernel side (inode invalidation) but should be discarded before the rule evaluation
	err = test.GetSignal(t, func() error {
		return os.Remove(testFile)
	}, func(event *model.Event, r *rules.Rule) {
		t.Errorf("shouldn't get an event")
	})

	if err == nil {
		t.Errorf("shouldn't get an event")
	}
}
