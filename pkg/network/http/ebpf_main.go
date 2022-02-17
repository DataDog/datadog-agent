// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"math"
	"os"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

const (
	httpInFlightMap          = "http_in_flight"
	httpBatchesMap           = "http_batches"
	httpBatchStateMap        = "http_batch_state"
	httpNotificationsPerfMap = "http_notifications"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to inspect plain HTTP traffic
	httpSocketFilter = "socket/http_filter"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128

	// size of the channel containing completed http_notification_objects
	batchNotificationsChanSize = 100
)

type ebpfProgram struct {
	*manager.Manager
	cfg         *config.Config
	bytecode    bytecode.AssetReader
	offsets     []manager.ConstantEditor
	subprograms []subprogram

	batchCompletionHandler *ddebpf.PerfHandler
}

type subprogram interface {
	ConfigureManager(*manager.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map) (*ebpfProgram, error) {
	var bytecode bytecode.AssetReader
	var err error
	if enableRuntimeCompilation(c) {
		bytecode, err = getRuntimeCompiledHTTP(c)
		if err != nil {
			if !c.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network http tracer: %s", err)
			}
			log.Warnf("error compiling network http tracer, falling back to pre-compiled: %s", err)
		}
	}

	if bytecode == nil {
		bytecode, err = netebpf.ReadHTTPModule(c.BPFDir, c.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
	}

	batchCompletionHandler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: httpInFlightMap},
			{Name: httpBatchesMap},
			{Name: httpBatchStateMap},
			{Name: sslSockByCtxMap},
			{Name: "ssl_read_args"},
			{Name: "bio_new_socket_args"},
			{Name: "fd_by_ssl_bio"},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: httpNotificationsPerfMap},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        batchCompletionHandler.DataHandler,
					LostHandler:        batchCompletionHandler.LostHandler,
				},
			},
		},
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.TCPSendMsgReturn), EBPFFuncName: "kretprobe__tcp_sendmsg"}, KProbeMaxActive: maxActive},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: httpSocketFilter, EBPFFuncName: "socket__http_filter"}},
		},
	}

	sslProgram, _ := newSSLProgram(c, sockFD)
	program := &ebpfProgram{
		Manager:                mgr,
		bytecode:               bytecode,
		cfg:                    c,
		offsets:                offsets,
		batchCompletionHandler: batchCompletionHandler,
		subprograms:            []subprogram{sslProgram},
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	defer e.bytecode.Close()

	for _, s := range e.subprograms {
		s.ConfigureManager(e.Manager)
	}
	e.Manager.DumpHandler = dumpMapsHandler

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			httpInFlightMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  httpSocketFilter,
					EBPFFuncName: "socket__http_filter",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.TCPSendMsgReturn),
					EBPFFuncName: "kretprobe__tcp_sendmsg",
				},
			},
		},
		ConstantEditors: e.offsets,
	}

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	err := e.InitWithOptions(e.bytecode, options)
	if err != nil {
		return err
	}

	return nil
}

func (e *ebpfProgram) Start() error {
	err := e.Manager.Start()
	if err != nil {
		return err
	}

	for _, s := range e.subprograms {
		s.Start()
	}

	return nil
}

func (e *ebpfProgram) Close() error {
	err := e.Manager.Stop(manager.CleanAll)
	e.batchCompletionHandler.Stop()
	for _, s := range e.subprograms {
		s.Stop()
	}
	return err
}

func enableRuntimeCompilation(c *config.Config) bool {
	if !c.EnableRuntimeCompiler {
		return false
	}

	// The runtime-compiled version of HTTP monitoring requires Kernel 4.6
	// because we use the `bpf_skb_load_bytes` helper.
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. falling back to pre-compiled program.")
		return false
	}

	return kversion >= kernel.VersionCode(4, 6, 0)
}
