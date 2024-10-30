// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/probe"
	"regexp"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	cudaEventMap      = "cuda_events"
	cudaAllocCacheMap = "cuda_alloc_cache"
	gpuAttacherName   = "gpu"
)

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	// Telemetry is the telemetry component
	Telemetry telemetry.Component

	// NvmlLib is the NVML library interface
	NvmlLib nvml.Interface
}

// Probe represents the GPU monitoring probe
type Probe struct {
	m              *ddebpf.Manager
	cfg            *config.Config
	consumer       *cudaEventConsumer
	attacher       *uprobes.UprobeAttacher
	statsGenerator *statsGenerator
	deps           ProbeDependencies
	sysCtx         *systemContext
	procMon        *monitor.ProcessMonitor
}

// NewProbe starts the GPU monitoring probe, setting up the eBPF program and the uprobes, the
// consumers for the events generated from the uprobes, and the stats generator to aggregate the data from
// streams into per-process GPU stats.
func NewProbe(cfg *config.Config, deps ProbeDependencies) (*Probe, error) {
	var err error
	var m *ddebpf.Manager
	if err = config.CheckGPUSupported(); err != nil {
		return nil, err
	}

	log.Tracef("creating GPU monitoring probe...")
	filename := "gpu.o"
	if cfg.BPFDebug {
		filename = "gpu-debug.o"
	}

	err = ddebpf.LoadCOREAsset(filename, func(ar bytecode.AssetReader, o manager.Options) error {
		m, err = probe.GetManager(ar, o)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("error loading CO-RE %s: %w", sysconfig.GPUMonitoringModule, err)
	}

	attachCfg := uprobes.AttacherConfig{
		Rules: []*uprobes.AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libcudart\.so`),
				Targets:          uprobes.AttachToExecutable | uprobes.AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.AllOf{
						Selectors: []manager.ProbesSelector{
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaLaunchKernel"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaMalloc"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uretprobe__cudaMalloc"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaStreamSynchronize"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uretprobe__cudaStreamSynchronize"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaFree"}},
						},
					},
				},
			},
		},
		EbpfConfig:         &cfg.Config,
		PerformInitialScan: cfg.InitialProcessSync,
	}

	// Note: this will later be replaced by a common way to enable the process monitor across system-probe
	procMon := monitor.GetProcessMonitor()
	if err := procMon.Initialize(false); err != nil {
		return nil, fmt.Errorf("error initializing process monitor: %w", err)
	}

	attacher, err := uprobes.NewUprobeAttacher(gpuAttacherName, attachCfg, m, nil, &uprobes.NativeBinaryInspector{}, procMon)
	if err != nil {
		return nil, fmt.Errorf("error creating uprobes attacher: %w", err)
	}

	sysCtx, err := getSystemContext(deps.NvmlLib)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}

	p := &Probe{
		m:        m,
		cfg:      cfg,
		attacher: attacher,
		deps:     deps,
		procMon:  procMon,
	}

	p.sysCtx, err = getSystemContext(deps.NvmlLib, cfg.Config.ProcRoot)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}


	p.consumer = newCudaEventConsumer(probe.EventHandler, p.cfg)
	//TODO: decouple this to avoid sharing streamHandlers between consumer and statsGenerator
	p.statsGenerator = newStatsGenerator(sysCtx, p.consumer.streamHandlers)
	log.Tracef("GPU monitoring probe successfully created")
	return p, nil
}

func (p *Probe) Start(deps ProbeDependencies) error {
	log.Tracef("starting GPU monitoring probe...")
	// Note: this will later be replaced by a common way to enable the process monitor across system-probe
	procMon := monitor.GetProcessMonitor()
	if err := procMon.Initialize(false); err != nil {
		return fmt.Errorf("error initializing process monitor: %w", err)
	}

	p.consumer.Start()

	if err := p.m.Start(); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	if err := p.attacher.Start(); err != nil {
		return fmt.Errorf("error starting uprobes attacher: %w", err)
	}

	log.Tracef("GPU monitoring probe sucessfully started")
	return nil
}

// Close stops the probe
func (p *Probe) Close() {
	if p.procMon != nil {
		p.procMon.Stop()
	}

	if p.attacher != nil {
		p.attacher.Stop()
	}

	_ = p.m.Stop(manager.CleanAll)

	if p.consumer != nil {
		p.consumer.Stop()
	}
}

// GetAndFlush returns the GPU stats
func (p *Probe) GetAndFlush() (*model.GPUStats, error) {
	now, err := ddebpf.NowNanoseconds()
	if err != nil {
		return nil, fmt.Errorf("error getting current time: %w", err)
	}

	stats := p.statsGenerator.getStats(now)

	p.cleanupFinished()

	return stats, nil
}

func (p *Probe) cleanupFinished() {
	p.statsGenerator.cleanupFinishedAggregators()
	p.consumer.cleanFinishedHandlers()
}

func (p *Probe) startEventConsumer() {
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaEventMap},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: handler.RecordHandler,
			RecordGetter:  handler.RecordGetter,
		},
	}
	p.m.RingBuffers = append(p.m.RingBuffers, rb)
	p.consumer = newCudaEventConsumer(p.sysCtx, handler, p.cfg)
	p.consumer.Start()
}
