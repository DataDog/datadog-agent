// +build functionaltests

package tests

import (
	"os"
	"runtime"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestMkdir(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test_rule",
		Expression: `mkdir.filename == "{{.Root}}/test-mkdir" || mkdir.filename == "{{.Root}}/testat-mkdir" || (event.type == "mkdir" && event.retval == EPERM)`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-mkdir")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_MKDIR, uintptr(testFilePtr), uintptr(0707), 0); errno != 0 {
		t.Fatal(err)
	}
	defer syscall.Rmdir(testFile)

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "mkdir" {
			t.Errorf("expected mkdir event, got %s", event.GetType())
		}

		if mode := event.Mkdir.Mode; mode != 0707 {
			t.Errorf("expected mkdir mode 0707, got %#o", mode)
		}
	}

	testatFile, testatFilePtr, err := test.Path("testat-mkdir")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
		t.Fatal(error(errno))
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "mkdir" {
			t.Errorf("expected mkdir event, got %s", event.GetType())
		}

		if mode := event.Mkdir.Mode; mode != 0777 {
			t.Errorf("expected mkdir mode 0777, got %#o", mode)
		}
	}

	if err := syscall.Rmdir(testatFile); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(test.Root(), 0711); err != nil {
		t.Fatal(err)
	}

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 10000, 10000, 0); errno != 0 {
			t.Fatal(err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 10000, 10000, 0); errno != 0 {
			t.Fatal(err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno == 0 {
			t.Fatal(error(errno))
		}
	}()

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "mkdir" {
			t.Errorf("expected mkdir event, got %s", event.GetType())
		}

		if retval := event.Event.Retval; retval != -int64(syscall.EACCES) {
			t.Errorf("expected retval != EACCES, got %d", retval)
		}
	}
}
