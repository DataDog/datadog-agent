// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type fakeProcInfo struct {
	createdAt uint64
	exe       string
}

type recordedKill struct {
	pid int
	sig uint32
}

type recordedTracerKill struct {
	pid        uint32
	createTime uint64
}

type ebpflessKillTestHarness struct {
	probe  *EBPFLessProbe
	killer *ProcessKillerEBPFLess
	client *client

	mu          sync.Mutex
	procs       map[uint32]fakeProcInfo
	killed      []recordedKill
	tracerKills []recordedTracerKill
	tracerCh    chan recordedTracerKill
}

func newEBPFLessKillTestHarness(t *testing.T, selfNSID uint64, fallbackDelay time.Duration) *ebpflessKillTestHarness {
	t.Helper()

	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:           true,
			EnforcementRuleSourceAllowed: []string{"test"},
		},
	}

	resolver, err := process.NewEBPFLessResolver(nil, nil, nil, process.NewResolverOpts())
	require.NoError(t, err)

	r := &resolvers.EBPFLessResolvers{ProcessResolver: resolver}
	fh := &EBPFLessFieldHandlers{BaseFieldHandlers: &BaseFieldHandlers{}, resolvers: r}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		_ = c1.Close()
		_ = c2.Close()
	})

	p := &EBPFLessProbe{
		Resolvers:         r,
		fieldHandlers:     fh,
		event:             NewEBPFLessEvent(fh),
		probe:             &Probe{},
		config:            cfg,
		ctx:               ctx,
		cancelFnc:         cancel,
		clients:           make(map[net.Conn]*client),
		containerContexts: make(map[containerutils.ContainerID]*ebpfless.ContainerContext),
		fileHasher:        &FileHasher{},
	}

	h := &ebpflessKillTestHarness{
		probe:    p,
		procs:    make(map[uint32]fakeProcInfo),
		tracerCh: make(chan recordedTracerKill, 4),
	}

	k := NewProcessKillerEBPFLess(p)
	k.selfNSID = selfNSID
	k.selfPID = 9999
	k.fallbackDelay = fallbackDelay
	k.getProcInfoFn = func(pid uint32) (uint64, string, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		info, ok := h.procs[pid]
		if !ok {
			return 0, "", assert.AnError
		}
		return info.createdAt, info.exe, nil
	}
	k.killProcessFn = func(sig uint32, pc *killContext) error {
		h.mu.Lock()
		h.killed = append(h.killed, recordedKill{pid: pc.pid, sig: sig})
		h.mu.Unlock()
		return nil
	}
	k.killTracerFn = func(pid uint32, expectedCreateTime uint64) error {
		rec := recordedTracerKill{pid: pid, createTime: expectedCreateTime}
		h.mu.Lock()
		h.tracerKills = append(h.tracerKills, rec)
		h.mu.Unlock()
		h.tracerCh <- rec
		return nil
	}

	pk, err := NewProcessKiller(cfg, k)
	require.NoError(t, err)
	p.ebpfLessKiller = k
	p.processKiller = pk
	h.killer = k

	cl := &client{conn: c1, probe: p}
	p.clients[c1] = cl
	h.client = cl

	t.Cleanup(func() {
		_ = p.Close()
	})

	return h
}

func (h *ebpflessKillTestHarness) setProc(pid uint32, exe string, createdAt uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.procs[pid] = fakeProcInfo{createdAt: createdAt, exe: exe}
}

func (h *ebpflessKillTestHarness) killedPIDs() []recordedKill {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedKill, len(h.killed))
	copy(out, h.killed)
	return out
}

func (h *ebpflessKillTestHarness) tracerKilled() []recordedTracerKill {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedTracerKill, len(h.tracerKills))
	copy(out, h.tracerKills)
	return out
}

func makeEBPFLessKillRule(scope, signal string) (*rules.Rule, *rules.KillDefinition) {
	kd := &rules.KillDefinition{
		Scope:  scope,
		Signal: signal,
	}
	rule := &rules.Rule{
		PolicyRule: &rules.PolicyRule{
			Def: &rules.RuleDefinition{
				ID: "test_ebpfless_kill",
			},
			Policy: rules.PolicyInfo{
				Source: "test",
			},
			Actions: []*rules.Action{
				{
					Def: &rules.ActionDefinition{Kill: kd},
				},
			},
		},
		Rule: &eval.Rule{
			ID: "test_ebpfless_kill",
		},
	}
	return rule, kd
}

