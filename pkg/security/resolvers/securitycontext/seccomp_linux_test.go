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
	"time"

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
	t.Setenv("HOST_PROC", t.TempDir())

	res, err := readLocalhostSeccompProfile(nil, "profiles/audit.json")
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
	_, err := readLocalhostSeccompProfile(nil, "")
	assert.Error(t, err)

	t.Setenv("HOST_ROOT", t.TempDir())
	t.Setenv("HOST_PROC", t.TempDir())
	_, err = readLocalhostSeccompProfile(nil, "does-not-exist.json")
	assert.Error(t, err)
}

func TestReadLocalhostSeccompProfileFromKubeletRoot(t *testing.T) {
	proc := t.TempDir()
	dir := filepath.Join(proc, "4242", "root", "data/kubelet/seccomp")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(testSeccompProfileJSON), 0o600))
	t.Setenv("HOST_PROC", proc)
	t.Setenv("HOST_ROOT", "")

	res, err := readLocalhostSeccompProfile(&kubeletInfo{pid: "4242", rootDir: "/data/kubelet"}, "p.json")
	require.NoError(t, err)
	assert.Equal(t, ActionErrno, res.DefaultAction)
}

func TestResolveSeccompFilterAfterKubeletRestart(t *testing.T) {
	proc := t.TempDir()
	writeKubelet := func(pid string) {
		dir := filepath.Join(proc, pid, "root", "var/lib/kubelet/seccomp")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(testSeccompProfileJSON), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(proc, pid, "cmdline"), []byte("kubelet\x00"), 0o600))
	}
	t.Setenv("HOST_PROC", proc)
	t.Setenv("HOST_ROOT", "")

	writeKubelet("100")
	r := &WorkloadmetaResolver{kubelet: findKubelet()}
	require.NotNil(t, r.kubelet)

	require.NoError(t, os.RemoveAll(filepath.Join(proc, "100")))
	writeKubelet("200")

	res, err := r.ResolveSeccompFilter(&SeccompProfile{Type: SeccompLocalhost, LocalhostProfile: "p.json"})
	require.NoError(t, err)
	assert.Equal(t, ActionErrno, res.DefaultAction)
	assert.Equal(t, "200", r.kubelet.pid)
}

func TestResolveSeccompFilterIgnoresReusedKubeletPID(t *testing.T) {
	proc := t.TempDir()
	hostRoot := t.TempDir()
	writeProfile := func(root, body string) {
		dir := filepath.Join(root, "data/kubelet/seccomp")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(body), 0o600))
	}
	writeProcess := func(cmdline string) {
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "100"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proc, "100", "cmdline"), []byte(cmdline), 0o600))
	}
	t.Setenv("HOST_PROC", proc)
	t.Setenv("HOST_ROOT", hostRoot)
	writeProfile(hostRoot, testSeccompProfileJSON)

	writeProcess("kubelet\x00--root-dir=/data/kubelet\x00")
	r := &WorkloadmetaResolver{kubelet: findKubelet(), kubeletLookupTime: time.Now()}
	require.NotNil(t, r.kubelet)

	writeProcess("other\x00")
	writeProfile(filepath.Join(proc, "100", "root"), `{"defaultAction": "SCMP_ACT_ALLOW"}`)

	res, err := r.ResolveSeccompFilter(&SeccompProfile{Type: SeccompLocalhost, LocalhostProfile: "p.json"})
	require.NoError(t, err)
	assert.Equal(t, ActionErrno, res.DefaultAction)
	assert.Empty(t, r.kubelet.pid)
	assert.Equal(t, "/data/kubelet", r.kubelet.rootDir)
}

func TestReadLocalhostSeccompProfileFromInitRoot(t *testing.T) {
	proc := t.TempDir()
	dir := filepath.Join(proc, "1", "root", "var/lib/kubelet/seccomp")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(testSeccompProfileJSON), 0o600))
	t.Setenv("HOST_PROC", proc)
	t.Setenv("HOST_ROOT", "")

	_, err := readLocalhostSeccompProfile(nil, "p.json")
	require.NoError(t, err)
}

