// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"io"
	"math"
	"os"
	"regexp"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	gpuAttacherName = "gpu"

	// consumerChannelSize controls the size of the go channel that buffers ringbuffer
	// events (*ddebpf.RingBufferHandler).
	// This value must be multiplied by the single event size and the result will represent the heap memory pre-allocated in Go runtime
	// TODO: probably we need to reduce this value (see pkg/network/protocols/events/configuration.go for reference)
	consumerChannelSize = 4096
)

var (
	// defaultRingBufferSize controls the amount of memory in bytes used for buffering perf event data
	defaultRingBufferSize = os.Getpagesize()
)

// bpfMapName stores the name of the BPF maps storing statistics and other info
type bpfMapName = string

const (
	cudaEventsMap     bpfMapName = "cuda_events"
	cudaAllocCacheMap bpfMapName = "cuda_alloc_cache"
	cudaSyncCacheMap  bpfMapName = "cuda_sync_cache"
)

// probeFuncName stores the ebpf hook function name
type probeFuncName = string

const (
	cudaLaunchKernelProbe  probeFuncName = "uprobe__cudaLaunchKernel"
	cudaMallocProbe        probeFuncName = "uprobe__cudaMalloc"
	cudaMallocRetProbe     probeFuncName = "uretprobe__cudaMalloc"
	cudaStreamSyncProbe    probeFuncName = "uprobe__cudaStreamSynchronize"
	cudaStreamSyncRetProbe probeFuncName = "uretprobe__cudaStreamSynchronize"
	cudaFreeProbe          probeFuncName = "uprobe__cudaFree"
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
	eventHandler   ddebpf.EventHandler
}

