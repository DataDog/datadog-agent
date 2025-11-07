// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestListMount(t *testing.T) {
	SkipIfNotAvailable(t)

	dstMntBasename := "test-dest-mount"

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`chmod.file.path == "{{.Root}}/%s/test-mount"`, dstMntBasename),
	}, {
		ID:         "test_rule_pending",
		Expression: fmt.Sprintf(`chown.file.path == "{{.Root}}/%s/test-release"`, dstMntBasename),
	}}

	// Only meaningful if the kernel supports listmount/statmount
	if !mount.HasListMount() {
		t.Skip("listmount/statmount not supported on this kernel")
	}

	// Enable listmount snapshotter through config
	//t.Setenv("DD_EVENT_MONITORING_CONFIG_SNAPSHOT_USING_LISTMOUNT", "true")

	testDrive, err := newTestDrive(t, "xfs", []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mntPath := testDrive.Path("test-mount")
	os.MkdirAll(mntPath, 0755)
	defer os.RemoveAll(mntPath)

	dstMntPath := testDrive.Path(dstMntBasename)
	os.MkdirAll(dstMntPath, 0755)
	defer os.RemoveAll(dstMntPath)

	//var mntID uint32
	t.Run("mount", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
				return fmt.Errorf("could not create bind mount: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			//mntID = event.Mount.MountID
			if !assert.Equal(t, "mount", event.GetType(), "wrong event type") {
				return true
			}
			if !ebpfLessEnabled {
				assert.Equal(t, false, event.Mount.Detached, "Mount should not be detached")
				assert.Equal(t, true, event.Mount.Visible, "Mount should be visible")
				assert.Equal(t, model.MountOriginEvent, event.Mount.Origin, "Incorrect mount source")
				assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")
			}

			// filter by pid
			if event.ProcessContext.Pid != testSuitePid {
				return false
			}

			return assert.Equal(t, "/"+dstMntBasename, event.Mount.MountPointStr, "wrong mount point") &&
				assert.Equal(t, "xfs", event.Mount.GetFSType(), "wrong mount fs type")
		}, 3*time.Second, model.FileMountEventType)
		if err != nil {
			t.Fatal(err)
		}
	})
}
