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
	"strings"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
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

var istioSpec = &protocols.ProtocolSpec{
	Factory: newIstioMonitor,
	Maps:    sharedLibrariesMaps,
	Probes: []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__tcp_sendmsg",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslDoHandshakeProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslDoHandshakeRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslSetBioProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: istioSslReadRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: istioSslWriteRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslShutdownProbe,
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
	cfg            *config.Config
	attacher       *uprobes.UprobeAttacher
	envoyCmd       string
	processMonitor *monitor.ProcessMonitor
}

// Ensure istioMonitor implements the Protocol interface.
var _ protocols.Protocol = (*istioMonitor)(nil)

func newIstioMonitor(mgr *manager.Manager, c *config.Config) (protocols.Protocol, error) {
	if !c.EnableIstioMonitoring || !usmconfig.TLSSupported(c) || !usmconfig.UretprobeSupported() {
		return nil, nil
	}

	m := &istioMonitor{
		cfg:            c,
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

// ConfigureOptions changes map attributes to the given options.
func (m *istioMonitor) ConfigureOptions(options *manager.Options) {
	sharedLibrariesConfigureOptions(options, m.cfg)
}

// PreStart is called before the start of the provided eBPF manager.
func (m *istioMonitor) PreStart() error {
	if m.attacher == nil {
		return errors.New("istio monitoring is enabled but the attacher is nil")
	}

	if err := m.attacher.Start(); err != nil {
		return fmt.Errorf("cannot start istio attacher: %w", err)
	}

	return nil
}

// PostStart is called after the start of the provided eBPF manager.
func (*istioMonitor) PostStart() error {
	return nil
}

// Stop the istioMonitor.
func (m *istioMonitor) Stop() {
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

// DumpMaps is a no-op.
func (*istioMonitor) DumpMaps(io.Writer, string, *ebpf.Map) {}

// Name return the program's name.
func (*istioMonitor) Name() string {
	return istioAttacherName
}

// GetStats is a no-op.
func (*istioMonitor) GetStats() (*protocols.ProtocolStats, func()) {
	return nil, nil
}

// IsBuildModeSupported returns always true, as tls module is supported by all modes.
func (*istioMonitor) IsBuildModeSupported(buildmode.Type) bool {
	return true
}
