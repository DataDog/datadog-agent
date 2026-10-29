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

		cfg := testConfig()
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
				cfg := testConfig()
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

// TestOOMKillCgroupMemoryStats checks that the memory statistics of the cgroup are reported for a
// cgroup OOM kill only when oom_kill.emit_cgroup_memory_stats is enabled and the fentry program is
// used.
func TestOOMKillCgroupMemoryStats(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		t.Run("enabled", func(t *testing.T) {
			skipIfCgroupMemoryStatsUnsupported(t)
			assertCgroupMemoryStats(t, cgroupOOMKillStats(t, true))
		})

		t.Run("disabled", func(t *testing.T) {
			skipIfFentryUnsupported(t)
			stats := cgroupOOMKillStats(t, false)
			assert.Nil(t, stats, "no cgroup memory stats should be reported when disabled")
		})
	})
}

func assertCgroupMemoryStats(t *testing.T, stats *model.CgroupMemoryStats) {
	require.NotNil(t, stats, "cgroup memory stats should be reported")
	assert.NotZero(t, stats.Usage, "usage")
	assert.NotZero(t, stats.ActiveAnon+stats.InactiveAnon+stats.ActiveFile+stats.InactiveFile, "lru pages")
	assert.NotZero(t, stats.Kernel, "kernel memory")
}

// cgroupOOMKillStats triggers a cgroup OOM kill with a probe configured to emit the cgroup memory
// stats or not, and returns the cgroup memory stats reported for it
func cgroupOOMKillStats(t *testing.T, emit bool) *model.CgroupMemoryStats {
	setTestPinDir(t)
	p, err := NewProbe(testConfig(withCgroupMemoryStats(emit)))
	require.NoError(t, err)
	t.Cleanup(p.Close)

	pid := triggerOOMKill(t)

	var result model.OOMKillStats
	require.Eventually(t, func() bool {
		for _, r := range p.GetAndFlush() {
			if r.TriggerPid == uint32(pid) {
				result = r
				return true
			}
		}
		return false
	}, 10*time.Second, 500*time.Millisecond, "failed to find an OOM killed process with pid %d", pid)

	require.Equal(t, uint32(1), result.MemCgOOM, "the OOM kill should be a cgroup OOM kill")
	return result.CgroupMemory
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

	first, err := NewProbe(testConfig(withPersistMonitoring(true)))
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

	second, err := NewProbe(testConfig(withPersistMonitoring(persistMonitoring)))
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

// TestOOMKillMemcgFlushPinnedTakeover checks that the memcg flush programs pinned with the oom-kill
// probe keep counting the memcg stats flushes after system-probe died, so that the cgroup memory
// stats of the OOM kills recorded in between are checked, and that the next probe takes them over.
func TestOOMKillMemcgFlushPinnedTakeover(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfPinningUnsupported(t)
		skipIfCgroupMemoryStatsUnsupported(t)
		setTestPinDir(t)

		cfg := testConfig(withPersistMonitoring(true), withCgroupMemoryStats(true))
		firstLinkID, firstFlushLinkIDs := crashPinnedMemcgFlushProbe(t, cfg)
		assertMemcgFlushCounting(t, "pinned memcg flush probes should keep counting the flushes")

		// This OOM kill happens while no system-probe is running
		pid := triggerOOMKill(t)

		second, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(second.Close)
		require.True(t, second.pinned, "second probe should own the pins")
		assert.Equal(t, firstLinkID, linkID(t, second.link), "second probe should take over the pinned oom-kill link")
		assert.Equal(t, firstFlushLinkIDs, memcgFlushLinkIDs(t, second), "second probe should take over the pinned memcg flush links")
		assertPins(t, "pins should be kept after taking over the oom-kill probe")
		assertMemcgFlushPins(t, "pins should be kept after taking over the memcg flush probes")

		results := second.GetAndFlush()
		idx := slices.IndexFunc(results, func(r model.OOMKillStats) bool {
			return r.TriggerPid == uint32(pid)
		})
		require.NotEqual(t, -1, idx, "OOM kill of pid %d recorded while no probe was running should be reported", pid)
		assertCgroupMemoryStats(t, results[idx].CgroupMemory)
	})
}

