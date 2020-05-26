package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestRmdir(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `rmdir.filename == "{{.Root}}/test"`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test")
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mkdir(testFile, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if _, _, err := syscall.Syscall(syscall.SYS_RMDIR, uintptr(testFilePtr), 0, 0); err != 0 {
		t.Fatal(error(err))
	}

	event, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "rmdir" {
			t.Errorf("expected rmdir event, got %s", event.GetType())
		}
	}
}
