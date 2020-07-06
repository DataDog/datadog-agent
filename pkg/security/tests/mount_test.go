package tests

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestMount(t *testing.T) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `container.id == "{{.Root}}/test-mount"`,
	}

	test, err := newTestProbe(nil, []*policy.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mntPath, _, err := test.Path("test-mount")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(mntPath, 0755)

	dstMntPath, _, err := test.Path("test-dest-mount")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dstMntPath, 0755)

	// Test mount
	if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
		t.Fatalf("could not create bind mount: %s", err)
	}
	var mntId uint32

	event, err := test.GetEvent(3 * time.Second)
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "mount" {
			t.Errorf("expected mount event, got %s", event.GetType())
		}

		if p := event.Mount.ParentPathStr; p != dstMntPath {
			t.Errorf("expected %v for ParentPathStr, got %v", mntPath, p)
		}

		if fs := event.Mount.FSType; fs != "bind" {
			t.Errorf("expected a bind mount, got %v", fs)
		}
		mntId = event.Mount.NewMountID
	}

	// Test umount
	if err := syscall.Unmount(dstMntPath, syscall.MNT_DETACH); err != nil {
		t.Fatalf("could not unmount test-mount: %s", err)
	}

	event, err = test.GetEvent(3 * time.Second)
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "umount" {
			t.Errorf("expected umount event, got %s", event.GetType())
		}

		if uMntID := event.Umount.MountID; uMntID != mntId {
			t.Errorf("expected mount_id %v, got %v", mntId, uMntID)
		}
	}
}
