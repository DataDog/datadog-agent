// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ssluprobes

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ValidateSupported returns an error if TLS cert collection can't be supported
func ValidateSupported() error {
	if features.HaveBoundedLoops() != nil {
		return fmt.Errorf("TLS cert collection requires bounded loops (linux 5.4+)")
	}

	if features.HaveProgramType(ebpf.RawTracepoint) != nil {
		return fmt.Errorf("TLS cert collection requires raw tracepoints (linux 4.17+)")
	}

	if features.HaveMapType(ebpf.LRUHash) != nil {
		return fmt.Errorf("TLS cert collection requires LRU maps (linux 4.10+)")
	}

	// pass in EnableCORE: true so we're only checking kernel features. This is because
	// ConfigureOptions is called before we even know what tracer loaded successfully.
	// newEbpfTracer properly disables TLS cert collection on prebuilt
	if !sharedlibraries.IsSupported(&ddebpf.Config{EnableCORE: true}) {
		return fmt.Errorf("TLS cert collection requires shared library monitoring (kernel 4.14 on x86, 5.5 on arm64)")
	}

	hasUretprobeBug, err := kernelbugs.HasUretprobeSyscallSeccompBug()
	if err != nil {
		return fmt.Errorf("disabling TLS cert collection due to failed to check for uretprobe syscall seccomp bug: %v", err)
	}
	if hasUretprobeBug {
		return fmt.Errorf("disabling TLS cert collection due to kernel bug that causes segmentation faults with uretprobes and seccomp filters")
	}
	return nil
}

var openSSLProbes = []manager.ProbesSelector{
	&manager.BestEffort{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLReadExProbe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLReadExRetprobe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLWriteExProbe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLWriteExRetprobe),
			},
		},
	},
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLDoHandshakeProbe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLDoHandshakeRetprobe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLReadProbe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLReadRetprobe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLWriteProbe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.SSLWriteRetprobe),
			},
		},
	},
}

var cryptoProbes = []manager.ProbesSelector{
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.I2DX509Probe),
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: IDPairFromFuncName(probes.I2DX509Retprobe),
			},
		},
	},
}

// ConfigureOptions applies ssluprobes options to the ebpf manager
func ConfigureOptions(options *manager.Options, cfg *config.Config) error {
	if !cfg.EnableCertCollection {
		return nil
	}
	if err := ValidateSupported(); err != nil {
		return fmt.Errorf("TLS cert collection was requested even though it's not supported (shouldn't get here): %w", err)
	}
	options.MapSpecEditors[probes.SSLCertsStatemArgsMap] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections / 32,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[probes.SSLCertsI2DX509ArgsMap] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections / 32,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[probes.SSLHandshakeStateMap] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections / 32,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[probes.SSLCertInfoMap] = manager.MapSpecEditor{
		// now that we know LRU is supported, set its type to that
		Type:       ebpf.LRUHash,
		MaxEntries: cfg.MaxTrackedConnections / 32,
		EditorFlag: manager.EditMaxEntries,
	}

	schedExitIDPair := IDPairFromFuncName(probes.RawTracepointSchedProcessExit)
	options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: schedExitIDPair})

	return nil
}

// SSLCertsProgram handles attaching SSL uprobes
type SSLCertsProgram struct {
	cfg                      *config.Config
	ebpfManager              *manager.Manager
	attacher                 *uprobes.UprobeAttacher
	handshakeStateMapCleaner *ddebpf.MapCleaner[uint64, netebpf.SSLHandshakeState]
}

// NewSSLCertsProgram creates an SSLCertsProgram for the given ebpf manager
func NewSSLCertsProgram(mgr *manager.Manager, cfg *config.Config) (*SSLCertsProgram, error) {
	if !cfg.EnableCertCollection {
		return nil, nil
	}
	if err := ValidateSupported(); err != nil {
		return nil, fmt.Errorf("TLS cert collection was requested even though it's not supported (shouldn't get here): %w", err)
	}

	procRoot := kernel.ProcFSRoot()

	rules := []*uprobes.AttachRule{
		{
			Targets:          uprobes.AttachToSharedLibraries,
			ProbesSelector:   openSSLProbes,
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
		},
		{
			Targets:          uprobes.AttachToSharedLibraries,
			ProbesSelector:   cryptoProbes,
			LibraryNameRegex: regexp.MustCompile(`libcrypto.so`),
		},
	}

	program := &SSLCertsProgram{
		cfg:         cfg,
		ebpfManager: mgr,
	}
	attacherConfig := uprobes.AttacherConfig{
		ProcRoot:                       procRoot,
		Rules:                          rules,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		EbpfConfig:                     &cfg.Config,
		PerformInitialScan:             true,
		EnablePeriodicScanNewProcesses: true,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		ScanProcessesInterval:          30 * time.Second,
		EnableDetailedLogging:          false,
	}
	err := program.setupHandshakeStateMapCleaner()
	if err != nil {
		return nil, fmt.Errorf("error creating handshake map cleaner: %w", err)
	}

	program.attacher, err = uprobes.NewUprobeAttacher(CNMModuleName, CNMTLSAttacherName, attacherConfig, mgr, uprobes.NopOnAttachCallback, &uprobes.NativeBinaryInspector{}, monitor.GetProcessMonitor())
	if err != nil {
		return nil, fmt.Errorf("error initializing uprobes attacher: %w", err)
	}

	return program, nil
}

const (
	defaultMapCleanerBatchSize = 100
	handshakeStateTTL          = 30 * time.Second
	certInfoTTL                = 2 * time.Minute
)

func (p *SSLCertsProgram) setupHandshakeStateMapCleaner() error {
	mapObj, _, err := p.ebpfManager.GetMap(probes.SSLHandshakeStateMap)
	if err != nil {
		return fmt.Errorf("setupHandshakeStateMapCleaner failed to get map: %w", err)
	}

	p.handshakeStateMapCleaner, err = ddebpf.NewMapCleaner[uint64, netebpf.SSLHandshakeState](mapObj, defaultMapCleanerBatchSize, probes.SSLHandshakeStateMap, CNMModuleName)
	if err != nil {
		return fmt.Errorf("setupHandshakeStateMapCleaner failed to create cleaner: %w", err)
	}

	p.handshakeStateMapCleaner.Start(30*time.Second, nil, nil, func(now int64, _ uint64, val netebpf.SSLHandshakeState) bool {
		ts := int64(val.Timestamp)
		expired := ts > 0 && now-ts > handshakeStateTTL.Nanoseconds()
		return expired
	})

	return nil
}

// Start starts the attachment process
func (p *SSLCertsProgram) Start() error {
	return p.attacher.Start()
}

// Stop shuts down the attacher
func (p *SSLCertsProgram) Stop() {
	p.handshakeStateMapCleaner.Stop()
	p.attacher.Stop()
}
