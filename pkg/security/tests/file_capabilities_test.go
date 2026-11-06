// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

import (
	"encoding/binary"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// setTestFileCapabilities writes the version-2 security.capability xattr. The
// collector observes the kernel's decoded result, not this on-disk representation.
func setTestFileCapabilities(t *testing.T, filename string, caps model.FileCapabilities) {
	t.Helper()
	data := make([]byte, 20)
	magic := uint32(0x02000000) // VFS_CAP_REVISION_2
	if caps.Effective {
		magic |= 1 // VFS_CAP_FLAGS_EFFECTIVE
	}
	binary.LittleEndian.PutUint32(data[0:4], magic)
	binary.LittleEndian.PutUint32(data[4:8], uint32(caps.Permitted))
	binary.LittleEndian.PutUint32(data[8:12], uint32(caps.Inheritable))
	binary.LittleEndian.PutUint32(data[12:16], uint32(caps.Permitted>>32))
	binary.LittleEndian.PutUint32(data[16:20], uint32(caps.Inheritable>>32))
	require.NoError(t, unix.Setxattr(filename, "security.capability", data, 0))
}

func TestFileCapabilities(t *testing.T) {
	SkipIfNotAvailable(t)
	checkKernelCompatibility(t, "exec file capability collection requires Linux 4.14 or newer", func(kv *kernel.Version) bool {
		return kv.Code < kernel.Kernel4_14
	})

	// /tmp can be mounted nosuid, in which case the kernel never reads file
	// capabilities at exec. Use a separate suid-enabled tmpfs for the fixtures.
	testRoot, err := createTempDir(t)
	require.NoError(t, err)
	require.NoError(t, unix.Mount("tmpfs", testRoot, "tmpfs", 0, "size=16M,mode=0755"))
	t.Cleanup(func() {
		assert.NoError(t, unix.Unmount(testRoot, unix.MNT_DETACH))
	})

	const bindService = uint64(1 << unix.CAP_NET_BIND_SERVICE)
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_binary_file_capabilities",
			Expression: `exec.file.path =~ "{{.Root}}/file-capabilities-*" && exec.file.capabilities.available`,
		},
		{
			ID:         "test_binary_without_file_capabilities",
			Expression: `exec.file.path =~ "{{.Root}}/file-capabilities-*" && !exec.file.capabilities.available`,
		},
		{
			ID:         "test_inherited_file_capabilities",
			Expression: `open.file.path == "/dev/null" && process.file.path == "{{.Root}}/file-capabilities-fork" && process.is_thread && process.file.capabilities.available && process.file.capabilities.effective && process.file.capabilities.permitted & CAP_NET_BIND_SERVICE != 0`,
		},
		{
			ID:         "test_file_capabilities_exec_reset",
			Expression: `exec.file.path == "{{.Root}}/no-capabilities-reexec" && !exec.file.capabilities.available && process.parent.file.capabilities.available`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testRoot}))
	require.NoError(t, err)
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*probe.EBPFProbe)
	require.True(t, ok)
	status := p.GetConstantFetcherStatus()
	require.NotNil(t, status)
	permittedAt := status.Values[constantfetch.OffsetNameCPUVFSCapDataStructPermitted].Value
	inheritableAt := status.Values[constantfetch.OffsetNameCPUVFSCapDataStructInheritable].Value
	if !status.IsPresent(constantfetch.OffsetNameCPUVFSCapDataStructPermitted) ||
		!status.IsPresent(constantfetch.OffsetNameCPUVFSCapDataStructInheritable) ||
		permittedAt == 0 || inheritableAt == 0 {
		t.Skip("exec file capabilities require cpu_vfs_cap_data offsets resolved through BTF/BTFHub")
	}

	for _, tc := range []struct {
		name string
		caps model.FileCapabilities
	}{
		{"effective", model.FileCapabilities{Permitted: bindService, Effective: true, Available: true}},
		{"permitted-only", model.FileCapabilities{Permitted: bindService, Available: true}},
		{"inheritable", model.FileCapabilities{Inheritable: 1 << unix.CAP_CHOWN, Available: true}},
		{"distinct-low-high-masks", model.FileCapabilities{
			Permitted:   bindService | 1<<unix.CAP_MAC_ADMIN,
			Inheritable: 1<<unix.CAP_SETUID | 1<<unix.CAP_AUDIT_READ,
			Available:   true,
		}},
		{"empty-xattr", model.FileCapabilities{Available: true}},
		{"no-xattr", model.FileCapabilities{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filename, _, err := test.Path("file-capabilities-" + tc.name)
			require.NoError(t, err)
			require.NoError(t, copyFile(which(t, "true"), filename, 0755))
			if tc.caps.Available {
				setTestFileCapabilities(t, filename, tc.caps)
			}
			ruleID := "test_binary_without_file_capabilities"
			if tc.caps.Available {
				ruleID = "test_binary_file_capabilities"
			}
			test.WaitSignalFromRule(t, func() error {
				return exec.Command(filename).Run()
			}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
				assert.Equal(t, ruleID, rule.ID)
				assert.Equal(t, tc.caps, event.Exec.FileCapabilities)
				assert.Equal(t, tc.caps, event.ProcessContext.FileCapabilities)
				assert.Zero(t, event.ProcessContext.CapsUsed&bindService, "declared capabilities are not runtime usage")
			}), ruleID)
		})
	}

	capableBinary, _, err := test.Path("file-capabilities-fork")
	require.NoError(t, err)
	require.NoError(t, copyFile(which(t, "sh"), capableBinary, 0755))
	caps := model.FileCapabilities{Permitted: bindService, Effective: true, Available: true}
	setTestFileCapabilities(t, capableBinary, caps)

	t.Run("interpreter-capabilities-not-attributed-to-script", func(t *testing.T) {
		script, _, err := test.Path("file-capabilities-script")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(script, []byte("#!"+capableBinary+"\nexit 0\n"), 0755))
		test.WaitSignalFromRule(t, func() error {
			return exec.Command(script).Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, model.FileCapabilities{}, event.Exec.FileCapabilities)
		}), "test_binary_without_file_capabilities")
	})

	t.Run("fork-context", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			// The background shell builtin opens /dev/null after fork, without exec.
			return exec.Command(capableBinary, "-c", ": </dev/null & wait").Run()
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, caps, event.ProcessContext.FileCapabilities)
		}, "test_inherited_file_capabilities")
	})

	t.Run("exec-resets-file-capabilities", func(t *testing.T) {
		plainBinary, _, err := test.Path("no-capabilities-reexec")
		require.NoError(t, err)
		require.NoError(t, copyFile(which(t, "true"), plainBinary, 0755))
		test.WaitSignalFromRule(t, func() error {
			return exec.Command(capableBinary, "-c", `exec "$1"`, "sh", plainBinary).Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, model.FileCapabilities{}, event.Exec.FileCapabilities)
			require.NotNil(t, event.ProcessContext.Parent)
			assert.Equal(t, caps, event.ProcessContext.Parent.FileCapabilities)
		}), "test_file_capabilities_exec_reset")
	})
}
