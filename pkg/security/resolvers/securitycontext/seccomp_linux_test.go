// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSeccompProfileJSON = `{
  "defaultAction": "SCMP_ACT_ERRNO",
  "syscalls": [
    {"names": ["read", "write"], "action": "SCMP_ACT_ALLOW"},
    {"names": ["clone"], "action": "SCMP_ACT_ALLOW",
     "args": [{"index": 0, "value": 2114060288, "op": "SCMP_CMP_MASKED_EQ"}]},
    {"names": ["socket"], "action": "SCMP_ACT_ERRNO",
     "args": [{"index": 0, "value": 16, "op": "SCMP_CMP_EQ"}]}
  ]
}`

func TestReadLocalhostSeccompProfile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "var/lib/kubelet/seccomp", "profiles")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "audit.json"), []byte(testSeccompProfileJSON), 0o600))
	t.Setenv("HOST_ROOT", root)

	res, err := readLocalhostSeccompProfile("profiles/audit.json")
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, ActionErrno, res.DefaultAction)

	// Names in a single entry share the same rule.
	assert.Equal(t, ActionAllow, res.Syscalls["read"].Action)
	assert.Empty(t, res.Syscalls["read"].ArgConditions)
	assert.Equal(t, ActionAllow, res.Syscalls["write"].Action)

	// SCMP_CMP_MASKED_EQ maps to "&", carrying the mask as Value.
	clone := res.Syscalls["clone"]
	assert.Equal(t, ActionAllow, clone.Action)
	require.Len(t, clone.ArgConditions, 1)
	assert.Equal(t, ArgCondition{Index: 0, Op: "&", Value: 2114060288, Action: ActionAllow}, clone.ArgConditions[0])

	// SCMP_CMP_EQ maps to "==".
	socket := res.Syscalls["socket"]
	assert.Equal(t, ActionErrno, socket.Action)
	require.Len(t, socket.ArgConditions, 1)
	assert.Equal(t, ArgCondition{Index: 0, Op: "==", Value: 16, Action: ActionErrno}, socket.ArgConditions[0])
}

func TestReadLocalhostSeccompProfileErrors(t *testing.T) {
	_, err := readLocalhostSeccompProfile("")
	assert.Error(t, err)

	t.Setenv("HOST_ROOT", t.TempDir())
	_, err = readLocalhostSeccompProfile("does-not-exist.json")
	assert.Error(t, err)
}

func TestSeccompProfilePathHostRoot(t *testing.T) {
	t.Setenv("HOST_ROOT", "/host")
	assert.Equal(t, "/host/var/lib/kubelet/seccomp/p.json", seccompProfilePath("p.json"))

	t.Setenv("HOST_ROOT", "")
	assert.Equal(t, "/var/lib/kubelet/seccomp/p.json", seccompProfilePath("p.json"))
}

func TestScmpActionToSeccompAction(t *testing.T) {
	cases := map[string]SeccompAction{
		"SCMP_ACT_ALLOW":        ActionAllow,
		"SCMP_ACT_ERRNO":        ActionErrno,
		"SCMP_ACT_KILL":         ActionKillThread,
		"SCMP_ACT_KILL_THREAD":  ActionKillThread,
		"SCMP_ACT_KILL_PROCESS": ActionKillProcess,
		"SCMP_ACT_TRAP":         ActionTrap,
		"SCMP_ACT_TRACE":        ActionTrace,
		"SCMP_ACT_LOG":          ActionLog,
		"SCMP_ACT_NOTIFY":       ActionNotify,
	}
	for in, want := range cases {
		assert.Equal(t, want, scmpActionToSeccompAction(in), in)
	}
	// Unknown actions pass through unchanged.
	assert.Equal(t, SeccompAction("SCMP_ACT_FUTURE"), scmpActionToSeccompAction("SCMP_ACT_FUTURE"))
}

func TestScmpOp(t *testing.T) {
	cases := map[string]string{
		"SCMP_CMP_EQ":        "==",
		"SCMP_CMP_NE":        "!=",
		"SCMP_CMP_LT":        "<",
		"SCMP_CMP_LE":        "<=",
		"SCMP_CMP_GT":        ">",
		"SCMP_CMP_GE":        ">=",
		"SCMP_CMP_MASKED_EQ": "&",
		"SCMP_CMP_UNKNOWN":   "",
	}
	for in, want := range cases {
		assert.Equal(t, want, scmpOp(in), in)
	}
}

func TestKubeletConfigSeccompDefault(t *testing.T) {
	writeConfig := func(t *testing.T, body string) {
		root := t.TempDir()
		dir := filepath.Join(root, "var/lib/kubelet")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600))
		t.Setenv("HOST_ROOT", root)
	}

	t.Run("enabled", func(t *testing.T) {
		writeConfig(t, "seccompDefault: true\n")
		assert.True(t, kubeletConfigSeccompDefault())
	})

	t.Run("disabled", func(t *testing.T) {
		writeConfig(t, "seccompDefault: false\n")
		assert.False(t, kubeletConfigSeccompDefault())
	})

	t.Run("unset", func(t *testing.T) {
		writeConfig(t, "kind: KubeletConfiguration\n")
		assert.False(t, kubeletConfigSeccompDefault())
	})

	t.Run("missing file", func(t *testing.T) {
		t.Setenv("HOST_ROOT", t.TempDir())
		assert.False(t, kubeletConfigSeccompDefault())
	})
}

func TestKubeletFlagSeccompDefault(t *testing.T) {
	writeCmdline := func(t *testing.T, args ...string) string {
		proc := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "1234"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proc, "1234", "cmdline"), []byte(strings.Join(args, "\x00")), 0o600))
		// A non-numeric entry must be ignored.
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "notapid"), 0o755))
		t.Setenv("HOST_PROC", proc)
		return proc
	}

	t.Run("bare flag", func(t *testing.T) {
		writeCmdline(t, "/usr/bin/kubelet", "--seccomp-default")
		val, found := kubeletFlagSeccompDefault()
		assert.True(t, found)
		assert.True(t, val)
	})

	t.Run("flag true", func(t *testing.T) {
		writeCmdline(t, "kubelet", "--seccomp-default=true")
		val, found := kubeletFlagSeccompDefault()
		assert.True(t, found)
		assert.True(t, val)
	})

	t.Run("flag false", func(t *testing.T) {
		writeCmdline(t, "kubelet", "--seccomp-default=false")
		val, found := kubeletFlagSeccompDefault()
		assert.True(t, found)
		assert.False(t, val)
	})

	t.Run("kubelet without flag", func(t *testing.T) {
		writeCmdline(t, "kubelet", "--v=2")
		val, found := kubeletFlagSeccompDefault()
		assert.True(t, found)
		assert.False(t, val)
	})

	t.Run("no kubelet process", func(t *testing.T) {
		writeCmdline(t, "/usr/bin/kube-proxy")
		_, found := kubeletFlagSeccompDefault()
		assert.False(t, found)
	})
}

func TestKubeletSeccompDefaultEnabled(t *testing.T) {
	// No kubelet process: value comes from the config file.
	t.Run("config drives when no flag", func(t *testing.T) {
		t.Setenv("HOST_PROC", t.TempDir())
		root := t.TempDir()
		dir := filepath.Join(root, "var/lib/kubelet")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("seccompDefault: true\n"), 0o600))
		t.Setenv("HOST_ROOT", root)
		assert.True(t, kubeletSeccompDefaultEnabled())
	})

	// The flag wins over a conflicting config file.
	t.Run("flag overrides config", func(t *testing.T) {
		proc := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "1"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proc, "1", "cmdline"), []byte("kubelet\x00--seccomp-default=false"), 0o600))
		t.Setenv("HOST_PROC", proc)

		root := t.TempDir()
		dir := filepath.Join(root, "var/lib/kubelet")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("seccompDefault: true\n"), 0o600))
		t.Setenv("HOST_ROOT", root)

		assert.False(t, kubeletSeccompDefaultEnabled())
	})
}
