// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"unsafe"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
)

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Consuming HTTP transaction "events" that are sent from Kernel space;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	consumer       *events.Consumer
	ebpfProgram    *ebpfProgram
	telemetry      *telemetry
	statkeeper     *httpStatKeeper
	processMonitor *monitor.ProcessMonitor

	// termination
	closeFilterFn func()
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*Monitor, error) {
	mgr, err := newEBPFProgram(c, offsets, sockFD, bpfTelemetry)
	if err != nil {
		return nil, fmt.Errorf("error setting up http ebpf program: %s", err)
	}

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing http ebpf program: %s", err)
	}

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFSection: protocolDispatcherSocketFilterSection, EBPFFuncName: protocolDispatcherSocketFilterFunction, UID: probeUID})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling HTTP traffic inspection: %s", err)
	}

	telemetry, err := newTelemetry()
	if err != nil {
		closeFilterFn()
		return nil, err
	}

	statkeeper := newHTTPStatkeeper(c, telemetry)
	processMonitor := monitor.GetProcessMonitor()

	return &Monitor{
		ebpfProgram:    mgr,
		telemetry:      telemetry,
		closeFilterFn:  closeFilterFn,
		statkeeper:     statkeeper,
		processMonitor: processMonitor,
	}, nil
}

// Start consuming HTTP events
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var err error
	m.consumer, err = events.NewConsumer(
		"http",
		m.ebpfProgram.Manager.Manager,
		m.process,
	)
	if err != nil {
		return err
	}
	m.consumer.Start()

	if err := m.ebpfProgram.Start(); err != nil {
		return err
	}

	return m.processMonitor.Initialize()
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *Monitor) GetHTTPStats() map[Key]*RequestStats {
	if m == nil {
		return nil
	}

	m.consumer.Sync()
	m.telemetry.log()
	return m.statkeeper.GetAndResetAllStats()
}

// Stop HTTP monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()
	m.ebpfProgram.Close()
	m.consumer.Stop()
	m.closeFilterFn()
}

func (m *Monitor) process(data []byte) {
	tx := (*ebpfHttpTx)(unsafe.Pointer(&data[0]))
	m.telemetry.count(tx)
	m.statkeeper.Process(tx)
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(maps ...string) (string, error) {
	return m.ebpfProgram.DumpMaps(maps...)
}
