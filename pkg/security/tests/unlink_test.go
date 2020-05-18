package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestUnlink(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `unlink.filename == "/test" || unlink.filename == "/testat"`,
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

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(testFile)

	if _, _, err := syscall.Syscall(syscall.SYS_UNLINK, uintptr(testFilePtr), 0, 0); err != 0 {
		t.Fatal(err)
	}

	event, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "unlink" {
		t.Errorf("expected unlink event, got %s", event.GetType())
	}

	testatFile, testatFilePtr, err := test.drive.Path("testat")
	if err != nil {
		t.Fatal(err)
	}

	f, err = os.Create(testatFile)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(testatFile)

	if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testatFilePtr), 0); err != 0 {
		t.Fatal(err)
	}

	event, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "unlink" {
		t.Errorf("expected unlink event, got %s", event.GetType())
	}
}
