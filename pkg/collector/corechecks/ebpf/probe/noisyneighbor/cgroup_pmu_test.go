// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScalePMUDelta_overflowProtection separately asserts the function
// doesn't panic on adversarial inputs, including running > enabled (which
// causes the subtraction inside scalePMUDelta to wrap under uint64) — we
// only require the function to return without panicking, since the result
// is mathematically meaningless when the kernel violates the invariant.
func TestScalePMUDelta_overflowProtection(t *testing.T) {
	cases := []struct{ counter, enabled, running uint64 }{
		{100, 100, 200},         // running > enabled
		{1<<63 - 1, 1 << 63, 1}, // counter near uint64 limit
		{1, 1<<63 + 1, 1 << 62}, // large enabled & running spread
	}
	for _, c := range cases {
		assert.NotPanics(t, func() {
			_ = scalePMUDelta(c.counter, c.enabled, c.running)
		})
	}
}

// TestScalePMUDelta exercises every branch of the multiplexing-scaling
// arithmetic. The scaling formula is counter + counter*(enabled-running)/running,
// chosen instead of counter*enabled/running to keep the multiplication operands
// small in the common multiplexing case. The fast-path returns counter
// untouched when enabled == running.
func TestScalePMUDelta(t *testing.T) {
	tests := []struct {
		name    string
		counter uint64
		enabled uint64
		running uint64
		want    uint64
	}{
		{
			name:    "running=0 returns 0 even with non-zero counter",
			counter: 12345,
			enabled: 1_000_000,
			running: 0,
			want:    0,
		},
		{
			name:    "no multiplexing (enabled == running) returns raw counter",
			counter: 1_000_000,
			enabled: 5_000_000,
			running: 5_000_000,
			want:    1_000_000,
		},
		{
			name:    "no multiplexing with zero counter returns zero",
			counter: 0,
			enabled: 5_000_000,
			running: 5_000_000,
			want:    0,
		},
		{
			name:    "exact half multiplexing doubles the counter",
			counter: 1_000_000,
			enabled: 2_000_000,
			running: 1_000_000,
			want:    2_000_000,
		},
		{
			name:    "quarter multiplexing multiplies counter by 4x",
			counter: 1_000_000,
			enabled: 4_000_000,
			running: 1_000_000,
			want:    4_000_000,
		},
		{
			name:    "small multiplexing slice (1% missing)",
			counter: 990_000,
			enabled: 1_000_000,
			running: 990_000,
			want:    990_000 + (990_000*10_000)/990_000,
		},
		{
			name:    "zero counter returns 0 even when multiplexed",
			counter: 0,
			enabled: 4_000_000,
			running: 1_000_000,
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scalePMUDelta(tt.counter, tt.enabled, tt.running)
			assert.Equalf(t, tt.want, got,
				"scalePMUDelta(counter=%d, enabled=%d, running=%d)",
				tt.counter, tt.enabled, tt.running)
		})
	}
}

// TestClassifyCgroupName covers every input shape walkContainerCgroups can
// encounter on cgroup v2 hierarchies in the wild: container scopes named with
// the standard 64-char hex id, AWS ECS hex-with-suffix, Garden UUIDs, systemd
// `.mount` aliases that share a container id with the real scope, and the
// crio-conmon / libpod-conmon monitor cgroups that hold no relevant procs.
func TestClassifyCgroupName(t *testing.T) {
	const hex64 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	const ecs = "abcdef0123456789abcdef0123456789-1234"
	// Garden uses 8-4-4-4-4 hex groups (NOT the standard 8-4-4-4-12 UUID).
	const garden = "12345678-aaaa-bbbb-cccc-dddd"

	tests := []struct {
		name string
		in   string
		want cgroupKind
	}{
		{"empty", "", cgroupOther},
		{"plain slice", "kubelet.slice", cgroupOther},
		{"system slice", "system.slice", cgroupOther},
		{"user slice", "user.slice", cgroupOther},
		{"besteffort pod slice", "kubelet-kubepods-besteffort-pod1234abcd.slice", cgroupOther},
		{"hex64 container id", hex64, cgroupContainer},
		{"docker scope", "docker-" + hex64 + ".scope", cgroupContainer},
		{"cri-containerd scope", "cri-containerd-" + hex64 + ".scope", cgroupContainer},
		{"ECS hex-with-suffix", ecs, cgroupContainer},
		{"Garden UUID", garden, cgroupContainer},
		{"systemd .mount alias", hex64 + ".mount", cgroupSkip},
		{"docker .mount alias", "docker-" + hex64 + ".mount", cgroupSkip},
		{"crio-conmon monitor", "crio-conmon-" + hex64, cgroupSkip},
		{"libpod-conmon monitor", "libpod-conmon-" + hex64, cgroupSkip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, classifyCgroupName(tt.in),
				"classifyCgroupName(%q)", tt.in)
		})
	}
}

