// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"strings"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	istioSslReadRetprobe  = "istio_uretprobe__SSL_read"
	istioSslWriteRetprobe = "istio_uretprobe__SSL_write"

	istioAttacherName = "istio"
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
	attacher       *uprobes.UprobeAttacher
	envoyCmd       string
	processMonitor *monitor.ProcessMonitor
}

func newIstioMonitor(c *config.Config, mgr *manager.Manager) (*istioMonitor, error) {
	if !c.EnableIstioMonitoring {
		return nil, nil
	}

	m := &istioMonitor{
		envoyCmd:       c.EnvoyPath,
		attacher:       nil,
		processMonitor: monitor.GetProcessMonitor(),
	}

	attachCfg := uprobes.AttacherConfig{
		ProcRoot: c.ProcRoot,
		Rules: []*uprobes.AttachRule{{
			Targets:          uprobes.AttachToExecutable,
			ProbesSelector:   istioProbes,
			ExecutableFilter: m.isIstioBinary,
		}},
		EbpfConfig:                     &c.Config,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		EnablePeriodicScanNewProcesses: true,
	}
	attacher, err := uprobes.NewUprobeAttacher(consts.USMModuleName, istioAttacherName, attachCfg, mgr, nil, &uprobes.NativeBinaryInspector{}, m.processMonitor)
	if err != nil {
		return nil, fmt.Errorf("Cannot create uprobe attacher: %w", err)
	}

	m.attacher = attacher

	return m, nil
}

// Start the istioMonitor
func (m *istioMonitor) Start() {
	if m == nil {
		return
	}

	if m.attacher == nil {
		log.Error("istio monitoring is enabled but the attacher is nil")
		return
	}

	if err := m.attacher.Start(); err != nil {
		log.Errorf("Cannot start istio attacher: %s", err)
	}

	log.Info("istio monitoring enabled")
}

// Stop the istioMonitor.
func (m *istioMonitor) Stop() {
	if m == nil {
		return
	}

	if m.attacher == nil {
		log.Error("istio monitoring is enabled but the attacher is nil")
		return
	}

	m.attacher.Stop()
}

// isIstioBinary checks whether the given file is an istioBinary, based on the expected envoy
// command substring (as defined by m.envoyCmd).
func (m *istioMonitor) isIstioBinary(path string, _ *uprobes.ProcInfo) bool {
	return strings.Contains(path, m.envoyCmd)
}
