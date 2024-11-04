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
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// nodeJSMonitor essentially scans for Node processes and attaches SSL uprobes
// to them.
type nodeJSMonitor struct {
	attacher       *uprobes.UprobeAttacher
	processMonitor *monitor.ProcessMonitor
}

func newNodeJSMonitor(c *config.Config, mgr *manager.Manager) (*nodeJSMonitor, error) {
	if !c.EnableNodeJSMonitoring {
		return nil, nil
	}

	attachCfg := uprobes.AttacherConfig{
		ProcRoot: kernel.ProcFSRoot(),
		Rules: []*uprobes.AttachRule{{
			Targets:          uprobes.AttachToExecutable,
			ProbesSelector:   nodeJSProbes,
			ExecutableFilter: isNodeJSBinary,
		}},
		EbpfConfig:                     &c.Config,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		EnablePeriodicScanNewProcesses: true,
	}

	procMon := monitor.GetProcessMonitor()
	attacher, err := uprobes.NewUprobeAttacher(nodeJsAttacherName, attachCfg, mgr, uprobes.NopOnAttachCallback, &uprobes.NativeBinaryInspector{}, procMon)
	if err != nil {
		return nil, fmt.Errorf("cannot create uprobe attacher: %w", err)
	}

	return &nodeJSMonitor{
		attacher:       attacher,
		processMonitor: procMon,
	}, nil
}

// Start the nodeJSMonitor
func (m *nodeJSMonitor) Start() {
	if m == nil {
		return
	}

	m.attacher.Start()
	log.Info("Node JS TLS monitoring enabled")
}

// Stop the nodeJSMonitor.
func (m *nodeJSMonitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()
	m.attacher.Stop()
}

// getNodeJSPath checks if the given PID is a NodeJS process and returns the path to the binary
func isNodeJSBinary(path string, procInfo *uprobes.ProcInfo) bool {
	exe, err := procInfo.Exe()
	if err != nil {
		return false
	}
	return strings.Contains(exe, nodeJSPath)
}
