// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package oomkill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	bpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

// oomFillFile is the tmpfs file written to exhaust the memory of the cgroup in triggerOOMKill
const oomFillFile = "/dev/shm/asdf"

func TestOOMKillCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		cfg := testConfig(false)
		cfg.BPFDebug = true
		out, err := runtime.OomKill.Compile(&cfg.Config, []string{"-g"})
		require.NoError(t, err)
		_ = out.Close()
	})
}

func TestOOMKillProbe(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		for _, progName := range []string{oomFentryName, oomKprobeName} {
			t.Run(progName, func(t *testing.T) {
				if progName == oomFentryName {
					skipIfFentryUnsupported(t)
				} else {
					disableFentry(t)
				}
				cfg := testConfig(false)
				testOOMKillProbe(t, cfg, progName)
			})
		}
	})
}

func testOOMKillProbe(t *testing.T, cfg *Config, progName string) {
	setTestPinDir(t)
	oomKillProbe, err := NewProbe(cfg)
	require.NoError(t, err)
	t.Cleanup(oomKillProbe.Close)

	require.Equal(t, progName, oomKillProbe.progName, "unexpected program attached")
	_, loaded := oomKillProbe.coll.Programs[otherProgram(progName)]
	assert.False(t, loaded, "the unused program should not be loaded")

	pid := triggerOOMKill(t)

	var result model.OOMKillStats
	require.Eventually(t, func() bool {
		for _, r := range oomKillProbe.GetAndFlush() {
			if r.TriggerPid == uint32(pid) {
				result = r
				return true
			}
		}
		return false
	}, 10*time.Second, 500*time.Millisecond, "failed to find an OOM killed process with pid %d", pid)

	assert.Regexp(t, regexp.MustCompile(`run-.+\.scope`), result.CgroupName, "cgroup name")
	assert.Equal(t, result.TriggerPid, result.VictimPid, "tpid == pid")
	assert.NotZero(t, result.Score, "score")
	assert.Equal(t, int16(42), result.ScoreAdj, "score adj")
	assert.Equal(t, "dd", result.VictimComm, "victim comm")
	assert.Equal(t, "dd", result.TriggerComm, "trigger comm")
	assert.NotZero(t, result.Pages, "pages")
	assert.Equal(t, uint32(1), result.MemCgOOM, "memcg oom")
}

func otherProgram(progName string) string {
	if progName == oomFentryName {
		return oomKprobeName
	}
	return oomFentryName
}

// triggerOOMKill runs a process in a memory-limited cgroup until it is OOM killed, and returns its pid
func triggerOOMKill(t *testing.T) int {
	t.Cleanup(func() {
		out, err := exec.Command("swapon", "-a").CombinedOutput()
		if err != nil {
			t.Logf("swapon -a: %s: %s", err, out)
		}
	})
	require.NoError(t, exec.Command("swapoff", "-a").Run())

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		if err := os.Remove(oomFillFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Logf("failed to remove %s: %s", oomFillFile, err)
		}
	})

	err := os.WriteFile("/proc/self/oom_score_adj", []byte("42"), 0644)
	require.NoError(t, err)

	// Retry the dd command until the OOM killer actually kills it.
	// Sometimes the kernel returns ENOMEM to the write() syscall instead of
	// invoking the OOM killer, causing dd to exit gracefully with status 1.
	memoryProperty := memoryLimitProperty(t)
	var cmd *exec.Cmd
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		cmd = exec.CommandContext(ctx, "systemd-run", "--scope", "-p", memoryProperty+"=1M", "dd", "if=/dev/zero", "of="+oomFillFile, "bs=1K", "count=2K")
		obytes, err := cmd.CombinedOutput()
		output := string(obytes)
		require.Error(c, err)
		require.NotErrorIs(c, err, context.DeadlineExceeded)

		var exiterr *exec.ExitError
		require.ErrorAs(c, err, &exiterr, output)

		status, sok := exiterr.Sys().(syscall.WaitStatus)
		require.True(c, sok, output)

		if status.Signaled() {
			require.Equal(c, unix.SIGKILL, status.Signal(), output)
		} else {
			require.Equal(c, 128+unix.SIGKILL, status.ExitStatus(), output)
		}
	}, 30*time.Second, 1*time.Second, "failed to trigger OOM kill after multiple attempts")
	return cmd.Process.Pid
}

func TestOOMKillProbePinnedTakeover(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		for _, persist := range []bool{true, false} {
			t.Run(fmt.Sprintf("persist_monitoring=%t", persist), func(t *testing.T) {
				testPinnedTakeover(t, persist)
			})
		}
	})
}

