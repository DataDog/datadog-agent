package tests

import (
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestMkdir(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `mkdir.filename == "/test" || mkdir.filename == "/testat"`,
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

	if _, _, errno := syscall.Syscall(syscall.SYS_MKDIR, uintptr(testFilePtr), uintptr(0707), 0); errno != 0 {
		t.Fatal(err)
	}
	defer syscall.Rmdir(testFile)

	event, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}

	if mode := event.Mkdir.Mode; mode != 0707 {
		t.Errorf("expected mkdir mode 0707, got %#o", mode)
	}

	testatFile, testatFilePtr, err := test.drive.Path("testat")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
		t.Fatal(error(errno))
	}
	defer syscall.Rmdir(testatFile)

	event, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}

	if mode := event.Mkdir.Mode; mode != 0777 {
		t.Errorf("expected mkdir mode 0777, got %#o", mode)
	}
}