// TestStatInode validates the syscall.Stat → inode plumbing against a real
// tempdir. Existence-positive and ENOENT-negative cases — the function is a
// thin wrapper but it's the only place we inject a non-mock inode into the
// PMU manager, so a regression here would silently break cgroup-id matching
// with what BPF reports.
func TestStatInode(t *testing.T) {
	t.Run("existing directory returns non-zero inode matching os.Stat", func(t *testing.T) {
		dir := t.TempDir()
		got, err := statInode(dir)
		require.NoError(t, err)
		assert.NotZero(t, got)

		// Cross-check against os.Stat's view of the same path.
		fi, err := os.Stat(dir)
		require.NoError(t, err)
		osInode := fi.Sys().(*syscall.Stat_t).Ino
		assert.Equal(t, osInode, got)
	})

	t.Run("missing path returns error", func(t *testing.T) {
		_, err := statInode("/this/path/should/never/exist/abc123")
		assert.Error(t, err)
	})
}

// TestWalkContainerCgroups builds a synthetic cgroup tree that mirrors the
// kubelet.slice / containerd / Garden layouts seen in production and verifies
// only container scopes are returned, while .mount aliases and conmon monitor
// cgroups are skipped (and skipped means we don't descend either — a nested
// container under a conmon dir should not appear).
func TestWalkContainerCgroups(t *testing.T) {
	const (
		hex1   = "1111111111111111111111111111111111111111111111111111111111111111"
		hex2   = "2222222222222222222222222222222222222222222222222222222222222222"
		hex3   = "3333333333333333333333333333333333333333333333333333333333333333"
		hex4   = "4444444444444444444444444444444444444444444444444444444444444444"
		hex5   = "5555555555555555555555555555555555555555555555555555555555555555"
		hex6   = "6666666666666666666666666666666666666666666666666666666666666666"
		garden = "12345678-aaaa-bbbb-cccc-dddd"
	)

	root := t.TempDir()

	// k8s + containerd: deep nesting under kubelet.slice
	makeDirs := []string{
		"kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/" +
			"kubelet-kubepods-besteffort-podabc.slice/cri-containerd-" + hex1 + ".scope",
		"kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/" +
			"kubelet-kubepods-burstable-poddef.slice/cri-containerd-" + hex2 + ".scope",
		// Standalone Docker scope
		"system.slice/docker-" + hex3 + ".scope",
		// systemd .mount alias — must be classified Skip
		"system.slice/docker-" + hex4 + ".scope.mount",
		// crio-conmon — must Skip and not descend
		"system.slice/crio-conmon-" + hex5 + "/nested-fake-" + hex6 + ".scope",
		// Garden / Cloud Foundry layout
		"garden/" + garden,
		// Non-container directories that should NOT be picked up
		"user.slice/user-1000.slice",
		"init.scope",
	}
	for _, rel := range makeDirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, rel), 0o755))
	}

	seen, err := walkContainerCgroups(root)
	require.NoError(t, err)

	// Build the expected set of paths and verify by absolute path, not by
	// inode (inodes are filesystem-specific and unstable across reruns).
	gotPaths := make(map[string]struct{}, len(seen))
	for _, p := range seen {
		gotPaths[p] = struct{}{}
	}

	wantPaths := []string{
		filepath.Join(root, "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/"+
			"kubelet-kubepods-besteffort-podabc.slice/cri-containerd-"+hex1+".scope"),
		filepath.Join(root, "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/"+
			"kubelet-kubepods-burstable-poddef.slice/cri-containerd-"+hex2+".scope"),
		filepath.Join(root, "system.slice/docker-"+hex3+".scope"),
		filepath.Join(root, "garden/"+garden),
	}
	for _, p := range wantPaths {
		assert.Contains(t, gotPaths, p, "expected container scope %s", p)
	}
	assert.Len(t, gotPaths, len(wantPaths), "got unexpected entries: %v", gotPaths)

	// Affirmative skip: the nested fake scope under crio-conmon must NOT be
	// returned even though its name matches the container regex — SkipDir
	// must short-circuit the descent.
	for p := range gotPaths {
		assert.NotContains(t, p, "crio-conmon-", "must not descend into conmon: %s", p)
		assert.NotContains(t, p, ".scope.mount", "must skip .mount aliases: %s", p)
	}

	// Sanity: every returned inode is a real kernfs inode and matches what
	// stat() reports on the returned path. This is what BPF's
	// bpf_get_current_cgroup_id matches against.
	for inode, path := range seen {
		got, err := statInode(path)
		require.NoError(t, err)
		assert.Equal(t, inode, got, "map key inode must equal statInode(path)")
	}
}

