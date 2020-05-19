package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestRename(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `rename.oldfilename == "/test" && rename.newfilename == "/test2"`,
	}

	test, err := newSimpleTest(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testOldFile, testOldFilePtr, err := test.drive.Path("test")
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

	testNewFile, testNewFilePtr, err := test.drive.Path("test2")
	if err != nil {
		t.Fatal(err)
	}

	_, _, errno := syscall.Syscall(syscall.SYS_RENAME, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
	if errno != 0 {
		t.Fatal(err)
	}

	event, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "rename" {
			t.Errorf("expected rename event, got %s", event.GetType())
		}
	}

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	_, _, errno = syscall.Syscall6(syscall.SYS_RENAMEAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
	if errno != 0 {
		t.Fatal(err)
	}

	event, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "rename" {
			t.Errorf("expected rename event, got %s", event.GetType())
		}
	}

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	_, _, errno = syscall.Syscall6(316 /* syscall.SYS_RENAMEAT2 */, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
	if errno != 0 {
		t.Fatal(err)
	}

	event, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "rename" {
			t.Errorf("expected rename event, got %s", event.GetType())
		}
	}
}