func TestProcessKillerLinuxNilCgroupResolverReturnsError(t *testing.T) {
	pkos := NewProcessKillerOS(nil, nil, false)
	ev := model.NewFakeEvent()
	entry := model.NewProcessCacheEntry()
	entry.ContainerContext.ContainerID = "ctr-1"
	entry.CGroup.CGroupID = "/ecs/task1/ctr-1"

	for _, scope := range []string{"container", "cgroup"} {
		t.Run(scope, func(t *testing.T) {
			pcs, err := pkos.getProcesses(scope, ev, entry)
			assert.Error(t, err)
			assert.Empty(t, pcs)
		})
	}
}

func TestProcessKillerEBPFLessContainerAndCGroupScope(t *testing.T) {
	for _, scope := range []string{"container", "cgroup"} {
		t.Run(scope, func(t *testing.T) {
			h := newEBPFLessKillTestHarness(t, 42, 200*time.Millisecond)

			// Register tracer (PID 100), entrypoint (PID 200), child (PID 201),
			// sibling container (PID 300), and agent selfPID (PID 9999).
			h.setProc(100, "/opt/datadog-agent/embedded/bin/cws-instrumentation", 1000)
			h.setProc(200, "/usr/bin/caddy", 2000)
			h.setProc(201, "/bin/sh", 3000)
			h.setProc(300, "/usr/bin/sibling", 4000)
			h.setProc(9999, "/opt/datadog-agent/embedded/bin/security-agent", 500)

			// Send Hello with tracer PID 100 in WrappedMode.
			h.probe.handleClientMsg(h.client, &ebpfless.Message{
				Type: ebpfless.MessageTypeHello,
				Hello: &ebpfless.HelloMsg{
					NSID:             42,
					PID:              100,
					Mode:             ebpfless.WrappedMode,
					CGroupID:         "/ecs/task1/ctr-1",
					ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
				},
			})

			// Simulate scanProcfs discovering the tracer itself in the same container/cgroup,
			// plus normal processes and a sibling container in the same PID namespace.
			h.probe.Resolvers.ProcessResolver.AddProcFSEntry(
				process.CacheResolverKey{Pid: 100, NSID: 42}, 1,
				"/opt/datadog-agent/embedded/bin/cws-instrumentation",
				[]string{"cws-instrumentation"}, false, nil, false,
				"ctr-1", "/ecs/task1/ctr-1", 1000, "",
			)
			h.probe.Resolvers.ProcessResolver.AddExecEntry(
				process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
				"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
				"ctr-1", "/ecs/task1/ctr-1", 2000, "",
			)
			child := h.probe.Resolvers.ProcessResolver.AddExecEntry(
				process.CacheResolverKey{Pid: 201, NSID: 42}, 200,
				"/bin/sh", []string{"sh"}, false, nil, false,
				"ctr-1", "/ecs/task1/ctr-1", 3000, "",
			)
			h.probe.Resolvers.ProcessResolver.AddExecEntry(
				process.CacheResolverKey{Pid: 300, NSID: 42}, 1,
				"/usr/bin/sibling", []string{"sibling"}, false, nil, false,
				"ctr-2", "/ecs/task1/ctr-2", 4000, "",
			)
			h.probe.Resolvers.ProcessResolver.AddProcFSEntry(
				process.CacheResolverKey{Pid: 9999, NSID: 42}, 1,
				"/opt/datadog-agent/embedded/bin/security-agent",
				[]string{"security-agent"}, false, nil, false,
				"ctr-agent", "/ecs/task1/ctr-agent", 500, "",
			)

			ev := model.NewFakeEvent()
			ev.ProcessCacheEntry = child
			ev.ProcessContext = &child.ProcessContext

			rule, kd := makeEBPFLessKillRule(scope, "SIGKILL")
			killed, report := h.probe.processKiller.KillAndReport(kd, rule, ev)
			require.True(t, killed)
			require.NotNil(t, report)
			assert.Equal(t, KillActionStatusPerformed, report.Status)
			assert.False(t, report.KilledAt.IsZero())

			// Only PIDs 200 and 201 must be killed (not tracer 100, not sibling 300, not self 9999).
			assert.ElementsMatch(t, []recordedKill{
				{pid: 200, sig: uint32(unix.SIGKILL)},
				{pid: 201, sig: uint32(unix.SIGKILL)},
			}, h.killedPIDs())

			// Simulate tracer exiting cleanly before fallback timeout -> timer cancelled, tracer NOT killed.
			h.probe.handleClientGoodbye(h.client)
			time.Sleep(250 * time.Millisecond)
			assert.Empty(t, h.tracerKilled())
		})
	}
}

func TestProcessKillerEBPFLessTracerFallbackFiresOnTimeout(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, 40*time.Millisecond)

	h.setProc(100, "/cws-instrumentation-volume/cws-instrumentation", 1000)
	h.setProc(200, "/usr/bin/caddy", 2000)

	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              100,
			Mode:             ebpfless.WrappedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})
	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("container", "SIGKILL")
	killed, _ := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.True(t, killed)

	// No Goodbye arrives: tracer fallback must fire with the tracer's PID and create time.
	select {
	case rec := <-h.tracerCh:
		assert.Equal(t, recordedTracerKill{pid: 100, createTime: 1000}, rec)
	case <-time.After(time.Second):
		t.Fatal("expected tracer fallback SIGKILL to fire")
	}
}

