// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package process

import (
	"math"
	"os"
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"
)

const (
	eventsPerfMap = "events"

	// size of the channel containing event objects
	batchNotificationsChanSize = 100
	probeUID                   = "process"
)

var (
	syscalls = []struct {
		s       string
		enabled bool
	}{
		{"execve", true},
		{"execveat", true},
		{"fork", true},
		{"clone", true},
		{"clone3", true},
		{"vfork", true},
	}

	kprobes = []struct {
		f                      string
		enabled                bool
		minVersion, maxVersion kernel.Version
	}{
		{f: "kernel_clone", enabled: true, minVersion: kernel.VersionCode(5, 10, 0)},
		{f: "_do_fork", enabled: true, maxVersion: kernel.VersionCode(5, 8, 0)},
		{f: "do_exit", enabled: true},
		{f: "do_dentry_open", enabled: true},
	}

	tracepoints = []struct {
		section string
		f       string
		enabled bool
	}{
		{"tracepoint/sched/sched_process_fork", "sched_process_fork", true},
	}
)

type ebpfProgram struct {
	sync.Mutex
	mgr         *manager.Manager
	cfg         *config.Config
	bc          bytecode.AssetReader
	perfHandler *ddebpf.PerfHandler
	once        struct {
		start, stop sync.Once
	}
	lastErr error
	stopped bool

	handlers struct {
		exec func(*ebpf.ProcessExecEvent)
		exit func(*ebpf.ProcessExitEvent)
	}
}

func newEbpfProgram(c *config.Config, execHandler func(*ebpf.ProcessExecEvent), exitHandler func(*ebpf.ProcessExitEvent)) (*ebpfProgram, error) {
	bc, err := netebpf.ReadProcessModule(c.BPFDir, c.BPFDebug)
	if err != nil {
		return nil, err
	}

	handler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: eventsPerfMap},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        handler.DataHandler,
					LostHandler:        handler.LostHandler,
				},
			},
		},
	}

	for _, s := range syscalls {
		mgr.Probes = append(mgr.Probes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          probeUID,
				EBPFFuncName: "kprobe__sys_" + s.s,
				EBPFSection:  "kprobe/sys_" + s.s,
			},
			SyscallFuncName: s.s,
			Enabled:         s.enabled,
		})
	}

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}

	for _, k := range kprobes {
		if k.maxVersion == 0 {
			k.maxVersion = math.MaxUint32
		}

		mgr.Probes = append(mgr.Probes, &manager.Probe{
			ProbeIdentificationPair: kprobePair(k.f),
			Enabled:                 k.enabled && kv >= k.minVersion && kv <= k.maxVersion,
		})
	}

	for _, t := range tracepoints {
		mgr.Probes = append(mgr.Probes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          probeUID,
				EBPFFuncName: t.f,
				EBPFSection:  t.section,
			},
			KProbeMaxActive: int(512),
			Enabled:         t.enabled,
		})
	}

	p := &ebpfProgram{
		mgr:         mgr,
		cfg:         c,
		bc:          bc,
		perfHandler: handler,
	}

	p.handlers.exec = execHandler
	p.handlers.exit = exitHandler

	return p, nil
}

func (p *ebpfProgram) Init() error {
	p.Lock()
	defer p.Unlock()

	if p.lastErr != nil {
		return p.lastErr
	}

	defer p.bc.Close()

	kv, err := kernel.HostVersion()
	if p.lastErr = err; p.lastErr != nil {
		return p.lastErr
	}

	forkInput := uint64(0)
	if kv >= kernel.VersionCode(5, 3, 0) {
		forkInput = 1
	}

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ConstantEditors: []manager.ConstantEditor{
			{
				Name:  "do_fork_input",
				Value: forkInput,
			},
		},
	}

	for _, p := range p.mgr.Probes {
		if p.Enabled {
			options.ActivatedProbes = append(options.ActivatedProbes,
				&manager.ProbeSelector{
					ProbeIdentificationPair: p.ProbeIdentificationPair,
				},
			)
		}
	}

	p.lastErr = p.mgr.InitWithOptions(p.bc, options)
	return p.lastErr
}

func (p *ebpfProgram) Start() error {
	p.once.start.Do(func() {
		p.Lock()
		defer p.Unlock()

		if p.lastErr != nil {
			return
		}

		if p.lastErr = p.mgr.Start(); p.lastErr != nil {
			return
		}

		go func() {
			for {
				select {
				case event, ok := <-p.perfHandler.DataChannel:
					if !ok {
						return
					}

					ke := (*ebpf.ProcessKEvent)(unsafe.Pointer(&event.Data[0]))
					switch ebpf.ProcessEventType(ke.Type) {
					case ebpf.ProcessEventTypeFork, ebpf.ProcessEventTypeExec:
						if p.handlers.exec != nil {
							p.handlers.exec((*ebpf.ProcessExecEvent)(unsafe.Pointer(&event.Data[0])))
						}
					case ebpf.ProcessEventTypeExit:
						if p.handlers.exit != nil {
							p.handlers.exit((*ebpf.ProcessExitEvent)(unsafe.Pointer(&event.Data[0])))
						}
					}
				case _, ok := <-p.perfHandler.LostChannel:
					if !ok {
						return
					}
				}
			}
		}()
	})

	return p.lastErr
}

func (p *ebpfProgram) Stop() error {
	p.once.stop.Do(func() {
		p.Lock()
		defer p.Unlock()

		if p.lastErr != nil || p.stopped {
			return
		}

		p.perfHandler.Stop()

		if p.lastErr = p.mgr.Stop(manager.CleanAll); p.lastErr != nil {
			return
		}

		p.stopped = true
	})

	return p.lastErr
}

func kprobeSectionName(k string) string {
	return "kprobe/" + k
}

func kprobeFuncName(k string) string {
	return "kprobe__" + k
}

func kprobePair(k string) manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		UID:          probeUID,
		EBPFFuncName: kprobeFuncName(k),
		EBPFSection:  kprobeSectionName(k),
	}
}
