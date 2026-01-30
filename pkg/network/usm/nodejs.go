// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	nodeJSPath = "/bin/node"

	nodejsSslReadRetprobe    = "nodejs_uretprobe__SSL_read"
	nodejsSslReadExRetprobe  = "nodejs_uretprobe__SSL_read_ex"
	nodejsSslWriteRetprobe   = "nodejs_uretprobe__SSL_write"
	nodejsSslWriteExRetprobe = "nodejs_uretprobe__SSL_write_ex"

	nodeJsAttacherName = "nodejs"
)

var (
	nodeJSProbes = []manager.ProbesSelector{
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
						EBPFFuncName: sslSetFDProbe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: bioNewSocketProbe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: bioNewSocketRetprobe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: sslReadProbe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: nodejsSslReadRetprobe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: nodejsSslReadExRetprobe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: sslWriteProbe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: nodejsSslWriteRetprobe,
					},
				},
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: nodejsSslWriteExRetprobe,
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
)

var nodejsSpec = &protocols.ProtocolSpec{
	Factory: newNodeJSMonitor,
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
				EBPFFuncName: sslSetFDProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: bioNewSocketProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: bioNewSocketRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: nodejsSslReadRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: nodejsSslReadExRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: nodejsSslWriteRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: nodejsSslWriteExRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslShutdownProbe,
			},
		},
	},
}

// nodeJSMonitor essentially scans for Node processes and attaches SSL uprobes
// to them.
type nodeJSMonitor struct {
	cfg            *config.Config
	attacher       *uprobes.UprobeAttacher
	processMonitor *monitor.ProcessMonitor
}

// Ensuring nodeJSMonitor implements the protocols.Protocol interface.
var _ protocols.Protocol = (*nodeJSMonitor)(nil)

func newNodeJSMonitor(mgr *manager.Manager, c *config.Config) (protocols.Protocol, error) {
	if !c.EnableNodeJSMonitoring || !usmconfig.TLSSupported(c) || !usmconfig.UretprobeSupported() {
		return nil, nil
	}

	attachCfg := uprobes.AttacherConfig{
		ProcRoot: kernel.ProcFSRoot(),
		Rules: []*uprobes.AttachRule{
			{
				// Statically linked Node.js (SSL symbols in node binary)
				Targets:          uprobes.AttachToExecutable,
				ProbesSelector:   nodeJSProbes,
				ExecutableFilter: isNodeJSBinary,
			},
			{
				// Dynamically linked Node.js (SSL symbols in libnode.so)
				Targets:          uprobes.AttachToSharedLibraries,
				ProbesSelector:   nodeJSProbes,
				LibraryNameRegex: regexp.MustCompile(`libnode\.so`),
			},
		},
		EbpfConfig:                     &c.Config,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		PerformInitialScan:             true,
		EnablePeriodicScanNewProcesses: true,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
	}

	procMon := monitor.GetProcessMonitor()
	attacher, err := uprobes.NewUprobeAttacher(consts.USMModuleName, nodeJsAttacherName, attachCfg, mgr, uprobes.NopOnAttachCallback, uprobes.AttacherDependencies{
		Inspector:      &uprobes.NativeBinaryInspector{},
		ProcessMonitor: procMon,
		Telemetry:      telemetry.GetCompatComponent(),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create uprobe attacher: %w", err)
	}

	return &nodeJSMonitor{
		cfg:            c,
		attacher:       attacher,
		processMonitor: procMon,
	}, nil
}

// ConfigureOptions changes map attributes to the given options.
func (m *nodeJSMonitor) ConfigureOptions(options *manager.Options) {
	sharedLibrariesConfigureOptions(options, m.cfg)
}

// PreStart is called before the start of the provided eBPF manager.
func (m *nodeJSMonitor) PreStart() error {
	if err := m.attacher.Start(); err != nil {
		return fmt.Errorf("cannot start nodeJS attacher: %w", err)
	}
	return nil
}

// PostStart is called after the start of the provided eBPF manager.
func (*nodeJSMonitor) PostStart() error {
	return nil
}

// Stop the nodeJSMonitor.
func (m *nodeJSMonitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()
	m.attacher.Stop()
}

// isNodeJSBinary returns true if the process is a NodeJS binary.
func isNodeJSBinary(_ string, procInfo *uprobes.ProcInfo) bool {
	exe, err := procInfo.Exe()
	if err != nil {
		return false
	}
	return strings.Contains(exe, nodeJSPath)
}

// DumpMaps is a no-op.
func (*nodeJSMonitor) DumpMaps(io.Writer, string, *ebpf.Map) {}

// Name return the program's name.
func (*nodeJSMonitor) Name() string {
	return nodeJsAttacherName
}

// GetStats is a no-op.
func (*nodeJSMonitor) GetStats() (*protocols.ProtocolStats, func()) {
	return nil, nil
}

// IsBuildModeSupported returns always true, as tls module is supported by all modes.
func (*nodeJSMonitor) IsBuildModeSupported(buildmode.Type) bool {
	return true
}
