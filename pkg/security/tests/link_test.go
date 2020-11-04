// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestLink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.source.filename == "{{.Root}}/test-link" && link.target.filename == "{{.Root}}/test2-link"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testOldFile, testOldFilePtr, err := test.Path("test-link")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testOldFile)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	testNewFile, testNewFilePtr, err := test.Path("test2-link")
	if err != nil {
		t.Fatal(err)
	}

	_, _, errno := syscall.Syscall(syscall.SYS_LINK, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
	if errno != 0 {
		t.Fatal(err)
	}

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "link" {
			t.Errorf("expected link event, got %s", event.GetType())
		}
	}

	if err := os.Remove(testNewFile); err != nil {
		t.Fatal(err)
	}

	_, _, errno = syscall.Syscall6(syscall.SYS_LINKAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
	if errno != 0 {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "link" {
			t.Errorf("expected rename event, got %s", event.GetType())
		}

		if inode := getInode(t, testNewFile); inode != event.Link.Source.Inode {
			t.Errorf("expected inode %d, got %d", event.Link.Source.Inode, inode)
		}

		testContainerPath(t, event, "link.source.container_path")
		testContainerPath(t, event, "link.target.container_path")
	}
}
