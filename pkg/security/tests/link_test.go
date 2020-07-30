package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestLink(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.src_filename == "{{.Root}}/test-link" && link.new_filename == "{{.Root}}/test2-link"`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{rule}, testOpts{})
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
	}
}
