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
	"time"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	offsetsDataMap            = "offsets_data"
	goTLSReadArgsMap          = "go_tls_read_args"
	goTLSWriteArgsMap         = "go_tls_write_args"
	connectionTupleByGoTLSMap = "conn_tup_by_go_tls_conn"
	goTLSConnByTupleMap       = "go_tls_conn_by_tuple"

	// The interval of the periodic scan for terminated processes. Increasing the interval, might cause larger spikes in cpu
	// and lowering it might cause constant cpu usage.
	scanTerminatedProcessesInterval = 30 * time.Second

	connReadProbe     = "uprobe__crypto_tls_Conn_Read"
	connReadRetProbe  = "uprobe__crypto_tls_Conn_Read__return"
	connWriteProbe    = "uprobe__crypto_tls_Conn_Write"
	connWriteRetProbe = "uprobe__crypto_tls_Conn_Write__return"
	connCloseProbe    = "uprobe__crypto_tls_Conn_Close"

	// GoTLSAttacherName holds the name used for the uprobe attacher of go-tls programs. Used for tests.
	GoTLSAttacherName = "go-tls"
)

type pid = uint32

// goTLSProgram contains implementation for go-TLS.
type goTLSProgram struct {
	attacher  *uprobes.UprobeAttacher
	inspector *goTLSBinaryInspector
	cfg       *config.Config
	procMon   *monitor.ProcessMonitor
	manager   *manager.Manager
}

var goTLSSpec = &protocols.ProtocolSpec{
	Factory: newGoTLS,
	Maps: []*manager.Map{
		{Name: offsetsDataMap},
		{Name: goTLSReadArgsMap},
		{Name: goTLSWriteArgsMap},
		{Name: connectionTupleByGoTLSMap},
		{Name: goTLSConnByTupleMap},
	},
	Probes: []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: connReadProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: connReadRetProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: connWriteProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: connWriteRetProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: connCloseProbe,
			},
		},
	},
}

func newGoTLS(mgr *manager.Manager, c *config.Config) (protocols.Protocol, error) {
	if !c.EnableGoTLSSupport {
		return nil, nil
	}

	if !usmconfig.TLSSupported(c) {
		log.Warn("goTLS not supported by this platform")
		return nil, nil
	}

	if !c.EnableRuntimeCompiler && !c.EnableCORE {
		log.Warn("goTLS support requires runtime-compilation or CO-RE to be enabled")
		return nil, nil
	}

	attacherCfg := uprobes.AttacherConfig{
		EbpfConfig: &c.Config,
		Rules: []*uprobes.AttachRule{{
			Targets: uprobes.AttachToExecutable,
			ProbesSelector: []manager.ProbesSelector{
				&manager.AllOf{
					Selectors: []manager.ProbesSelector{
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: connReadProbe}},
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: connReadRetProbe}},
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: connWriteProbe}},
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: connWriteRetProbe}},
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: connCloseProbe}},
					},
				},
			},
			ProbeOptionsOverride: map[string]uprobes.ProbeOptions{
				connReadProbe:     {IsManualReturn: false, Symbol: bininspect.ReadGoTLSFunc},
				connReadRetProbe:  {IsManualReturn: true, Symbol: bininspect.ReadGoTLSFunc},
				connWriteProbe:    {IsManualReturn: false, Symbol: bininspect.WriteGoTLSFunc},
				connWriteRetProbe: {IsManualReturn: true, Symbol: bininspect.WriteGoTLSFunc},
				connCloseProbe:    {IsManualReturn: false, Symbol: bininspect.CloseGoTLSFunc},
			},
		}},
		ExcludeTargets:                 uprobes.ExcludeInternal,
		PerformInitialScan:             false, // the process monitor will scan for new processes at startup
		EnablePeriodicScanNewProcesses: true,
		ScanProcessesInterval:          scanTerminatedProcessesInterval,
	}

	if c.GoTLSExcludeSelf {
		attacherCfg.ExcludeTargets |= uprobes.ExcludeSelf
	}

	inspector := &goTLSBinaryInspector{
		structFieldsLookupFunctions: structFieldsLookupFunctions,
		paramLookupFunctions:        paramLookupFunctions,
		binAnalysisMetric:           libtelemetry.NewCounter("usm.go_tls.analysis_time", libtelemetry.OptPrometheus),
		binNoSymbolsMetric:          libtelemetry.NewCounter("usm.go_tls.missing_symbols", libtelemetry.OptPrometheus),
	}

	procMon := monitor.GetProcessMonitor()
	attacher, err := uprobes.NewUprobeAttacher(consts.USMModuleName, GoTLSAttacherName, attacherCfg, mgr, uprobes.NopOnAttachCallback, inspector, procMon)
	if err != nil {
		return nil, fmt.Errorf("cannot create uprobe attacher: %w", err)
	}

	return &goTLSProgram{
		cfg:       c,
		inspector: inspector,
		attacher:  attacher,
		procMon:   procMon,
		manager:   mgr,
	}, nil
}

// Name return the program's name.
func (p *goTLSProgram) Name() string {
	return "go-tls"
}

// IsBuildModeSupported return true if the build mode is supported.
func (*goTLSProgram) IsBuildModeSupported(mode buildmode.Type) bool {
	return mode == buildmode.CORE || mode == buildmode.RuntimeCompiled
}

// ConfigureOptions changes map attributes to the given options.
func (p *goTLSProgram) ConfigureOptions(options *manager.Options) {
	options.MapSpecEditors[connectionTupleByGoTLSMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[goTLSConnByTupleMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
}

// PreStart launches the goTLS main goroutine to handle events.
func (p *goTLSProgram) PreStart() error {
	var err error

	p.inspector.offsetsDataMap, _, err = p.manager.GetMap(offsetsDataMap)
	if err != nil {
		return fmt.Errorf("could not get offsets_data map: %s", err)
	}

	err = p.attacher.Start()
	if err != nil {
		return fmt.Errorf("could not start attacher: %w", err)
	}

	return nil
}

// PostStart is a no-op
func (p *goTLSProgram) PostStart() error {
	return nil
}

// DumpMaps is a no-op.
func (p *goTLSProgram) DumpMaps(io.Writer, string, *ebpf.Map) {}

// GetStats is a no-op.
func (p *goTLSProgram) GetStats() (*protocols.ProtocolStats, func()) {
	return nil, nil
}

// Stop terminates the uprobe attacher for GoTLS programs.
func (p *goTLSProgram) Stop() {
	p.procMon.Stop()
	p.attacher.Stop()
}

// GoTLSAttachPID attaches Go TLS hooks on the binary of process with
// provided PID, if Go TLS is enabled.
func GoTLSAttachPID(pid pid) error {
	if goTLSSpec.Instance == nil {
		return errors.New("GoTLS is not enabled")
	}

	err := goTLSSpec.Instance.(*goTLSProgram).attacher.AttachPID(pid)
	if errors.Is(err, utils.ErrPathIsAlreadyRegistered) {
		// The process monitor has attached the process before us.
		return nil
	}

	return err
}

// GoTLSDetachPID detaches Go TLS hooks on the binary of process with
// provided PID, if Go TLS is enabled.
func GoTLSDetachPID(pid pid) error {
	if goTLSSpec.Instance == nil {
		return errors.New("GoTLS is not enabled")
	}

	return goTLSSpec.Instance.(*goTLSProgram).attacher.DetachPID(pid)
}
