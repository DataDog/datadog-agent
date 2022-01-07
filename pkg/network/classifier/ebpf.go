// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package classifier

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

const (
	PROTO_PROG_TLS = 0 // protoProgsMap[0] pointing to socket/proto_tls tail_call
	protoProgsMap  = "proto_progs"
	tlsInFlightMap = "tls_in_flight"
	tlsProtoFilter = "socket/proto_tls"
)

type ebpfProgram struct {
	*manager.Manager
	cfg      *config.Config
	bytecode bytecode.AssetReader
}

func newEBPFProgram(c *config.Config) (*ebpfProgram, error) {
	bc, err := netebpf.ReadClassifierModule(c.BPFDir, c.BPFDebug)
	if err != nil {
		return nil, err
	}

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.TelemetryMap)}, // shared conn_stats_max_entries_hit
			{Name: string(probes.ConnMap)},
			{Name: string(probes.ClassifierTelemetryMap)},
			{Name: protoProgsMap},
			{Name: tlsInFlightMap},
		},
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SocketClassifierFilter), EBPFFuncName: "socket__classifier_filter"}},
		},
	}

	return &ebpfProgram{
		Manager:  mgr,
		bytecode: bc,
		cfg:      c,
	}, nil
}

func (e *ebpfProgram) Init(connMap *ebpf.Map, telemetryMap *ebpf.Map) error {
	defer e.bytecode.Close()

	e.Manager.DumpHandler = dumpMapsHandler

	return e.InitWithOptions(e.bytecode, manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapEditors: map[string]*ebpf.Map{
			string(probes.ConnMap):      connMap,
			string(probes.TelemetryMap): telemetryMap,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			tlsInFlightMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
		},
		TailCallRouter: []manager.TailCallRoute{
			{
				ProgArrayName: protoProgsMap,
				Key:           PROTO_PROG_TLS,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  tlsProtoFilter,
					EBPFFuncName: "socket__proto_tls",
				},
			},
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.SocketClassifierFilter),
					EBPFFuncName: "socket__classifier_filter",
				},
			},
		},
	})
}