// TestOOMKillMemcgFlushPartialPins checks the takeover of a probe collecting the cgroup memory stats
// when only part of its pins are left.
func TestOOMKillMemcgFlushPartialPins(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfPinningUnsupported(t)
		skipIfCgroupMemoryStatsUnsupported(t)
		cfg := testConfig(withPersistMonitoring(true), withCgroupMemoryStats(true))

		// Without the memcg flush links, new ones are attached on the pinned memcg_flush_seq. The
		// pinned oom-kill program checks the cgroup memory stats against that same counter, so it is
		// taken over.
		t.Run("memcg flush links missing", func(t *testing.T) {
			setTestPinDir(t)
			firstLinkID, firstFlushLinkIDs := crashPinnedMemcgFlushProbe(t, cfg)
			removePin(linkPinPath(memcgFlushBeginLinkName))
			removePin(linkPinPath(memcgFlushEndLinkName))

			second, err := NewProbe(cfg)
			require.NoError(t, err)
			t.Cleanup(second.Close)
			assert.Equal(t, firstLinkID, linkID(t, second.link), "second probe should take over the pinned oom-kill link")
			assertNoneEqual(t, firstFlushLinkIDs, memcgFlushLinkIDs(t, second), "second probe should attach new memcg flush links")
			assertPins(t, "the oom-kill probe should be pinned again")
			assertMemcgFlushPins(t, "the new memcg flush probes should be pinned")
			assertMemcgFlushCounting(t, "new memcg flush probes should count the flushes on the pinned counter")
		})

		// Without memcg_flush_seq, a new counter is created. Neither the pinned memcg flush programs
		// nor the pinned oom-kill program use it, so none of them are taken over.
		t.Run("memcg flush counter missing", func(t *testing.T) {
			setTestPinDir(t)
			firstLinkID, firstFlushLinkIDs := crashPinnedMemcgFlushProbe(t, cfg)
			removePin(mapPinPath(memcgFlushSeqName))

			second, err := NewProbe(cfg)
			require.NoError(t, err)
			t.Cleanup(second.Close)
			assert.NotEqual(t, firstLinkID, linkID(t, second.link), "second probe should attach a new oom-kill link")
			assertNoneEqual(t, firstFlushLinkIDs, memcgFlushLinkIDs(t, second), "second probe should attach new memcg flush links")
			assertPins(t, "the new oom-kill probe should be pinned")
			assertMemcgFlushPins(t, "the new memcg flush probes should be pinned")
			assertMemcgFlushCounting(t, "new memcg flush probes should count the flushes on the new counter")
		})
	})
}

// TestOOMKillMemcgFlushStatsDisabled checks that a probe not collecting the cgroup memory stats
// replaces a pinned one that does, and removes the memcg flush pins.
func TestOOMKillMemcgFlushStatsDisabled(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfPinningUnsupported(t)
		skipIfCgroupMemoryStatsUnsupported(t)
		setTestPinDir(t)

		firstLinkID, firstFlushLinkIDs := crashPinnedMemcgFlushProbe(t, testConfig(withPersistMonitoring(true), withCgroupMemoryStats(true)))

		second, err := NewProbe(testConfig(withPersistMonitoring(true)))
		require.NoError(t, err)
		t.Cleanup(second.Close)
		// emit_cgroup_memory_stats is part of the program tag
		assert.NotEqual(t, firstLinkID, linkID(t, second.link), "second probe should attach a new oom-kill link")
		assert.Empty(t, second.flushLinks, "second probe should not attach memcg flush links")
		assertPins(t, "the new oom-kill probe should be pinned")
		assertNoMemcgFlushPins(t, "memcg flush pins should be removed")
		firstFlushLinkIDs[oomLinkName] = firstLinkID
		assertLinksDetached(t, firstFlushLinkIDs, "the pinned probes should be detached")
	})
}

