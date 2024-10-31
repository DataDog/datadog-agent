// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	"os"
	"regexp"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cudaEventMap      = "cuda_events"
	cudaAllocCacheMap = "cuda_alloc_cache"
	gpuAttacherName   = "gpu"
)

const consumerChannelSize = 4096

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	// Telemetry is the telemetry component
	Telemetry telemetry.Component

	// NvmlLib is the NVML library interface
	NvmlLib nvml.Interface
}

// Probe represents the GPU monitoring probe
type Probe struct {
	mgr            *ddebpf.Manager
	cfg            *config.Config
	consumer       *cudaEventConsumer
	attacher       *uprobes.UprobeAttacher
	statsGenerator *statsGenerator
	deps           ProbeDependencies
	procMon        *monitor.ProcessMonitor
}

// NewProbe starts the GPU monitoring probe, setting up the eBPF program and the uprobes, the
// consumers for the events generated from the uprobes, and the stats generator to aggregate the data from
// streams into per-process GPU stats.
func NewProbe(cfg *config.Config, deps ProbeDependencies) (*Probe, error) {
	log.Debugf("starting GPU monitoring probe...")
	if err := config.CheckGPUSupported(); err != nil {
		return nil, err
	}

	var probe *Probe
	filename := "gpu.o"
	if cfg.BPFDebug {
		filename = "gpu-debug.o"
	}
	err := ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		var err error
		probe, err = startGPUProbe(buf, opts, deps, cfg)
		if err != nil {
			return fmt.Errorf("cannot start GPU monitoring probe: %s", err)
		}
		log.Debugf("started GPU monitoring probe")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading asset: %s", err)
	}

	return probe, nil
}

func startGPUProbe(buf bytecode.AssetReader, opts manager.Options, deps ProbeDependencies, cfg *config.Config) (*Probe, error) {
	mgr := ddebpf.NewManagerWithDefault(&manager.Manager{
		Maps: []*manager.Map{
			{Name: cudaAllocCacheMap},
		}})

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	// Ring buffer size has to be a multiple of the page size, and we want to have at least 4096 bytes
	pagesize := os.Getpagesize()
	ringbufSize := pagesize
	minRingbufSize := 4096
	if minRingbufSize > ringbufSize {
		ringbufSize = (minRingbufSize/pagesize + 1) * pagesize
	}

	opts.MapSpecEditors[cudaEventMap] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(ringbufSize),
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
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

	attacher, err := uprobes.NewUprobeAttacher(gpuAttacherName, attachCfg, mgr, nil, &uprobes.NativeBinaryInspector{}, procMon)
	if err != nil {
		return nil, fmt.Errorf("error creating uprobes attacher: %w", err)
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}

	p := &Probe{
		mgr:      mgr,
		cfg:      cfg,
		attacher: attacher,
		deps:     deps,
		procMon:  procMon,
	}

	sysCtx, err := getSystemContext(deps.NvmlLib)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}

	now, err := ddebpf.NowNanoseconds()
	if err != nil {
		return nil, fmt.Errorf("error getting current time: %w", err)
	}

	p.startEventConsumer()
	p.statsGenerator = newStatsGenerator(sysCtx, now, p.consumer.streamHandlers)

	if err := mgr.InitWithOptions(buf, &opts); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := mgr.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	if err := attacher.Start(); err != nil {
		return nil, fmt.Errorf("error starting uprobes attacher: %w", err)
	}

	return p, nil
}

// Close stops the probe
func (p *Probe) Close() {
	if p.procMon != nil {
		p.procMon.Stop()
	}

	if p.attacher != nil {
		p.attacher.Stop()
	}

	_ = p.mgr.Stop(manager.CleanAll)

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
	p.mgr.RingBuffers = append(p.mgr.RingBuffers, rb)
	p.consumer = newCudaEventConsumer(handler, p.cfg)
	p.consumer.Start()
}
