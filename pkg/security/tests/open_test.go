package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestOpen(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `open.filename == "/test" && open.flags & O_CREAT != 0`,
	}

	test, err := newSimpleTest(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.drive.Path("test")
	if err != nil {
		t.Fatal(err)
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT)
	if errno != 0 {
		t.Fatal(err)
	}
	defer syscall.Close(int(fd))
	defer os.Remove(testFile)

	event, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "open" {
			t.Errorf("expected open event, got %s", event.GetType())
		}

		if flags := event.Open.Flags; flags != syscall.O_CREAT {
			t.Errorf("expected open mode O_CREAT, got %d", flags)
		}
	}
}