// TestOOMKillMemcgFlushIncompatibleCounter checks that an incompatible memcg_flush_seq pin is
// replaced, and that the pinned probes using it are not taken over.
func TestOOMKillMemcgFlushIncompatibleCounter(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfPinningUnsupported(t)
		skipIfCgroupMemoryStatsUnsupported(t)
		setTestPinDir(t)

		cfg := testConfig(withPersistMonitoring(true), withCgroupMemoryStats(true))
		firstLinkID, firstFlushLinkIDs := crashPinnedMemcgFlushProbe(t, cfg)

		removePin(mapPinPath(memcgFlushSeqName))
		incompatible, err := bpflib.NewMap(&bpflib.MapSpec{Type: bpflib.Array, KeySize: 4, ValueSize: 4, MaxEntries: 1})
		require.NoError(t, err)
		require.NoError(t, incompatible.Pin(mapPinPath(memcgFlushSeqName)))
		incompatibleID := mapID(t, incompatible)
		require.NoError(t, incompatible.Close())

		second, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(second.Close)
		assert.NotEqual(t, firstLinkID, linkID(t, second.link), "second probe should attach a new oom-kill link")
		assertNoneEqual(t, firstFlushLinkIDs, memcgFlushLinkIDs(t, second), "second probe should attach new memcg flush links")
		assertPins(t, "the new oom-kill probe should be pinned")
		assertMemcgFlushPins(t, "the new memcg flush probes should be pinned")

		seq, err := bpflib.LoadPinnedMap(mapPinPath(memcgFlushSeqName), nil)
		require.NoError(t, err)
		defer seq.Close()
		assert.NotEqual(t, incompatibleID, mapID(t, seq), "the incompatible memcg_flush_seq should be replaced")
		assertMemcgFlushCounting(t, "new memcg flush probes should count the flushes on the new counter")
	})
}

// TestOOMKillMemcgFlushClose checks that a graceful close removes every pin, and detaches the
// oom-kill and memcg flush probes.
func TestOOMKillMemcgFlushClose(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfPinningUnsupported(t)
		skipIfCgroupMemoryStatsUnsupported(t)
		setTestPinDir(t)

		p, err := NewProbe(testConfig(withPersistMonitoring(true), withCgroupMemoryStats(true)))
		require.NoError(t, err)
		require.True(t, p.pinned, "probe should be pinned when pinning is supported")
		assertPins(t, "the oom-kill probe should be pinned")
		assertMemcgFlushPins(t, "the memcg flush probes should be pinned")
		linkIDs := memcgFlushLinkIDs(t, p)
		linkIDs[oomLinkName] = linkID(t, p.link)

		p.Close()
		assertNoPins(t, "graceful close should remove the pins")
		assertLinksDetached(t, linkIDs, "graceful close should detach the probes")
	})
}

// crashPinnedMemcgFlushProbe starts a pinned probe collecting the cgroup memory stats, then
// simulates system-probe dying without a graceful shutdown: the file descriptors are released but
// the pins are left in place, so the probes keep running. It returns the IDs of the oom-kill link
// and of the memcg flush links.
func crashPinnedMemcgFlushProbe(t *testing.T, cfg *Config) (link.ID, map[string]link.ID) {
	first, err := NewProbe(cfg)
	require.NoError(t, err)
	t.Cleanup(first.Close)
	require.True(t, first.pinned, "first probe should be pinned when pinning is supported")
	assertPins(t, "first probe should pin the oom-kill probe")
	assertMemcgFlushPins(t, "first probe should pin the memcg flush probes")
	oomLinkID := linkID(t, first.link)
	flushLinkIDs := memcgFlushLinkIDs(t, first)

	require.NoError(t, first.link.Close())
	first.closeMemcgFlushLinks()
	first.coll.Close()
	return oomLinkID, flushLinkIDs
}

// assertMemcgFlushCounting checks that the pinned memcg_flush_seq changes, which the memcg stats
// flushes do at least every 2s
func assertMemcgFlushCounting(t *testing.T, msg string) {
	seq, err := bpflib.LoadPinnedMap(mapPinPath(memcgFlushSeqName), nil)
	require.NoError(t, err)
	defer seq.Close()

	before, err := memcgFlushSeq(seq)
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		cur, err := memcgFlushSeq(seq)
		return err == nil && cur != before
	}, 10*time.Second, 100*time.Millisecond, msg)
}

// assertLinksDetached checks that the links of ids are released by the kernel
func assertLinksDetached(t *testing.T, ids map[string]link.ID, msg string) {
	for name, id := range ids {
		assert.Eventually(t, func() bool {
			l, err := link.NewFromID(id)
			if err != nil {
				return true
			}
			_ = l.Close()
			return false
		}, 5*time.Second, 100*time.Millisecond, "%s: %s", name, msg)
	}
}

