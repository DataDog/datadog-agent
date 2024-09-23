// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmstate "github.com/DataDog/datadog-agent/pkg/network/usm/state"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	startupError error
)

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Consuming HTTP transaction "events" that are sent from Kernel space;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	cfg *config.Config

	ebpfProgram *ebpfProgram

	processMonitor *monitor.ProcessMonitor

	// termination
	closeFilterFn func()

	lastUpdateTime *atomic.Int64
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, connectionProtocolMap *ebpf.Map) (m *Monitor, err error) {
	defer func() {
		// capture error and wrap it
		if err != nil {
			usmstate.Set(usmstate.NotRunning)
			err = fmt.Errorf("could not initialize USM: %w", err)
			startupError = err
		}
	}()

	mgr, err := newEBPFProgram(c, connectionProtocolMap)
	if err != nil {
		return nil, fmt.Errorf("error setting up ebpf program: %w", err)
	}

	if len(mgr.enabledProtocols) == 0 {
		usmstate.Set(usmstate.Disabled)
		log.Debug("not enabling USM as no protocols monitoring were enabled.")
		return nil, nil
	}

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing ebpf program: %w", err)
	}

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFFuncName: protocolDispatcherSocketFilterFunction, UID: probeUID})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}
	ddebpf.AddNameMappings(mgr.Manager.Manager, "usm_monitor")

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling traffic inspection: %s", err)
	}

	processMonitor := monitor.GetProcessMonitor()

	usmstate.Set(usmstate.Running)

	usmMonitor := &Monitor{
		cfg:            c,
		ebpfProgram:    mgr,
		closeFilterFn:  closeFilterFn,
		processMonitor: processMonitor,
	}

	usmMonitor.lastUpdateTime = atomic.NewInt64(time.Now().Unix())

	return usmMonitor, nil
}

// Start USM monitor.
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var err error

	defer func() {
		if err != nil {
			if errors.Is(err, syscall.ENOMEM) {
				err = fmt.Errorf("could not enable usm monitoring: not enough memory to attach http ebpf socket filter. please consider raising the limit via sysctl -w net.core.optmem_max=<LIMIT>")
			} else {
				err = fmt.Errorf("could not enable USM: %s", err)
			}

			m.Stop()

			startupError = err
		}
	}()

	err = m.ebpfProgram.Start()
	if err != nil {
		return err
	}

	// Need to explicitly save the error in `err` so the defer function could save the startup error.
	if usmconfig.NeedProcessMonitor(m.cfg) {
		err = m.processMonitor.Initialize(m.cfg.EnableUSMEventStream)
	}

	return err
}

// Pause bypasses the eBPF programs in the monitor
func (m *Monitor) Pause() error {
	if m == nil {
		return nil
	}

	return m.ebpfProgram.Pause()
}

// Resume enables the previously bypassed eBPF programs in the monitor
func (m *Monitor) Resume() error {
	if m == nil {
		return nil
	}

	return m.ebpfProgram.Resume()
}

// GetUSMStats returns the current state of the USM monitor
func (m *Monitor) GetUSMStats() map[string]interface{} {
	response := map[string]interface{}{
		"state": usmstate.Get(),
	}

	if startupError != nil {
		response["error"] = startupError.Error()
	}

	response["blocked_processes"] = utils.GetBlockedPathIDsList()

	tracedPrograms := utils.GetTracedProgramList()
	response["traced_programs"] = tracedPrograms

	if m != nil {
		response["last_check"] = m.lastUpdateTime
	}
	return response
}

// GetProtocolStats returns the current stats for all protocols
func (m *Monitor) GetProtocolStats() map[protocols.ProtocolType]interface{} {
	if m == nil {
		return nil
	}

	defer func() {
		// Update update time
		now := time.Now().Unix()
		m.lastUpdateTime.Swap(now)
		telemetry.ReportPrometheus()
	}()

	return m.ebpfProgram.getProtocolStats()
}

// Stop HTTP monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()

	ddebpf.RemoveNameMappings(m.ebpfProgram.Manager.Manager)

	m.ebpfProgram.Close()
	m.closeFilterFn()
	usmstate.Set(usmstate.Stopped)
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(w io.Writer, maps ...string) error {
	return m.ebpfProgram.DumpMaps(w, maps...)
}