// testPinnedTakeover checks that a probe left pinned by a system-probe that died is taken over
// by the next one, which reports the OOM kills recorded in between. With persistMonitoring false,
// the new probe must also remove the pins.
func testPinnedTakeover(t *testing.T, persistMonitoring bool) {
	skipIfPinningUnsupported(t)
	setTestPinDir(t)

	first, err := NewProbe(testConfig(true))
	require.NoError(t, err)
	t.Cleanup(first.Close)
	require.True(t, first.pinned, "first probe should be pinned when pinning is supported")
	firstLinkID := linkID(t, first.link)
	firstProgID := programID(t, first.coll.Programs[first.progName])

	// Simulate system-probe dying without a graceful shutdown: release the file descriptors
	// but leave the pins in place, so the probe keeps running.
	require.NoError(t, first.link.Close())
	first.coll.Close()

	// This OOM kill happens while no system-probe is running
	pid := triggerOOMKill(t)

	second, err := NewProbe(testConfig(persistMonitoring))
	require.NoError(t, err)
	t.Cleanup(second.Close)
	assert.Equal(t, persistMonitoring, second.pinned, "second probe should own the pins only if persisting")
	// The reused map would hold the OOM kill even if the second probe had attached its own program,
	// so check that the pinned link and program were taken over rather than replaced.
	assert.Equal(t, firstLinkID, linkID(t, second.link), "second probe should take over the pinned link")
	assert.Equal(t, firstProgID, programID(t, second.coll.Programs[second.progName]), "second probe should take over the pinned program")
	if persistMonitoring {
		assertPins(t, "pins should be kept after taking over the probe")
	} else {
		assertNoPins(t, "pins should be removed when persist_monitoring is disabled")
	}

	found := slices.ContainsFunc(second.GetAndFlush(), func(r model.OOMKillStats) bool {
		return r.TriggerPid == uint32(pid)
	})
	assert.True(t, found, "OOM kill of pid %d recorded while no probe was running should be reported", pid)

	second.Close()
	assertNoPins(t, "graceful close should remove the pins")
}

func TestOOMKillMapCompatibility(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}
		require.NoError(t, rlimit.RemoveMemlock())
		mapSpec := loadOOMMapSpec(t)

		t.Run("same version", func(t *testing.T) {
			m := newTestMap(t, mapSpec.Copy())
			assert.NoError(t, compatibleMap(mapSpec, m))
		})

		t.Run("different version", func(t *testing.T) {
			pinnedSpec := mapSpec.Copy()
			setOOMStatsVersion(t, pinnedSpec, "oom_stats_v0")
			m := newTestMap(t, pinnedSpec)

			// The marker is zero-size, so only the version check can tell the maps apart
			require.NoError(t, mapSpec.Compatible(m))
			err := compatibleMap(mapSpec, m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pinned map has version oom_stats_v0")
		})

		t.Run("no version marker", func(t *testing.T) {
			pinnedSpec := mapSpec.Copy()
			value, ok := btf.As[*btf.Struct](pinnedSpec.Value)
			require.True(t, ok)
			value.Members = slices.DeleteFunc(value.Members, func(m btf.Member) bool {
				return m.Name == oomStatsVersionField
			})
			m := newTestMap(t, pinnedSpec)

			require.NoError(t, mapSpec.Compatible(m))
			assert.ErrorContains(t, compatibleMap(mapSpec, m), "has no field "+oomStatsVersionField)
		})
	})
}

// loadOOMMapSpec returns the spec of the oom_stats map of the probe for the current build mode
func loadOOMMapSpec(t *testing.T) *bpflib.MapSpec {
	cfg := testConfig(false)
	var spec *bpflib.CollectionSpec
	if cfg.EnableCORE {
		err := ebpf.LoadCOREAsset("oom-kill.o", func(buf bytecode.AssetReader, _ manager.Options) error {
			var err error
			spec, err = bpflib.LoadCollectionSpecFromReader(buf)
			return err
		})
		require.NoError(t, err)
	} else {
		buf, err := runtime.OomKill.Compile(&cfg.Config, getCFlags(&cfg.Config))
		require.NoError(t, err)
		defer buf.Close()
		spec, err = bpflib.LoadCollectionSpecFromReader(buf)
		require.NoError(t, err)
	}

	mapSpec, ok := spec.Maps[oomMapName]
	require.True(t, ok, "map %s not found", oomMapName)
	return mapSpec
}