func TestProcessKillerEBPFLessTracerFallbackNotArmedForNonSIGKILL(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, 40*time.Millisecond)

	h.setProc(100, "/cws-instrumentation-volume/cws-instrumentation", 1000)
	h.setProc(200, "/usr/bin/caddy", 2000)

	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              100,
			Mode:             ebpfless.WrappedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})
	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("container", "SIGTERM")
	killed, _ := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.True(t, killed)
	assert.Equal(t, []recordedKill{{pid: 200, sig: uint32(unix.SIGTERM)}}, h.killedPIDs())

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, h.tracerKilled())
}

func TestProcessKillerEBPFLessTracerFallbackNotArmedForAttachedMode(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, 40*time.Millisecond)

	h.setProc(100, "/cws-instrumentation-volume/cws-instrumentation", 1000)
	h.setProc(200, "/usr/bin/caddy", 2000)

	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              100,
			Mode:             ebpfless.AttachedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})
	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 1,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("container", "SIGKILL")
	killed, _ := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.True(t, killed)

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, h.tracerKilled())
}

func TestProcessKillerEBPFLessDifferentPIDNamespaceRejected(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 99, 40*time.Millisecond)
	h.setProc(200, "/usr/bin/caddy", 2000)

	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)
	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	for _, scope := range []string{"process", "container", "cgroup"} {
		t.Run(scope, func(t *testing.T) {
			rule, kd := makeEBPFLessKillRule(scope, "SIGKILL")
			killed, report := h.probe.processKiller.KillAndReport(kd, rule, ev)
			assert.False(t, killed)
			assert.Nil(t, report)
			assert.Empty(t, h.killedPIDs())
		})
	}
}

func TestProcessKillerEBPFLessLegacyTracerPIDInferredFromPPID(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, 40*time.Millisecond)

	h.setProc(150, "/cws-instrumentation-volume/cws-instrumentation", 1500)
	h.setProc(200, "/usr/bin/caddy", 2000)

	// Older cws-instrumentation sends PID: 0 in HelloMsg.
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              0,
			Mode:             ebpfless.WrappedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})

	// First non-FromProcFS Exec carries PPID = tracer PID (150).
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:        ebpfless.SyscallTypeExec,
			PID:         200,
			Timestamp:   2000,
			ContainerID: "ctr-1",
			CGroupID:    "/ecs/task1/ctr-1",
			Exec: &ebpfless.ExecSyscallMsg{
				PPID: 150,
				File: ebpfless.FileSyscallMsg{Filename: "/usr/bin/caddy"},
				Args: []string{"caddy"},
			},
		},
	})

	entry := h.probe.Resolvers.ProcessResolver.Resolve(process.CacheResolverKey{Pid: 200, NSID: 42})
	require.NotNil(t, entry)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("cgroup", "SIGKILL")
	killed, _ := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.True(t, killed)
	assert.Equal(t, []recordedKill{{pid: 200, sig: uint32(unix.SIGKILL)}}, h.killedPIDs())

	select {
	case rec := <-h.tracerCh:
		assert.Equal(t, recordedTracerKill{pid: 150, createTime: 1500}, rec)
	case <-time.After(time.Second):
		t.Fatal("expected inferred tracer fallback SIGKILL to fire")
	}
}

