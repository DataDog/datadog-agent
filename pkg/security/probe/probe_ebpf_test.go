// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package probe holds probe related files
package probe

import (
	"math"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	gopsutilprocess "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// selfExeInode returns the inode of the running test binary.
func selfExeInode(t *testing.T) uint64 {
	t.Helper()
	var st unix.Stat_t
	require.NoError(t, unix.Stat("/proc/self/exe", &st), "stat /proc/self/exe")
	return st.Ino
}

// selfStartTime returns the running test process's start time via gopsutil.
func selfStartTime(t *testing.T) time.Time {
	t.Helper()
	proc, err := gopsutilprocess.NewProcess(int32(os.Getpid()))
	require.NoError(t, err, "gopsutil.NewProcess(self)")
	ms, err := proc.CreateTime()
	require.NoError(t, err, "proc.CreateTime")
	return time.UnixMilli(ms)
}

func newTestEBPFProbe() *EBPFProbe {
	fieldHandlers := &EBPFFieldHandlers{}
	p := &EBPFProbe{
		fieldHandlers: fieldHandlers,
	}
	p.eventPool = ddsync.NewTypedPool(func() *model.Event {
		return &model.Event{}
	})
	return p
}

func TestDNSAnswerIPNet(t *testing.T) {
	t.Run("accept IPv4-mapped IPv6 AAAA answer", func(t *testing.T) {
		ipNet, ok := dnsAnswerIPNet(layers.DNSResourceRecord{
			Type: layers.DNSTypeAAAA,
			IP:   net.ParseIP("::ffff:192.0.2.1"),
		})

		assert.True(t, ok)
		assert.Equal(t, net.ParseIP("::ffff:192.0.2.1").To16(), ipNet.IP)
		assert.Equal(t, net.CIDRMask(128, 128), ipNet.Mask)
	})
}

func TestGetPoolEvent(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("get event from pool assigns field handlers", func(t *testing.T) {
		event := p.getPoolEvent()

		assert.NotNil(t, event)
		assert.Equal(t, p.fieldHandlers, event.FieldHandlers)
	})

	t.Run("get multiple events from pool", func(t *testing.T) {
		event1 := p.getPoolEvent()
		event2 := p.getPoolEvent()

		assert.NotNil(t, event1)
		assert.NotNil(t, event2)
		assert.Equal(t, p.fieldHandlers, event1.FieldHandlers)
		assert.Equal(t, p.fieldHandlers, event2.FieldHandlers)
	})
}

func TestPutBackPoolEvent(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("put back event with nil ProcessCacheEntry", func(t *testing.T) {
		event := p.getPoolEvent()
		event.ProcessCacheEntry = nil
		// Set some fields to verify they get reset
		event.Type = uint32(model.ExecEventType)
		event.TimestampRaw = 12345

		p.putBackPoolEvent(event)

		// Get a new event from the pool (should be the one we just put back)
		newEvent := p.getPoolEvent()

		// Verify that the event was reset (Type should be 0)
		assert.Equal(t, uint32(0), newEvent.Type)
		assert.Equal(t, uint64(0), newEvent.TimestampRaw)
	})

	t.Run("put back event preserves Os field after reset", func(t *testing.T) {
		event := p.getPoolEvent()
		event.ProcessCacheEntry = nil
		event.Type = uint32(model.ExecEventType)
		event.Os = "linux"

		p.putBackPoolEvent(event)

		// Get a new event from the pool (should be the one we just put back)
		newEvent := p.getPoolEvent()

		// Verify that the Os field is preserved after reset (should be "linux")
		assert.Equal(t, "linux", newEvent.Os)
	})
}

func TestGetAndPutBackPoolEventRoundTrip(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("round trip get and put back event", func(t *testing.T) {
		// Get an event
		event := p.getPoolEvent()
		assert.NotNil(t, event)
		assert.Equal(t, p.fieldHandlers, event.FieldHandlers)

		// Modify the event
		event.Type = uint32(model.ExecEventType)
		event.TimestampRaw = 99999

		// Put it back
		p.putBackPoolEvent(event)

		// Get another event (should be the same one, reset)
		event2 := p.getPoolEvent()
		assert.Equal(t, uint32(0), event2.Type)
		assert.Equal(t, uint64(0), event2.TimestampRaw)
		assert.Equal(t, p.fieldHandlers, event2.FieldHandlers)
	})
}

func TestSameProcessAsCached(t *testing.T) {
	selfInode := selfExeInode(t)
	selfStart := selfStartTime(t)
	selfPid := uint32(os.Getpid())

	t.Run("self pid with matching cached inode and forktime returns true", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.FileEvent.Inode = selfInode
		pr.ForkTime = selfStart
		assert.True(t, sameProcessAsCached(pr))
	})

	t.Run("zero forktime degrades to inode-only and returns true", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.FileEvent.Inode = selfInode
		assert.True(t, sameProcessAsCached(pr))
	})

	t.Run("self pid with mismatching cached inode returns false", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.FileEvent.Inode = 0xdeadbeef
		pr.ForkTime = selfStart
		assert.False(t, sameProcessAsCached(pr))
	})

	t.Run("self pid with mismatching cached forktime returns false", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.FileEvent.Inode = selfInode
		pr.ForkTime = selfStart.Add(-time.Hour)
		assert.False(t, sameProcessAsCached(pr))
	})

	t.Run("self pid with forktime just outside tolerance returns false", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.FileEvent.Inode = selfInode
		pr.ForkTime = selfStart.Add(startTimeTolerance + time.Second)
		assert.False(t, sameProcessAsCached(pr))
	})

	t.Run("zero cached inode returns false", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: selfPid}}
		pr.ForkTime = selfStart
		assert.False(t, sameProcessAsCached(pr))
	})

	t.Run("nonexistent pid returns false", func(t *testing.T) {
		pr := &model.Process{PIDContext: model.PIDContext{Pid: uint32(math.MaxInt32)}}
		pr.FileEvent.Inode = selfInode
		pr.ForkTime = selfStart
		assert.False(t, sameProcessAsCached(pr))
	})
}

