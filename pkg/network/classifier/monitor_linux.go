// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package classifier

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

type classifier struct {
	p           *ebpfProgram
	closeFilter func()
}

// NewClassifier starts the packets classifier
func NewClassifier(cfg *config.Config, connMap *ebpf.Map, telemetryMap *ebpf.Map) (Classifier, error) {
	p, err := newEBPFProgram(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating ebpf program: %w", err)
	}

	if err := p.Init(connMap, telemetryMap); err != nil {
		return nil, fmt.Errorf("error initializing ebpf programs: %w", err)
	}

	filter, _ := p.GetProbe(manager.ProbeIdentificationPair{
		EBPFSection:  string(probes.SocketClassifierFilter),
		EBPFFuncName: "socket__classifier_filter",
	})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter %s", string(probes.SocketClassifierFilter))
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(cfg.ProcRoot, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling packets inspection: %s", err)
	}

	if err := p.Start(); err != nil {
		return nil, err
	}

	return &classifier{
		p,
		closeFilterFn,
	}, nil
}

// GetStats return classifier statistics
func (c *classifier) GetStats() map[string]int64 {
	mp, _, err := c.p.GetMap(string(probes.ClassifierTelemetryMap))
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return map[string]int64{}
	}

	var zero uint64
	telemetry := &netebpf.ClassifierTelemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		log.Tracef("error retrieving the telemetry struct: %s", err)
	}

	return map[string]int64{
		"tail_call_failed":    int64(telemetry.Tail_call_failed),
		"tls_flow_classified": int64(telemetry.Tls_flow_classified),
	}
}

// Close releases associated resources
func (c *classifier) Close() {
	_ = c.p.Stop(manager.CleanAll)
	c.closeFilter()
}

// DumpMaps return map info
func (c *classifier) DumpMaps(maps ...string) (string, error) {
	return c.p.DumpMaps(maps...)
}
