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
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type monitorState = string

const (
	disabled   monitorState = "disabled"
	running    monitorState = "running"
	notRunning monitorState = "Not running"
)

var (
	state        = disabled
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

func findScopeFiles(root string) ([]string, error) {
	var scopeFiles []string

	// Walk through the directory tree starting from the root path
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if the path matches the pattern "*.scope"
		matched, err := filepath.Match("*.scope", info.Name())
		if err != nil {
			return err
		}
		// If the path matches, add it to the scopeFiles slice
		if matched {
			scopeFiles = append(scopeFiles, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return scopeFiles, nil
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, connectionProtocolMap *ebpf.Map) (m *Monitor, err error) {
	defer func() {
		// capture error and wrap it
		if err != nil {
			state = notRunning
			err = fmt.Errorf("could not initialize USM: %w", err)
			startupError = err
		}
	}()

	mgr, err := newEBPFProgram(c, connectionProtocolMap)
	if err != nil {
		return nil, fmt.Errorf("error setting up ebpf program: %w", err)
	}

	if len(mgr.enabledProtocols) == 0 {
		state = disabled
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
	ebpfcheck.AddNameMappings(mgr.Manager.Manager, "usm_monitor")

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling traffic inspection: %s", err)
	}

	sockmap, found, _ := mgr.GetMap("sockhash")
	if found {
		fmt.Println("sockhash", sockmap)

		probe, found := mgr.GetProbe(manager.ProbeIdentificationPair{
			EBPFFuncName: kafkaStreamVerdict,
			UID:          probeUID,
		})
		if found {
			probe.SockMap = sockmap
		}

		// probe, found := mgr.GetProbe(manager.ProbeIdentificationPair{
		// 	EBPFFuncName: kafkaStreamParser,
		// 	UID:          probeUID,
		// })
		// if found {
		// 	probe.SockMap = sockmap
		// }
	} else {
		fmt.Println("no sockhash")
	}

	// sockmap, found, _ := mgr.GetMap("sockhash")
	// if found {
	// 	probe := &manager.Probe{
	// 		ProbeIdentificationPair: manager.ProbeIdentificationPair{
	// 			EBPFFuncName: "sk_skb__kafka_stream_parser",
	// 		},
	// 		SockMap: sockmap,
	// 	}
	// 	if err := mgr.AddHook("", probe); err != nil {
	// 		log.Errorf("error adding hook: %s", err)
	// 	}
	// 	fmt.Println("sockhash", sockmap)
	// } else {
	// 	fmt.Println("no sockhash")
	// }

	cgroupList, err := findScopeFiles("/sys/fs/cgroup")
	if err != nil {
		return nil, fmt.Errorf("error finding cgroup scope files: %s", err)
	}

	sockops, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFFuncName: sockopsFunction, UID: probeUID})
	sockops.CGroupPath = "/sys/fs/cgroup"
	for _, cgroup := range cgroupList {
		uid, _ := utils.NewPathIdentifier(cgroup)
		probe := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sockopsFunction,
				UID:          uid.Key()[:10],
			},
			CGroupPath: cgroup,
		}
		fmt.Println(cgroup)
		if err := mgr.AddHook("", probe); err != nil {
			log.Errorf("error adding hook: %s", err)
		}
	}

	processMonitor := monitor.GetProcessMonitor()

	state = running

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
	if m.cfg.EnableNativeTLSMonitoring || m.cfg.EnableGoTLSSupport || m.cfg.EnableJavaTLSSupport || m.cfg.EnableIstioMonitoring || m.cfg.EnableNodeJSMonitoring {
		err = m.processMonitor.Initialize()
	}

	return err
}

// GetUSMStats returns the current state of the USM monitor
func (m *Monitor) GetUSMStats() map[string]interface{} {
	response := map[string]interface{}{
		"state": state,
	}

	if startupError != nil {
		response["error"] = startupError.Error()
	}

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

	ebpfcheck.RemoveNameMappings(m.ebpfProgram.Manager.Manager)

	m.ebpfProgram.Close()
	m.closeFilterFn()
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(w io.Writer, maps ...string) error {
	return m.ebpfProgram.DumpMaps(w, maps...)
}
