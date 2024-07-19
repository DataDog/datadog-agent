// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"strings"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	attacher *uprobes.UprobeAttacher
	envoyCmd string
}

func newIstioMonitor(c *config.Config, mgr *manager.Manager) *istioMonitor {
	if !c.EnableIstioMonitoring {
		return nil
	}

	monitor := &istioMonitor{
		envoyCmd: c.EnvoyPath,
		attacher: nil,
	}

	attachCfg := uprobes.AttacherConfig{
		ProcRoot: kernel.ProcFSRoot(),
		Rules: []*uprobes.AttachRule{{
			Targets:          uprobes.AttachToExecutable,
			ProbesSelector:   nodeJSProbes,
			ExecutableFilter: monitor.isIstioBinary,
		}},
		EbpfConfig:     &c.Config,
		ExcludeTargets: uprobes.ExcludeSelf | uprobes.ExcludeInternal,
	}

	attacher, err := uprobes.NewUprobeAttacher("istio", &attachCfg, mgr, nil, &uprobes.NativeBinaryInspector{})
	if err != nil {
		log.Errorf("Cannot create uprobe attacher: %v", err)
	}

	monitor.attacher = attacher
	return monitor
}

// Start the istioMonitor
func (m *istioMonitor) Start() {
	if m == nil {
		return
	}

	m.attacher.Start()
	log.Info("Istio monitoring enabled")
}

// Stop the istioMonitor.
func (m *istioMonitor) Stop() {
	if m == nil {
		return
	}

	m.attacher.Stop()
}

// isIstioBinary checks whether the given file is an istioBinary, based on the expected envoy
// command substring (as defined by m.envoyCmd).
func (m *istioMonitor) isIstioBinary(path string, procInfo *uprobes.ProcInfo) bool {
	exec, err := procInfo.Exe()
	return err == nil && strings.Contains(exec, m.envoyCmd)
}