// newTestMap creates a map from spec, skipping the test if the kernel does not keep its BTF
func newTestMap(t *testing.T, spec *bpflib.MapSpec) *bpflib.Map {
	m, err := bpflib.NewMap(spec)
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	h, err := m.Handle()
	if errors.Is(err, bpflib.ErrNotSupported) {
		t.Skipf("map BTF is not supported: %s", err)
	}
	require.NoError(t, err)
	_ = h.Close()
	return m
}

// setOOMStatsVersion renames the version marker of the oom_stats value of spec
func setOOMStatsVersion(t *testing.T, spec *bpflib.MapSpec, version string) {
	value, ok := btf.As[*btf.Struct](spec.Value)
	require.True(t, ok, "value of map %s is not a struct", oomMapName)
	for i, m := range value.Members {
		if m.Name != oomStatsVersionField {
			continue
		}
		td, ok := m.Type.(*btf.Typedef)
		require.True(t, ok, "field %s is not a typedef", oomStatsVersionField)
		value.Members[i].Type = &btf.Typedef{Name: version, Type: td.Type}
		return
	}
	require.FailNow(t, "version marker not found", "struct %s has no field %s", value.Name, oomStatsVersionField)
}

// disableFentry makes fentry detection fail for the duration of the test, so the kprobe program is used
func disableFentry(t *testing.T) {
	orig := supportsFentry
	supportsFentry = func(string) error { return errors.New("fentry disabled by test") }
	t.Cleanup(func() { supportsFentry = orig })
}

// skipIfFentryUnsupported skips the test unless the kernel supports fentry on oom_kill_process
func skipIfFentryUnsupported(t *testing.T) {
	if err := supportsFentry(oomHookName); err != nil {
		t.Skipf("fentry is not supported on %s: %s", oomHookName, err)
	}
}

// skipIfPinningUnsupported skips the test unless fentry is used, bpffs is available and fentry
// links can be pinned
func skipIfPinningUnsupported(t *testing.T) {
	skipIfFentryUnsupported(t)
	if !bpffsAvailable() {
		t.Skipf("bpffs is not mounted at %s", bpffsRoot)
	}
	if err := fentryPinningSupported(); err != nil {
		t.Skipf("pinning fentry links is not supported: %s", err)
	}
}

// memoryLimitProperty returns the systemd property limiting the memory of a unit: MemoryLimit for
// cgroup v1 (also used by the hybrid layout) and MemoryMax for cgroup v2, since recent systemd
// versions ignore MemoryLimit.
func memoryLimitProperty(t *testing.T) string {
	var st unix.Statfs_t
	require.NoError(t, unix.Statfs("/sys/fs/cgroup", &st))
	if st.Type == unix.CGROUP2_SUPER_MAGIC {
		return "MemoryMax"
	}
	return "MemoryLimit"
}

func linkID(t *testing.T, l link.Link) link.ID {
	info, err := l.Info()
	require.NoError(t, err)
	return info.ID
}

func programID(t *testing.T, p *bpflib.Program) bpflib.ProgramID {
	info, err := p.Info()
	require.NoError(t, err)
	id, ok := info.ID()
	require.True(t, ok, "program ID not available")
	return id
}

func assertPins(t *testing.T, msg string) {
	_, err := os.Stat(linkPinPath(oomLinkName))
	assert.NoError(t, err, "link pin: %s", msg)
	_, err = os.Stat(mapPinPath(oomMapName))
	assert.NoError(t, err, "map pin: %s", msg)
}

func assertNoPins(t *testing.T, msg string) {
	_, err := os.Stat(linkPinPath(oomLinkName))
	assert.ErrorIs(t, err, os.ErrNotExist, "link pin: %s", msg)
	_, err = os.Stat(mapPinPath(oomMapName))
	assert.ErrorIs(t, err, os.ErrNotExist, "map pin: %s", msg)
}

func setTestPinDir(t *testing.T) {
	prev := pinDir
	testRoot := filepath.Join(bpffsRoot, "datadog-agent-test")
	pinDir = filepath.Join(testRoot, strings.ReplaceAll(t.Name(), "/", "_"))
	t.Cleanup(func() {
		_ = os.RemoveAll(pinDir)
		// Only removed once empty, so that the pins of other tests are left alone
		_ = os.Remove(testRoot)
		pinDir = prev
	})
}

func testConfig(persistMonitoring bool) *Config {
	cfg := NewConfig()
	cfg.PersistMonitoring = persistMonitoring
	return cfg
}