func TestApplyFullProcessValuesArgs(t *testing.T) {
	pr := &model.Process{
		Argv0:         "/usr/bin/ls",
		Args:          "stale-args-cache",
		Argv:          []string{"stale", "argv", "cache"},
		ArgsTruncated: true,
		ArgsEntry: &model.ArgsEntry{
			Values:           []string{"/usr/bin/ls", "old"},
			Truncated:        true,
			ScrubbedResolved: true,
		},
	}
	originalEntry := pr.ArgsEntry

	full := []string{"/usr/bin/ls", "-al", "/very/long/path"}
	applyFullProcessValues(pr, full, enrichArgs)

	require.NotNil(t, pr.ArgsEntry)
	// fresh allocation so the entry's unexported scrubber cache doesn't leak.
	assert.NotSame(t, originalEntry, pr.ArgsEntry)
	assert.Equal(t, full, pr.ArgsEntry.Values)
	assert.False(t, pr.ArgsEntry.Truncated)
	assert.False(t, pr.ArgsEntry.ScrubbedResolved)
	assert.False(t, pr.ArgsTruncated)
	assert.Empty(t, pr.Args)
	assert.Nil(t, pr.Argv)
	assert.Equal(t, "/usr/bin/ls", pr.Argv0)
}

func TestApplyFullProcessValuesEnvs(t *testing.T) {
	pr := &model.Process{
		Envs:          []string{"stale-envs-cache"},
		Envp:          []string{"stale-envp-cache"},
		EnvsTruncated: true,
		EnvsEntry: &model.EnvsEntry{
			Values:    []string{"PATH=/old"},
			Truncated: true,
		},
	}
	originalEntry := pr.EnvsEntry

	full := []string{"PATH=/usr/bin", "HOME=/root", "LANG=C.UTF-8"}
	applyFullProcessValues(pr, full, enrichEnvs)

	require.NotNil(t, pr.EnvsEntry)
	assert.NotSame(t, originalEntry, pr.EnvsEntry)
	assert.Equal(t, full, pr.EnvsEntry.Values)
	assert.False(t, pr.EnvsEntry.Truncated)
	assert.False(t, pr.EnvsTruncated)
	assert.Nil(t, pr.Envs)
	assert.Nil(t, pr.Envp)
}

