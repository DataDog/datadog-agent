// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"fmt"
	"os"
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

// envoyCmd represents the search term used for determining
// whether or not a given PID represents an Envoy process.
// The search is done over the /proc/<pid>/cmdline file.
var envoyCmd = []byte("/bin/envoy")

// readBufferPool is used for reading /proc/<pid>/cmdline files.
// We use a pointer to a slice to avoid allocations when casting
// values to the empty interface during Put() calls.
var readBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 128)
		return &b
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

	// `utils.FileRegistry` callbacks
	registerCB   func(utils.FilePath) error
	unregisterCB func(utils.FilePath) error

	// Termination
	wg   sync.WaitGroup
	done chan struct{}
}

func newIstioMonitor(c *config.Config, mgr *manager.Manager) *istioMonitor {
	if !c.EnableIstioMonitoring {
		return nil
	}

	procRoot := kernel.ProcFSRoot()
	return &istioMonitor{
		registry: utils.NewFileRegistry("istio"),
		procRoot: procRoot,
		done:     make(chan struct{}),

		// Callbacks
		registerCB:   addHooks(mgr, procRoot, istioProbes),
		unregisterCB: removeHooks(mgr, istioProbes),
	}
}

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

	log.Debug("Istio monitoring enabled")
}

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
		m.handleProcessExec(uint32(pid))
		return nil
	})

	// At this point all entries from deletionCandidates are no longer alive, so
	// we should dettach our SSL probes from them
	for pid := range deletionCandidates {
		m.handleProcessExit(pid)
	}
}

func (m *istioMonitor) handleProcessExec(pid uint32) {
	path := m.getEnvoyPath(pid)
	if path == "" {
		return
	}

	m.registry.Register(
		path,
		pid,
		m.registerCB,
		m.unregisterCB,
	)
}

func (m *istioMonitor) handleProcessExit(pid uint32) {
	// We avoid filtering PIDs here because it's cheaper to simply do a registry lookup
	// instead of fetching a process name in order to determine whether it is an
	// envoy process or not (which at the very minimum involves syscalls)
	m.registry.Unregister(pid)
}

// getEnvoyPath returns the executable path of the envoy binary for a given PID.
// In case the PID doesn't represent an envoy process, an empty string is returned.
//
// TODO:
// refine process detection heuristic so we can remove the number of false
// positives. A common case that is likely worth optimizing for is filtering
// out "vanilla" envoy processes, and selecting only envoy processes that are
// running inside istio containers. Based on a quick inspection I made, it
// seems that we could also search for "istio" in the cmdline string in addition
// to "envoy", since the command line arguments look more or less the following:
//
// /usr/local/bin/envoy -cetc/istio/proxy/envoy-rev.json ...
func (m *istioMonitor) getEnvoyPath(pid uint32) string {
	cmdlinePath := fmt.Sprintf("%s/%d/cmdline", m.procRoot, pid)

	f, err := os.Open(cmdlinePath)
	if err != nil {
		// This can happen often in the context of ephemeral processes
		return ""
	}
	defer f.Close()

	// From here on we shouldn't allocate for the common case
	// (eg., a process is *not* envoy)
	bufferPtr := readBufferPool.Get().(*[]byte)
	defer func() {
		readBufferPool.Put(bufferPtr)
	}()

	buffer := *bufferPtr
	n, _ := f.Read(buffer)
	if n == 0 {
		return ""
	}

	buffer = buffer[:n]
	i := bytes.Index(buffer, envoyCmd)
	if i < 0 {
		return ""
	}

	executable := buffer[:i+len(envoyCmd)]
	return string(executable)
}