func mapID(t *testing.T, m *bpflib.Map) bpflib.MapID {
	info, err := m.Info()
	require.NoError(t, err)
	id, ok := info.ID()
	require.True(t, ok, "map ID not available")
	return id
}

// assertNoneEqual checks that no link of got has the ID of the link of the same name in want
func assertNoneEqual(t *testing.T, want, got map[string]link.ID, msg string) {
	for name, id := range got {
		assert.NotEqual(t, want[name], id, "%s: %s", name, msg)
	}
}

// memcgFlushLinkIDs returns the IDs of the memcg flush links of p, keyed by their pin name
func memcgFlushLinkIDs(t *testing.T, p *Probe) map[string]link.ID {
	ids := make(map[string]link.ID)
	for name, l := range p.flushLinks {
		ids[name] = linkID(t, l)
	}
	require.Len(t, ids, 2, "both memcg flush links should be attached")
	return ids
}

func memcgFlushSeq(m *bpflib.Map) (uint64, error) {
	var seq uint64
	err := m.Lookup(uint32(0), &seq)
	return seq, err
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
	spec := loadOOMCollectionSpec(t)
	mapSpec, ok := spec.Maps[oomMapName]
	require.True(t, ok, "map %s not found", oomMapName)
	return mapSpec
}

func loadOOMCollectionSpec(t *testing.T) *bpflib.CollectionSpec {
	cfg := testConfig()
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
	return spec
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

// skipIfCgroupMemoryStatsUnsupported skips the test unless the cgroup memory stats are collected
// on the running kernel, which requires tracking the memcg stats flushes
func skipIfCgroupMemoryStatsUnsupported(t *testing.T) {
	if err := cgroupMemoryStatsSupported(); err != nil {
		t.Skipf("cgroup memory stats are not supported: %s", err)
	}
	if err := memcgFlushTrackingSupported(loadOOMCollectionSpec(t)); err != nil {
		t.Skipf("memcg stats flushes cannot be tracked: %s", err)
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

// oomKillPins returns the pins of the oom-kill probe
func oomKillPins() []string {
	return []string{linkPinPath(oomLinkName), mapPinPath(oomMapName)}
}

// memcgFlushPins returns the pins added when the oom-kill probe collects the cgroup memory stats
func memcgFlushPins() []string {
	return []string{
		linkPinPath(memcgFlushBeginLinkName),
		linkPinPath(memcgFlushEndLinkName),
		mapPinPath(memcgFlushSeqName),
	}
}

func assertPinned(t *testing.T, paths []string, msg string) {
	for _, path := range paths {
		_, err := os.Stat(path)
		assert.NoError(t, err, "pin %s: %s", path, msg)
	}
}

func assertNotPinned(t *testing.T, paths []string, msg string) {
	for _, path := range paths {
		_, err := os.Stat(path)
		assert.ErrorIs(t, err, os.ErrNotExist, "pin %s: %s", path, msg)
	}
}

func assertPins(t *testing.T, msg string) {
	assertPinned(t, oomKillPins(), msg)
}

func assertMemcgFlushPins(t *testing.T, msg string) {
	assertPinned(t, memcgFlushPins(), msg)
}

func assertNoMemcgFlushPins(t *testing.T, msg string) {
	assertNotPinned(t, memcgFlushPins(), msg)
}

// assertNoPins checks that none of the pins of the oom-kill probe are left, including the ones added
// when it collects the cgroup memory stats
func assertNoPins(t *testing.T, msg string) {
	assertNotPinned(t, append(oomKillPins(), memcgFlushPins()...), msg)
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

// testConfigOpt modifies the config returned by testConfig
type testConfigOpt func(*Config)

func withPersistMonitoring(enabled bool) testConfigOpt {
	return func(cfg *Config) { cfg.PersistMonitoring = enabled }
}

func withCgroupMemoryStats(enabled bool) testConfigOpt {
	return func(cfg *Config) { cfg.EmitCgroupMemoryStats = enabled }
}

func testConfig(opts ...testConfigOpt) *Config {
	cfg := NewConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
