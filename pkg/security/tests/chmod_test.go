package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestChmod(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `chmod.filename == "{{.Root}}/test-chmod"`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-chmod")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)
	defer f.Close()

	if _, _, errno := syscall.Syscall(syscall.SYS_CHMOD, uintptr(testFilePtr), uintptr(0707), 0); errno != 0 {
		t.Fatal(err)
	}

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "chmod" {
			t.Errorf("expected chmod event, got %s", event.GetType())
		}

		if mode := event.Chmod.Mode; mode != 0707 {
			t.Errorf("expected chmod mode 0707, got %#o", mode)
		}
	}

	// fchmod syscall
	if _, _, errno := syscall.Syscall(syscall.SYS_FCHMOD, f.Fd(), uintptr(0717), 0); errno != 0 {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "chmod" {
			t.Errorf("expected chmod event, got %s", event.GetType())
		}

		if mode := event.Chmod.Mode; mode != 0717 {
			t.Errorf("expected chmod mode 0717, got %#o", mode)
		}
	}

	// fchmodat syscall
	if _, _, errno := syscall.Syscall6(syscall.SYS_FCHMODAT, 0, uintptr(testFilePtr), uintptr(0757), 0, 0, 0); errno != 0 {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "chmod" {
			t.Errorf("expected chmod event, got %s", event.GetType())
		}

		if mode := event.Chmod.Mode; mode != 0757 {
			t.Errorf("expected chmod mode 0757, got %#o", mode)
		}
	}
}
