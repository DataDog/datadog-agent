// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestUnlink(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.filename == "{{.Root}}/test-unlink" || unlink.filename == "{{.Root}}/testat-unlink" || unlink.filename == "{{.Root}}/testat-rmdir"`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-unlink")
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

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "unlink" {
			t.Errorf("expected unlink event, got %s", event.GetType())
		}
	}

	testatFile, testatFilePtr, err := test.Path("testat-unlink")
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

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "unlink" {
			t.Errorf("expected unlink event, got %s", event.GetType())
		}
	}

	testDir, testDirPtr, err := test.Path("testat-rmdir")
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mkdir(testDir, 0777); err != nil {
		t.Fatal(err)
	}

	if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testDirPtr), 512); err != 0 {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "unlink" {
			t.Errorf("expected unlink event, got %s", event.GetType())
		}
	}
}
