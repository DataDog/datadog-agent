package tests

import (
	"syscall"
	"testing"
	"time"
	"unsafe"

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

	if err := syscall.Mkdir("/tmp/test", 0777); err != nil {
		t.Fatal(err)
	}
	defer syscall.Rmdir("/tmp/test")

	event, err := test.client.GetEvent(3 * time.Second)
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}

	filenamePtr, err := syscall.BytePtrFromString("/tmp/testat")
	if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(unsafe.Pointer(filenamePtr)), uintptr(0777)); errno != 0 {
		t.Fatal(error(errno))
	}
	defer syscall.Rmdir("/tmp/testat")

	event, err = test.client.GetEvent(3 * time.Second)
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}
}