func TestProcessKillerEBPFLessRacingForkAndExecKilled(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, time.Second)

	h.setProc(100, "/cws-instrumentation-volume/cws-instrumentation", 1000)
	h.setProc(200, "/usr/bin/caddy", 2000)
	h.setProc(202, "/bin/sh", 3000)
	h.setProc(203, "/usr/bin/curl", 4000)
	h.setProc(204, "/opt/datadog-agent/bin/agent/agent", 4500)
	h.setProc(301, "/usr/bin/sibling", 5000)

	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              100,
			Mode:             ebpfless.WrappedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})
	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("container", "SIGKILL")
	killed, _ := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.True(t, killed)

	// Racing fork in the same container -> must be killed immediately.
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:        ebpfless.SyscallTypeFork,
			PID:         202,
			Timestamp:   3000,
			ContainerID: "ctr-1",
			CGroupID:    "/ecs/task1/ctr-1",
			Fork:        &ebpfless.ForkSyscallMsg{PPID: 200},
		},
	})

	// Racing exec in the same container -> must be killed immediately.
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:        ebpfless.SyscallTypeExec,
			PID:         203,
			Timestamp:   4000,
			ContainerID: "ctr-1",
			CGroupID:    "/ecs/task1/ctr-1",
			Exec: &ebpfless.ExecSyscallMsg{
				PPID: 202,
				File: ebpfless.FileSyscallMsg{Filename: "/usr/bin/curl"},
			},
		},
	})

	// Excluded binary -> must NOT be killed.
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:        ebpfless.SyscallTypeExec,
			PID:         204,
			Timestamp:   4500,
			ContainerID: "ctr-1",
			CGroupID:    "/ecs/task1/ctr-1",
			Exec: &ebpfless.ExecSyscallMsg{
				PPID: 200,
				File: ebpfless.FileSyscallMsg{Filename: "/opt/datadog-agent/bin/agent/agent"},
			},
		},
	})

	// Procfs scan picking up a sibling container process in pidMode: task -> must NOT be killed.
	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:        ebpfless.SyscallTypeExec,
			PID:         301,
			Timestamp:   5000,
			ContainerID: "ctr-2",
			CGroupID:    "/ecs/task1/ctr-2",
			Exec: &ebpfless.ExecSyscallMsg{
				PPID:       1,
				FromProcFS: true,
				File:       ebpfless.FileSyscallMsg{Filename: "/usr/bin/sibling"},
			},
		},
	})

	assert.Equal(t, []recordedKill{
		{pid: 200, sig: uint32(unix.SIGKILL)},
		{pid: 202, sig: uint32(unix.SIGKILL)},
		{pid: 203, sig: uint32(unix.SIGKILL)},
	}, h.killedPIDs())
}

func TestProcessKillerEBPFLessGoodbyeFlushesPendingReports(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, time.Second)

	report := &KillActionReport{
		Signal:   "SIGKILL",
		Scope:    "container",
		Status:   KillActionStatusPerformed,
		KilledAt: time.Now().Add(-killActionFlushDelay - time.Second),
	}
	h.probe.processKiller.Lock()
	h.probe.processKiller.pendingReports = append(h.probe.processKiller.pendingReports, report)
	h.probe.processKiller.Unlock()

	h.probe.handleClientGoodbye(h.client)

	report.RLock()
	assert.True(t, report.resolved)
	report.RUnlock()
}

func TestProcessKillerEBPFLessWarmupDoesNotArmTracerFallbackEarly(t *testing.T) {
	h := newEBPFLessKillTestHarness(t, 42, 40*time.Millisecond)
	h.probe.config.RuntimeSecurity.EnforcementDisarmerContainerEnabled = true
	h.probe.config.RuntimeSecurity.EnforcementDisarmerContainerMaxAllowed = 2
	h.probe.config.RuntimeSecurity.EnforcementDisarmerContainerPeriod = 150 * time.Millisecond
	h.probe.processKiller.useDisarmers.Store(true)

	h.setProc(100, "/cws-instrumentation-volume/cws-instrumentation", 1000)
	h.setProc(200, "/usr/bin/caddy", 2000)

	h.probe.handleClientMsg(h.client, &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             42,
			PID:              100,
			Mode:             ebpfless.WrappedMode,
			CGroupID:         "/ecs/task1/ctr-1",
			ContainerContext: &ebpfless.ContainerContext{ID: "ctr-1"},
		},
	})
	entry := h.probe.Resolvers.ProcessResolver.AddExecEntry(
		process.CacheResolverKey{Pid: 200, NSID: 42}, 100,
		"/usr/bin/caddy", []string{"caddy"}, false, nil, false,
		"ctr-1", "/ecs/task1/ctr-1", 2000, "",
	)

	ev := model.NewFakeEvent()
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	rule, kd := makeEBPFLessKillRule("container", "SIGKILL")
	killed, report := h.probe.processKiller.KillAndReport(kd, rule, ev)
	require.False(t, killed)
	require.NotNil(t, report)
	assert.Equal(t, KillActionStatusQueued, report.Status)

	// During warmup, neither processes nor the tracer fallback should be triggered.
	time.Sleep(80 * time.Millisecond)
	assert.Empty(t, h.killedPIDs())
	assert.Empty(t, h.tracerKilled())

	// Once warmup expires and processPendingKills runs, PID 200 is killed and the fallback fires.
	time.Sleep(100 * time.Millisecond)
	h.probe.processKiller.processPendingKills()
	assert.Equal(t, []recordedKill{{pid: 200, sig: uint32(unix.SIGKILL)}}, h.killedPIDs())

	select {
	case rec := <-h.tracerCh:
		assert.Equal(t, recordedTracerKill{pid: 100, createTime: 1000}, rec)
	case <-time.After(time.Second):
		t.Fatal("expected tracer fallback SIGKILL after queued kill executed")
	}
}
