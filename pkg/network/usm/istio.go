// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	istioSslReadRetprobe  = "istio_uretprobe__SSL_read"
	istioSslWriteRetprobe = "istio_uretprobe__SSL_write"
)

var istioProbes = []manager.ProbesSelector{
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslDoHandshakeProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslDoHandshakeRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslSetBioProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslReadProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: istioSslReadRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslWriteProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: istioSslWriteRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslShutdownProbe,
				},
			},
		},
	},
}

// istioMonitor essentially scans for Envoy processes and attaches SSL uprobes
// to them.
//
// Note that for now we only support Istio as opposed to "vanilla" Envoy
// because the Envoy binary embedded in the Istio containers have debug symbols
// whereas the "vanilla" Envoy images are distributed without them.
type istioMonitor struct {
	registry *utils.FileRegistry
	procRoot string
	envoyCmd string

	// `utils.FileRegistry` callbacks
	registerCB   func(utils.FilePath) error
	unregisterCB func(utils.FilePath) error

	// Termination
	wg   sync.WaitGroup
	done chan struct{}
}

// Validate that istioMonitor implements the Attacher interface.
var _ utils.Attacher = &istioMonitor{}

func newIstioMonitor(c *config.Config, mgr *manager.Manager) *istioMonitor {
	if !c.EnableIstioMonitoring {
		return nil
	}

	procRoot := kernel.ProcFSRoot()
	return &istioMonitor{
		registry: utils.NewFileRegistry("istio"),
		procRoot: procRoot,
		envoyCmd: c.EnvoyPath,
		done:     make(chan struct{}),

		// Callbacks
		registerCB:   addHooks(mgr, procRoot, istioProbes),
		unregisterCB: removeHooks(mgr, istioProbes),
	}
}

// DetachPID detaches a given pid from the eBPF program
func (m *istioMonitor) DetachPID(pid uint32) error {
	return m.registry.Unregister(pid)
}

var (
	// ErrNoEnvoyPath is returned when no envoy path is found for a given PID
	ErrNoEnvoyPath = fmt.Errorf("no envoy path found for PID")
)

// AttachPID attaches a given pid to the eBPF program
func (m *istioMonitor) AttachPID(pid uint32) error {
	path := m.getEnvoyPath(pid)
	if path == "" {
		return ErrNoEnvoyPath
	}

	return m.registry.Register(
		path,
		pid,
		m.registerCB,
		m.unregisterCB,
	)
}

// Start the istioMonitor
func (m *istioMonitor) Start() {
	if m == nil {
		return
	}

	processMonitor := monitor.GetProcessMonitor()

	// Subscribe to process events
	doneExec := processMonitor.SubscribeExec(m.handleProcessExec)
	doneExit := processMonitor.SubscribeExit(m.handleProcessExit)

	// Attach to existing processes
	m.sync()

	m.wg.Add(1)
	go func() {
		// This ticker is responsible for controlling the rate at which
		// we scrape the whole procFS again in order to ensure that we
		// terminate any dangling uprobes and register new processes
		// missed by the process monitor stream
		processSync := time.NewTicker(scanTerminatedProcessesInterval)

		defer func() {
			processSync.Stop()
			// Execute process monitor callback termination functions
			doneExec()
			doneExit()
			// Stopping the process monitor (if we're the last instance)
			processMonitor.Stop()
			// Cleaning up all active hooks
			m.registry.Clear()
			// marking we're finished.
			m.wg.Done()
		}()

		for {
			select {
			case <-m.done:
				return
			case <-processSync.C:
				m.sync()
				m.registry.Log()
			}
		}
	}()

	utils.AddAttacher("istio", m)
	log.Info("Istio monitoring enabled")
}

// Stop the istioMonitor.
func (m *istioMonitor) Stop() {
	if m == nil {
		return
	}

	close(m.done)
	m.wg.Wait()
}

// sync state of istioMonitor with the current state of procFS
// the purpose of this method is two-fold:
// 1) register processes for which we missed exec events (targeted mostly at startup)
// 2) unregister processes for which we missed exit events
func (m *istioMonitor) sync() {
	deletionCandidates := m.registry.GetRegisteredProcesses()

	_ = kernel.WithAllProcs(m.procRoot, func(pid int) error {
		if _, ok := deletionCandidates[uint32(pid)]; ok {
			// We have previously hooked into this process and it remains active,
			// so we remove it from the deletionCandidates list, and move on to the next PID
			delete(deletionCandidates, uint32(pid))
			return nil
		}

		// This is a new PID so we attempt to attach SSL probes to it
		_ = m.AttachPID(uint32(pid))
		return nil
	})

	// At this point all entries from deletionCandidates are no longer alive, so
	// we should detach our SSL probes from them
	for pid := range deletionCandidates {
		m.handleProcessExit(pid)
	}
}

func (m *istioMonitor) handleProcessExit(pid uint32) {
	// We avoid filtering PIDs here because it's cheaper to simply do a registry lookup
	// instead of fetching a process name in order to determine whether it is an
	// envoy process or not (which at the very minimum involves syscalls)
	_ = m.DetachPID(pid)
}

func (m *istioMonitor) handleProcessExec(pid uint32) {
	_ = m.AttachPID(pid)
}

// getEnvoyPath returns the executable path of the envoy binary for a given PID.
// It constructs the path to the symbolic link for the executable file of the process with the given PID,
// then resolves this symlink to determine the actual path of the binary.
//
// If the resolved path contains the expected envoy command substring (as defined by m.envoyCmd),
// the function returns this path. If the PID does not correspond to an envoy process or if an error
// occurs during resolution, it returns an empty string.
func (m *istioMonitor) getEnvoyPath(pid uint32) string {
	exePath := fmt.Sprintf("%s/%d/exe", m.procRoot, pid)

	envoyPath, err := utils.ResolveSymlink(exePath)
	if err != nil || !strings.Contains(envoyPath, m.envoyCmd) {
		return ""
	}

	return envoyPath
}