func TestHostRoots(t *testing.T) {
	t.Setenv("HOST_PROC", "/hp")
	t.Setenv("HOST_ROOT", "/host")
	assert.Equal(t, []string{"/hp/7/root", "/host", "/hp/1/root", "/"}, hostRoots(&kubeletInfo{pid: "7"}))
	assert.Equal(t, []string{"/host", "/hp/1/root", "/"}, hostRoots(&kubeletInfo{rootDir: "/data/kubelet"}))

	t.Setenv("HOST_ROOT", "")
	assert.Equal(t, []string{"/hp/1/root", "/"}, hostRoots(nil))
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
		t.Setenv("HOST_PROC", t.TempDir())
	}

	t.Run("enabled", func(t *testing.T) {
		writeConfig(t, "seccompDefault: true\n")
		assert.True(t, kubeletConfigSeccompDefault(nil))
	})

	t.Run("disabled", func(t *testing.T) {
		writeConfig(t, "seccompDefault: false\n")
		assert.False(t, kubeletConfigSeccompDefault(nil))
	})

	t.Run("unset", func(t *testing.T) {
		writeConfig(t, "kind: KubeletConfiguration\n")
		assert.False(t, kubeletConfigSeccompDefault(nil))
	})

	t.Run("missing file", func(t *testing.T) {
		t.Setenv("HOST_ROOT", t.TempDir())
		t.Setenv("HOST_PROC", t.TempDir())
		assert.False(t, kubeletConfigSeccompDefault(nil))
	})

	t.Run("config flag path under kubelet root", func(t *testing.T) {
		proc := t.TempDir()
		dir := filepath.Join(proc, "9", "root", "etc/kubernetes")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kubelet.yaml"), []byte("seccompDefault: true\n"), 0o600))
		t.Setenv("HOST_PROC", proc)
		t.Setenv("HOST_ROOT", "")
		assert.True(t, kubeletConfigSeccompDefault(&kubeletInfo{pid: "9", configPath: "/etc/kubernetes/kubelet.yaml"}))
	})
}

func TestFindKubelet(t *testing.T) {
	writeCmdline := func(t *testing.T, args ...string) {
		proc := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "1234"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proc, "1234", "cmdline"), []byte(strings.Join(args, "\x00")+"\x00"), 0o600))
		// A non-numeric entry must be ignored.
		require.NoError(t, os.MkdirAll(filepath.Join(proc, "notapid"), 0o755))
		t.Setenv("HOST_PROC", proc)
	}
	boolPtr := func(b bool) *bool { return &b }

	cases := []struct {
		name string
		args []string
		want *kubeletInfo
	}{
		{"bare seccomp flag", []string{"/usr/bin/kubelet", "--seccomp-default"},
			&kubeletInfo{pid: "1234", rootDir: defaultKubeletRootDir, seccompDefault: boolPtr(true)}},
		{"seccomp flag true", []string{"kubelet", "--seccomp-default=true"},
			&kubeletInfo{pid: "1234", rootDir: defaultKubeletRootDir, seccompDefault: boolPtr(true)}},
		{"seccomp flag false", []string{"kubelet", "--seccomp-default=false"},
			&kubeletInfo{pid: "1234", rootDir: defaultKubeletRootDir, seccompDefault: boolPtr(false)}},
		{"no flags", []string{"kubelet", "--v=2"},
			&kubeletInfo{pid: "1234", rootDir: defaultKubeletRootDir}},
		{"root-dir and config, both forms", []string{"kubelet", "--root-dir=/data/kubelet", "--config", "/etc/kubelet.yaml"},
			&kubeletInfo{pid: "1234", rootDir: "/data/kubelet", configPath: "/etc/kubelet.yaml"}},
		{"no kubelet process", []string{"/usr/bin/kube-proxy"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeCmdline(t, tc.args...)
			assert.Equal(t, tc.want, findKubelet())
		})
	}
}

func TestKubeletSeccompDefaultEnabled(t *testing.T) {
	writeConfig := func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "var/lib/kubelet")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("seccompDefault: true\n"), 0o600))
		t.Setenv("HOST_ROOT", root)
		t.Setenv("HOST_PROC", t.TempDir())
	}

	t.Run("config drives when no flag", func(t *testing.T) {
		writeConfig(t)
		assert.True(t, kubeletSeccompDefaultEnabled(nil))
	})

	// The flag wins over a conflicting config file.
	t.Run("flag overrides config", func(t *testing.T) {
		writeConfig(t)
		off := false
		assert.False(t, kubeletSeccompDefaultEnabled(&kubeletInfo{pid: "1", rootDir: defaultKubeletRootDir, seccompDefault: &off}))
	})
}