// TestEnrichRuleEventSelfPID is the happy path: cached inode matches
// /proc/self/exe, so both argv and envp are backfilled from /proc.
func TestEnrichRuleEventSelfPID(t *testing.T) {
	p := newTestEBPFProbe()

	require.NotEmpty(t, os.Args)
	selfInode := selfExeInode(t)
	selfStart := selfStartTime(t)

	ev := &model.Event{}
	ev.ProcessContext = &model.ProcessContext{
		Process: model.Process{
			PIDContext:    model.PIDContext{Pid: uint32(os.Getpid())},
			Argv0:         os.Args[0],
			ForkTime:      selfStart,
			ArgsTruncated: true,
			ArgsEntry: &model.ArgsEntry{
				Values:    []string{os.Args[0], "fake-truncat..."},
				Truncated: true,
			},
			EnvsTruncated: true,
			EnvsEntry: &model.EnvsEntry{
				Values:    []string{"FAKE_TRUNCATED_ENV=..."},
				Truncated: true,
			},
		},
	}
	ev.ProcessContext.Process.FileEvent.Inode = selfInode

	p.EnrichRuleEvent(ev)

	pr := &ev.ProcessContext.Process

	assert.False(t, pr.ArgsTruncated)
	require.NotNil(t, pr.ArgsEntry)
	assert.False(t, pr.ArgsEntry.Truncated)
	assert.Equal(t, os.Args, pr.ArgsEntry.Values)

	// envp: assert non-empty + a stable key rather than full equality to
	// avoid flakes from env mutation around the call.
	assert.False(t, pr.EnvsTruncated)
	require.NotNil(t, pr.EnvsEntry)
	assert.False(t, pr.EnvsEntry.Truncated)
	require.NotEmpty(t, pr.EnvsEntry.Values)

	hasPATH := false
	for _, e := range pr.EnvsEntry.Values {
		if strings.HasPrefix(e, "PATH=") {
			hasPATH = true
			break
		}
	}
	assert.True(t, hasPATH, "expected PATH= in enriched envp; got %d entries", len(pr.EnvsEntry.Values))
}

// TestEnrichRuleEventGonePID covers the "process has exited" path: stat
// on /proc/<pid>/exe fails, so the cached truncated values are kept.
func TestEnrichRuleEventGonePID(t *testing.T) {
	p := newTestEBPFProbe()

	const fakeArg = "fake-truncated-arg..."
	const fakeEnv = "FAKE_ENV=truncated..."

	ev := &model.Event{}
	ev.ProcessContext = &model.ProcessContext{
		Process: model.Process{
			PIDContext:    model.PIDContext{Pid: uint32(math.MaxInt32)},
			Comm:          "definitely-not-a-real-process",
			Argv0:         "/no/such/binary",
			ArgsTruncated: true,
			ArgsEntry:     &model.ArgsEntry{Values: []string{fakeArg}, Truncated: true},
			EnvsTruncated: true,
			EnvsEntry:     &model.EnvsEntry{Values: []string{fakeEnv}, Truncated: true},
		},
	}
	ev.ProcessContext.Process.FileEvent.Inode = 0xdeadbeef
	origArgsEntry := ev.ProcessContext.Process.ArgsEntry
	origEnvsEntry := ev.ProcessContext.Process.EnvsEntry

	p.EnrichRuleEvent(ev)

	pr := &ev.ProcessContext.Process
	assert.True(t, pr.ArgsTruncated)
	assert.True(t, pr.EnvsTruncated)
	assert.Same(t, origArgsEntry, pr.ArgsEntry)
	assert.Same(t, origEnvsEntry, pr.EnvsEntry)
	assert.Equal(t, []string{fakeArg}, pr.ArgsEntry.Values)
	assert.Equal(t, []string{fakeEnv}, pr.EnvsEntry.Values)
}

