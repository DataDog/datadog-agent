package tests

import (
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestRmdir(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `rmdir.filename == "/test"`,
	}

	test, err := newSimpleTest(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if err := syscall.Mkdir("/tmp/test", 0777); err != nil {
		t.Fatal(err)
	}

	filenamePtr, err := syscall.BytePtrFromString("/tmp/test")
	if _, _, err := syscall.Syscall(syscall.SYS_RMDIR, uintptr(unsafe.Pointer(filenamePtr)), 0, 0); err != 0 {
		t.Fatal(error(err))
	}

	event, err := test.client.GetEvent(3 * time.Second)
	if err != nil {
		t.Error(err)
	}

	if event.GetType() != "rmdir" {
		t.Errorf("expected rmdir event, got %s", event.GetType())
	}
}