// NewProbe creates and starts a GPU monitoring probe, containing relevant eBPF programs (uprobes), the
// consumers for the events generated from the uprobes, and the stats generator to aggregate the data from
// streams into per-process GPU stats.
func NewProbe(cfg *config.Config, deps ProbeDependencies) (*Probe, error) {
	log.Tracef("creating GPU monitoring probe...")
	if err := config.CheckGPUSupported(); err != nil {
		return nil, err
	}

<<<<<<< HEAD
	var probe *Probe
	filename := "gpu.o"
	if cfg.BPFDebug {
		filename = "gpu-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
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

func startGPUProbe(buf bytecode.AssetReader, opts manager.Options, deps ProbeDependencies, cfg *Config) (*Probe, error) {
	mgr := ddebpf.NewManagerWithDefault(&manager.Manager{
		Maps: []*manager.Map{
			{Name: cudaAllocCacheMap},
		}}, "gpu", &ebpftelemetry.ErrorsTelemetryModifier{})

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
=======
	if !cfg.EnableRuntimeCompiler && !cfg.EnableCORE {
		return nil, fmt.Errorf("%s probe supports CO-RE or Runtime Compilation modes, but none of them are enabled", sysconfig.GPUMonitoringModule)
>>>>>>> main
	}

	attachCfg := getAttacherConfig(cfg)
	// Note: this will later be replaced by a common way to enable the process monitor across system-probe
	procMon := monitor.GetProcessMonitor()
	if err := procMon.Initialize(false); err != nil {
		return nil, fmt.Errorf("error initializing process monitor: %w", err)
	}

	sysCtx, err := getSystemContext(deps.NvmlLib, cfg.ProcRoot)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}

	p := &Probe{
		cfg:     cfg,
		deps:    deps,
		procMon: procMon,
		sysCtx:  sysCtx,
	}

	allowRC := cfg.EnableRuntimeCompiler && cfg.AllowRuntimeCompiledFallback
	//try CO-RE first
	if cfg.EnableCORE {
		err = p.initCOREGPU(cfg)
		if err != nil {
			if allowRC {
				log.Warnf("error loading CO-RE %s, falling back to runtime compiled: %v", sysconfig.GPUMonitoringModule, err)
			} else {
				return nil, fmt.Errorf("error loading CO-RE %s: %w", sysconfig.GPUMonitoringModule, err)
			}
		}
	} else {
		//if CO-RE is disabled we don't need to check the AllowRuntimeCompiledFallback config flag
		allowRC = cfg.EnableRuntimeCompiler
	}

	//if manager is not initialized yet and RC is enabled, try runtime compilation
	if p.m == nil && allowRC {
		err = p.initRCGPU(cfg)
		if err != nil {
			return nil, fmt.Errorf("unable to compile %s probe: %w", sysconfig.GPUMonitoringModule, err)
		}
	}

	p.attacher, err = uprobes.NewUprobeAttacher(gpuAttacherName, attachCfg, p.m, nil, &uprobes.NativeBinaryInspector{}, procMon)
	if err != nil {
		return nil, fmt.Errorf("error creating uprobes attacher: %w", err)
	}

	p.consumer = newCudaEventConsumer(sysCtx, p.eventHandler, p.cfg)
	//TODO: decouple this to avoid sharing streamHandlers between consumer and statsGenerator
	p.statsGenerator = newStatsGenerator(sysCtx, p.consumer.streamHandlers)

	if err = p.start(); err != nil {
		return nil, err
	}
	log.Tracef("GPU monitoring probe successfully started")
	return p, nil
}

// Start loads the ebpf programs using the ebpf manager and starts the process monitor and event consumer
func (p *Probe) start() error {
	log.Tracef("starting GPU monitoring probe...")
	p.consumer.Start()

	if err := p.m.Start(); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	if err := p.attacher.Start(); err != nil {
		return fmt.Errorf("error starting uprobes attacher: %w", err)
	}
	return nil
}

// Close stops the probe
func (p *Probe) Close() {
	p.procMon.Stop()
	p.attacher.Stop()
	_ = p.m.Stop(manager.CleanAll)
	p.consumer.Stop()
	p.eventHandler.Stop()
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

func (p *Probe) initRCGPU(cfg *config.Config) error {
	buf, err := getRuntimeCompiledGPUMonitoring(cfg)
	if err != nil {
		return err
	}
	defer buf.Close()

	return p.setupManager(buf, manager.Options{})
}

func (p *Probe) initCOREGPU(cfg *config.Config) error {
	asset := getAssetName("gpu", cfg.BPFDebug)
	var err error
	err = ddebpf.LoadCOREAsset(asset, func(ar bytecode.AssetReader, o manager.Options) error {
		return p.setupManager(ar, o)
	})
	return err
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}

func (p *Probe) setupManager(buf io.ReaderAt, opts manager.Options) error {
	p.m = ddebpf.NewManagerWithDefault(&manager.Manager{
		/* 	We don't init the probes list here, because the manager will try to attach them at startup
		   	and fail since those are uprobes and their full path is resolved in runtime using the uprobeAttacher:
			adding those probe later via manager.AddHook API

		   	All manager's modifiers will still run as they operate on the ProgramSpecs map
			of the manager,which is populated while parsing the elf file and creating the CollectionSpec
		*/

		Maps: []*manager.Map{
			{
				Name: cudaAllocCacheMap,
			},
			{
				Name: cudaSyncCacheMap,
			},
		}})

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	p.setupSharedBuffer(&opts)

	if err := p.m.InitWithOptions(buf, &opts); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
	}

	return nil
}

// setupSharedBuffer sets up the ringbuffer to handle CUDA events produces by ebpf uprobes
// it must be called BEFORE the InitWithOptions method of the manager is called
func (p *Probe) setupSharedBuffer(o *manager.Options) {
	rbHandler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaEventsMap},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: rbHandler.RecordHandler,
			RecordGetter:  rbHandler.RecordGetter,
		},
	}

	ringBufferSize := toPowerOf2(defaultRingBufferSize)

	o.MapSpecEditors[cudaEventsMap] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(ringBufferSize),
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	p.m.Manager.RingBuffers = append(p.m.Manager.RingBuffers, rb)
	p.eventHandler = rbHandler
}

func getAttacherConfig(cfg *config.Config) uprobes.AttacherConfig {
	return uprobes.AttacherConfig{
		Rules: []*uprobes.AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libcudart\.so`),
				Targets:          uprobes.AttachToExecutable | uprobes.AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.AllOf{
						Selectors: []manager.ProbesSelector{
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaLaunchKernelProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaMallocProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaMallocRetProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaStreamSyncProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaStreamSyncRetProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaFreeProbe}},
						},
					},
				},
			},
		},
		EbpfConfig:         &cfg.Config,
		PerformInitialScan: cfg.InitialProcessSync,
	}
}

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}