// TestEnrichRuleEventExeMismatch covers the PID-reuse path: /proc/<pid>/exe
// is readable but its inode no longer matches the cached one, so we refuse
// to enrich.
func TestEnrichRuleEventExeMismatch(t *testing.T) {
	p := newTestEBPFProbe()

	const fakeArg = "fake-truncated-arg..."
	const fakeEnv = "FAKE_ENV=truncated..."

	ev := &model.Event{}
	ev.ProcessContext = &model.ProcessContext{
		Process: model.Process{
			PIDContext:    model.PIDContext{Pid: uint32(os.Getpid())},
			Comm:          "totally-different-binary",
			Argv0:         "/nope/binary",
			ForkTime:      selfStartTime(t),
			ArgsTruncated: true,
			ArgsEntry:     &model.ArgsEntry{Values: []string{fakeArg}, Truncated: true},
			EnvsTruncated: true,
			EnvsEntry:     &model.EnvsEntry{Values: []string{fakeEnv}, Truncated: true},
		},
	}
	ev.ProcessContext.Process.FileEvent.Inode = 0xdeadbeef
	origArgsEntry := ev.ProcessContext.Process.ArgsEntry
	origEnvsEntry := ev.ProcessContext.Process.EnvsEntry

	p.EnrichRuleEvent(ev)

	pr := &ev.ProcessContext.Process
	assert.True(t, pr.ArgsTruncated)
	assert.True(t, pr.EnvsTruncated)
	assert.Same(t, origArgsEntry, pr.ArgsEntry)
	assert.Same(t, origEnvsEntry, pr.EnvsEntry)
	assert.Equal(t, []string{fakeArg}, pr.ArgsEntry.Values)
	assert.Equal(t, []string{fakeEnv}, pr.EnvsEntry.Values)
}

// TestEnrichRuleEventForkTimeMismatch covers PID reuse where the inode
// coincides (recycled PID re-execed the same binary): the forktime check
// rejects and we keep the cached truncated values.
func TestEnrichRuleEventForkTimeMismatch(t *testing.T) {
	p := newTestEBPFProbe()

	const fakeArg = "fake-truncated-arg..."
	const fakeEnv = "FAKE_ENV=truncated..."

	selfInode := selfExeInode(t)
	staleStart := selfStartTime(t).Add(-time.Hour)

	ev := &model.Event{}
	ev.ProcessContext = &model.ProcessContext{
		Process: model.Process{
			PIDContext:    model.PIDContext{Pid: uint32(os.Getpid())},
			Comm:          "stale-incarnation",
			Argv0:         os.Args[0],
			ForkTime:      staleStart,
			ArgsTruncated: true,
			ArgsEntry:     &model.ArgsEntry{Values: []string{fakeArg}, Truncated: true},
			EnvsTruncated: true,
			EnvsEntry:     &model.EnvsEntry{Values: []string{fakeEnv}, Truncated: true},
		},
	}
	ev.ProcessContext.Process.FileEvent.Inode = selfInode
	origArgsEntry := ev.ProcessContext.Process.ArgsEntry
	origEnvsEntry := ev.ProcessContext.Process.EnvsEntry

	p.EnrichRuleEvent(ev)

	pr := &ev.ProcessContext.Process
	assert.True(t, pr.ArgsTruncated)
	assert.True(t, pr.EnvsTruncated)
	assert.Same(t, origArgsEntry, pr.ArgsEntry)
	assert.Same(t, origEnvsEntry, pr.EnvsEntry)
	assert.Equal(t, []string{fakeArg}, pr.ArgsEntry.Values)
	assert.Equal(t, []string{fakeEnv}, pr.EnvsEntry.Values)
}