// TestWalkContainerCgroups_NonexistentRoot verifies the walk doesn't panic
// when the cgroup root is missing (e.g., the system-probe ran outside its
// usual container or /host/proc wasn't mounted yet at startup). The walker
// swallows per-entry stat errors and just returns an empty result — this
// matches the behaviour Refresh() expects so a missing tree doesn't crash
// the probe at startup. The nil-error part of the contract is what makes
// Refresh() safe to call before any cgroups appear; lock it down.
func TestWalkContainerCgroups_NonexistentRoot(t *testing.T) {
	seen, err := walkContainerCgroups("/this/path/should/never/exist/cgrp")
	require.NoError(t, err, "missing root must surface as empty result, not error (so Refresh is safe at startup)")
	assert.Empty(t, seen)
}

// TestCgroupPMUEntry_close_idempotent asserts close() does not panic when
// called on a zero-value entry or twice in a row. cgroupFD is -1 in the zero
// value, which the close() guard catches; subsequent calls must remain safe.
func TestCgroupPMUEntry_close_idempotent(t *testing.T) {
	e := &cgroupPMUEntry{cgroupFD: -1}
	assert.NotPanics(t, e.close)
	assert.NotPanics(t, e.close)
}

// TestNewCgroupPMUManager checks construction sets the expected initial state.
// probeSupported() is called inside the constructor; when the root path
// doesn't exist (or is unprivileged), it logs and returns without populating
// `supported`, which leaves every event marked false. This is the desired
// behavior so the manager can be safely constructed on non-Linux test hosts.
func TestNewCgroupPMUManager(t *testing.T) {
	// Use a tempdir so probeSupported's unix.Open succeeds without surprising
	// the test environment, but the per-event opens will fail (the tempdir
	// isn't a real cgroup directory).
	mgr := newCgroupPMUManager(t.TempDir())
	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.entries)
	assert.Empty(t, mgr.entries)
	assert.Greater(t, mgr.numCPU, 0)
	// `supported` should be all-false since perf_event_open against a normal
	// directory (not a cgroup) returns EBADF/EINVAL.
	for i, ok := range mgr.supported {
		assert.False(t, ok, "event %d (%s) unexpectedly marked supported", i, pmuEvents[i].name)
	}
}

// TestCgroupPMUManager_Close_empty checks Close() is safe on a freshly-built
// manager with no entries (the typical early-exit path when probeSupported
// rejected every event).
func TestCgroupPMUManager_Close_empty(t *testing.T) {
	mgr := newCgroupPMUManager(t.TempDir())
	assert.NotPanics(t, mgr.Close)
}

// TestCgroupPMUManager_ReadAll_emptyEntries verifies ReadAll returns an empty
// map (not nil) when nothing is tracked yet, since callers iterate over the
// return value without a nil-check.
func TestCgroupPMUManager_ReadAll_emptyEntries(t *testing.T) {
	mgr := newCgroupPMUManager(t.TempDir())
	got := mgr.ReadAll()
	require.NotNil(t, got)
	assert.Empty(t, got)
}

// TestPMUEvents_stable freezes the supported event set's order and identity.
// The order is load-bearing — ReadAll indexes into pmuEvents via the same
// integer that ends up as a struct field, so a reorder would silently
// misattribute metric values.
func TestPMUEvents_stable(t *testing.T) {
	require.Equal(t, 7, numPMUEvents, "numPMUEvents must match the array length")
	expectedOrder := []string{
		"cycles",
		"instructions",
		"llc_misses",
		"branch_misses",
		"cache_references",
		"itlb_misses",
		"cpu_migrations",
	}
	require.Len(t, pmuEvents, len(expectedOrder))
	for i, want := range expectedOrder {
		assert.Equal(t, want, pmuEvents[i].name, "event index %d", i)
	}
}

// TestPerfReadValueSize freezes the read-buffer layout. The kernel writes
// exactly 24 bytes when read_format is TOTAL_TIME_ENABLED|TOTAL_TIME_RUNNING;
// a struct that doesn't match would corrupt every read.
func TestPerfReadValueSize(t *testing.T) {
	assert.Equal(t, 24, perfReadValueSize, "perfReadValue must be exactly 24 bytes (3 × u64)")
}
