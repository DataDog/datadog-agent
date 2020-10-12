// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/pkg/errors"
)

func openTestFile(test *testProbe, filename string, flags int) (int, string, error) {
	testFile, testFilePtr, err := test.Path(filename)
	if err != nil {
		return 0, "", err
	}

	if dir := filepath.Dir(testFile); dir != test.Root() {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return 0, "", errors.Wrap(err, "failed to create directory")
		}
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), uintptr(flags))
	if errno != 0 {
		return 0, "", error(errno)
	}

	return int(fd), testFile, nil
}

func waitForOpenEvent(test *testProbe, filename string) (*probe.Event, error) {
	timeout := time.After(3 * time.Second)
	exhaust := time.After(time.Second)

	var event *probe.Event
	for {
		select {
		case e := <-test.events:
			if value, _ := e.GetFieldValue("open.filename"); value == filename {
				event = e
			}
		case <-test.discarders:
		case <-exhaust:
			if event != nil {
				return event, nil
			}
		case <-timeout:
			return nil, errors.New("timeout")
		}
	}
}

func waitForOpenDiscarder(test *testProbe, filename string) (*probe.Event, error) {
	timeout := time.After(5 * time.Second)
	exhaust := time.After(time.Second)

	var event *probe.Event
	for {
		select {
		case <-test.events:
		case discarder := <-test.discarders:
			test.probe.OnNewDiscarder(test.rs, discarder.event.(*sprobe.Event), discarder.field)
			if value, _ := discarder.event.GetFieldValue("open.filename"); value == filename {
				event = discarder.event.(*sprobe.Event)
			}
		case <-exhaust:
			if event != nil {
				return event, nil
			}
		case <-timeout:
			return nil, errors.New("timeout")
		}
	}
}

func TestOpenBasenameApproverFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/test-oba-1"`,
	}

	test, err := newTestProbe(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fd1, testFile1, err := openTestFile(test, "test-oba-1", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd1)
	defer os.Remove(testFile1)

	if _, err := waitForOpenEvent(test, testFile1); err != nil {
		t.Fatal(err)
	}

	fd2, testFile2, err := openTestFile(test, "test-oba-2", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd2)
	defer os.Remove(testFile2)

	if event, err := waitForOpenEvent(test, testFile2); err == nil {
		t.Fatalf("shouldn't get an event: %+v", event)
	}
}

func TestOpenParentDiscarderFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "/etc/passwd"`,
	}

	test, err := newTestProbe(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true, disableApprovers: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fd1, testFile1, err := openTestFile(test, "test-obd-2", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd1)
	defer os.Remove(testFile1)

	if _, err := waitForOpenDiscarder(test, testFile1); err != nil {
		t.Fatal(err)
	}

	fd2, testFile2, err := openTestFile(test, "test-obd-2", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd2)
	defer os.Remove(testFile2)

	if event, err := waitForOpenEvent(test, testFile2); err == nil {
		t.Fatalf("shouldn't get an event: %+v", event)
	}
}

func TestOpenFlagsApproverFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.flags & (O_CREAT | O_TRUNC) > 0`,
	}

	test, err := newTestProbe(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fd1, testFile1, err := openTestFile(test, "test-ofa-1", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd1)
	defer os.Remove(testFile1)

	if _, err := waitForOpenEvent(test, testFile1); err != nil {
		t.Fatal(err)
	}

	fd2, testFile2, err := openTestFile(test, "test-ofa-1", syscall.O_TRUNC)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd2)

	if _, err := waitForOpenEvent(test, testFile2); err != nil {
		t.Fatal(err)
	}

	fd3, testFile3, err := openTestFile(test, "test-ofa-1", syscall.O_RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd3)

	if event, err := waitForOpenEvent(test, testFile3); err == nil {
		t.Fatalf("shouldn't get an event: %+v", event)
	}
}

func TestOpenProcessInodeApproverFilter(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename =~ "{{.Root}}/test-oba-1" && process.filename == "/bin/cat"`,
	}

	test, err := newTestProbe(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fd1, testFile1, err := openTestFile(test, "test-oba-1", syscall.O_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd1)
	defer os.Remove(testFile1)

	if event, err := waitForOpenEvent(test, testFile1); err == nil {
		t.Fatalf("shouldn't get an event: %+v", event)
	}
}
