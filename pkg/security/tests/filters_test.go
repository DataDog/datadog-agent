package tests

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/pkg/errors"
)

func openTestFile(test *testProbe, filename string) (int, string, error) {
	testFile, testFilePtr, err := test.Path(filename)
	if err != nil {
		return 0, "", err
	}

	if dir := filepath.Dir(testFile); dir != test.Root() {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return 0, "", errors.Wrap(err, "failed to create directory")
		}
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT)
	if errno != 0 {
		return 0, "", error(errno)
	}

	return int(fd), testFile, nil
}

func waitForOpenEvent(test *testProbe, filename string) (*probe.Event, error) {
	for {
		select {
		case event := <-test.events:
			if value, _ := event.GetFieldValue("open.filename"); value == filename {
				return event, nil
			}
		case <-time.After(3 * time.Second):
			return nil, errors.New("timeout")
		}
	}
}

func TestOpenSimpleFilter(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `open.filename == "{{.Root}}/test-1"`,
	}

	test, err := newTestProbe(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fd1, testFile1, err := openTestFile(test, "test-1")
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd1)
	defer os.Remove(testFile1)

	if _, err := waitForOpenEvent(test, testFile1); err != nil {
		t.Fatal(err)
	}

	fd2, testFile2, err := openTestFile(test, "test-2")
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd2)
	defer os.Remove(testFile2)

	if event, err := waitForOpenEvent(test, testFile2); err == nil {
		t.Fatalf("shouldn't get an event: %+v", event)
	}
}
