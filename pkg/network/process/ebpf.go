// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package process

import (
	"errors"
	"math"
	"os"
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
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
	syscalls = []string{
		"execve",
		"execveat",
		"fork",
		"clone",
		"vfork",
	}

	kprobes = []string{
		"do_exit",
		"do_dentry_open",
	}

	tracepoints = []struct {
		section string
		f       string
	}{
		{"tracepoint/sched/sched_process_fork", "sched_process_fork"},
	}

	ErrNotSupported = errors.New("not supported")

	kv *kernel.Version
)

func init() {
	kv, _ = kernel.NewKernelVersion()
}

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
	if !lruHashAvailable() {
		return nil, ErrNotSupported
	}

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
				EBPFFuncName: "kprobe__sys_" + s,
				EBPFSection:  "kprobe/sys_" + s,
			},
			SyscallFuncName: s,
			Enabled:         true,
		})
	}

	for _, k := range kprobes {
		mgr.Probes = append(mgr.Probes, &manager.Probe{
			ProbeIdentificationPair: kprobePair(k),
			Enabled:                 true,
		})
	}

	for _, t := range tracepoints {
		mgr.Probes = append(mgr.Probes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          probeUID,
				EBPFFuncName: t.f,
				EBPFSection:  t.section,
			},
			Enabled: true,
		})
	}

	// special cases
	specialProbes(mgr)

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

func lruHashAvailable() bool {
	return kv != nil && (kv.Code >= kernel.Kernel4_10 || kv.IsRH7Kernel())
}

func specialProbes(mgr *manager.Manager) {
	clone3(mgr)
	doForkOrKernelClone(mgr)
}

func clone3(mgr *manager.Manager) {
	// clone3 syscall
	if !clone3Available() {
		return
	}

	mgr.Probes = append(mgr.Probes, &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          probeUID,
			EBPFFuncName: "kprobe__sys_clone3",
			EBPFSection:  "kprobe/sys_clone3",
		},
		SyscallFuncName: "clone3",
		Enabled:         true,
	})
}

func doForkOrKernelClone(mgr *manager.Manager) {
	// either _do_fork or kernel_clone
	f := "kernel_clone"
	if useDoFork() {
		f = "_do_fork"
	}

	mgr.Probes = append(mgr.Probes, &manager.Probe{
		ProbeIdentificationPair: kprobePair(f),
		Enabled:                 true,
	})
}

func clone3Available() bool {
	return kv != nil && kv.Code >= kernel.Kernel5_3
}

func useDoFork() bool {
	return kv != nil && (kv.Code < kernel.Kernel5_10 || kv.IsRH7Kernel())
}

func (p *ebpfProgram) Init() error {
	p.Lock()
	defer p.Unlock()

	if p.lastErr != nil {
		return p.lastErr
	}

	defer p.bc.Close()

	forkInput := uint64(0)
	if kv.Code >= kernel.Kernel5_3 {
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
