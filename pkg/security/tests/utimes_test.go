package tests

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestUtime(t *testing.T) {
	ruleDef := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `utimes.filename == "{{.Root}}/test-utime"`,
	}

	test, err := newTestModule(nil, []*policy.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-utime")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(testFile)

	utimbuf := &syscall.Utimbuf{
		Actime:  123,
		Modtime: 456,
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_UTIME, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
		t.Fatal(err)
	}

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "utimes" {
			t.Errorf("expected utimes event, got %s", event.GetType())
		}

		if atime := event.Utimes.Atime.Unix(); atime != 123 {
			t.Errorf("expected access time of 123, got %d", atime)
		}

		if mtime := event.Utimes.Mtime.Unix(); mtime != 456 {
			t.Errorf("expected modification time of 456, got %d", mtime)
		}
	}

	var times = [2]syscall.Timeval{
		{
			Sec:  111,
			Usec: 222,
		},
		{
			Sec:  333,
			Usec: 444,
		},
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_UTIMES, uintptr(testFilePtr), uintptr(unsafe.Pointer(&times[0])), 0); errno != 0 {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "utimes" {
			t.Errorf("expected utimes event, got %s", event.GetType())
		}

		if atime := event.Utimes.Atime.Unix(); atime != 111 {
			t.Errorf("expected access time of 111, got %d", atime)
		}

		if atime := event.Utimes.Atime.UnixNano(); atime%int64(time.Second)/int64(time.Microsecond) != 222 {
			t.Errorf("expected access microseconds of 222, got %d", atime%int64(time.Second)/int64(time.Microsecond))
		}
	}
}
