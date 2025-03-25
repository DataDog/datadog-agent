// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	gpuAttacherName    = GpuModuleName
	gpuTelemetryModule = GpuModuleName

	// consumerChannelSize controls the size of the go channel that buffers ringbuffer
	// events (*ddebpf.RingBufferHandler).
	// This value must be multiplied by the single event size and the result will represent the heap memory pre-allocated in Go runtime
	// TODO: probably we need to reduce this value (see pkg/network/protocols/events/configuration.go for reference)
	consumerChannelSize = 4096

	defaultMapCleanerInterval  = 5 * time.Minute
	defaultMapCleanerBatchSize = 100
	defaultEventTTL            = defaultMapCleanerInterval
)

var (
	// defaultRingBufferSize controls the amount of memory in bytes used for buffering perf event data
	defaultRingBufferSize = 2 * os.Getpagesize()
)

// bpfMapName stores the name of the BPF maps storing statistics and other info
type bpfMapName = string

const (
	cudaEventsRingbuf      bpfMapName = "cuda_events"
	cudaAllocCacheMap      bpfMapName = "cuda_alloc_cache"
	cudaSyncCacheMap       bpfMapName = "cuda_sync_cache"
	cudaSetDeviceCacheMap  bpfMapName = "cuda_set_device_cache"
	cudaEventStreamMap     bpfMapName = "cuda_event_to_stream"
	cudaEventQueryCacheMap bpfMapName = "cuda_event_query_cache"
)

// probeFuncName stores the ebpf hook function name
type probeFuncName = string

const (
	cudaLaunchKernelProbe        probeFuncName = "uprobe__cudaLaunchKernel"
	cudaMallocProbe              probeFuncName = "uprobe__cudaMalloc"
	cudaMallocRetProbe           probeFuncName = "uretprobe__cudaMalloc"
	cudaStreamSyncProbe          probeFuncName = "uprobe__cudaStreamSynchronize"
	cudaStreamSyncRetProbe       probeFuncName = "uretprobe__cudaStreamSynchronize"
	cudaFreeProbe                probeFuncName = "uprobe__cudaFree"
	cudaSetDeviceProbe           probeFuncName = "uprobe__cudaSetDevice"
	cudaSetDeviceRetProbe        probeFuncName = "uretprobe__cudaSetDevice"
	cudaEventRecordProbe         probeFuncName = "uprobe__cudaEventRecord"
	cudaEventQueryProbe          probeFuncName = "uprobe__cudaEventQuery"
	cudaEventQueryRetProbe       probeFuncName = "uretprobe__cudaEventQuery"
	cudaEventSynchronizeProbe    probeFuncName = "uprobe__cudaEventSynchronize"
	cudaEventSynchronizeRetProbe probeFuncName = "uretprobe__cudaEventSynchronize"
	cudaEventDestroyProbe        probeFuncName = "uprobe__cudaEventDestroy"
)

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	// Telemetry is the telemetry component
	Telemetry telemetry.Component

	// NvmlLib is the NVML library interface
	NvmlLib nvml.Interface

	// ProcessMonitor is the process monitor interface
	ProcessMonitor uprobes.ProcessMonitor

	// WorkloadMeta used to retrieve data about workloads (containers, processes) running
	// on the host
	WorkloadMeta workloadmeta.Component
}

// NewProbeDependencies creates a new ProbeDependencies instance
func NewProbeDependencies(cfg *config.Config, telemetry telemetry.Component, processMonitor uprobes.ProcessMonitor, workloadMeta workloadmeta.Component) (ProbeDependencies, error) {
	nvmlLib := nvml.New(nvml.WithLibraryPath(cfg.NVMLLibraryPath))
	ret := nvmlLib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return ProbeDependencies{}, fmt.Errorf("unable to initialize NVML library: %w", ret)
	}

	return ProbeDependencies{
		Telemetry:      telemetry,
		NvmlLib:        nvmlLib,
		ProcessMonitor: processMonitor,
		WorkloadMeta:   workloadMeta,
	}, nil
}

// Probe represents the GPU monitoring probe
type Probe struct {
	m                *ddebpf.Manager
	cfg              *config.Config
	consumer         *cudaEventConsumer
	attacher         *uprobes.UprobeAttacher
	statsGenerator   *statsGenerator
	deps             ProbeDependencies
	sysCtx           *systemContext
	eventHandler     ddebpf.EventHandler
	telemetry        *probeTelemetry
	mapCleanerEvents *ddebpf.MapCleaner[gpuebpf.CudaEventKey, gpuebpf.CudaEventValue]
	streamHandlers   *streamCollection
}

type probeTelemetry struct {
	sentEntries telemetry.Counter
}

func newProbeTelemetry(tm telemetry.Component) *probeTelemetry {
	subsystem := gpuTelemetryModule + "__probe"

	return &probeTelemetry{
		sentEntries: tm.NewCounter(subsystem, "sent_entries", nil, "Number of GPU events sent to the agent"),
	}
}

// NewProbe creates and starts a GPU monitoring probe, containing relevant eBPF programs (uprobes), the
// consumers for the events generated from the uprobes, and the stats generator to aggregate the data from
// streams into per-process GPU stats.
func NewProbe(cfg *config.Config, deps ProbeDependencies) (*Probe, error) {
	log.Tracef("creating GPU monitoring probe...")
	if err := config.CheckGPUSupported(); err != nil {
		return nil, err
	}

	if !cfg.EnableRuntimeCompiler && !cfg.EnableCORE {
		return nil, fmt.Errorf("%s probe supports CO-RE or Runtime Compilation modes, but none of them are enabled", sysconfig.GPUMonitoringModule)
	}

	attachCfg := getAttacherConfig(cfg)
	sysCtx, err := getSystemContext(deps.NvmlLib, cfg.ProcRoot, deps.WorkloadMeta, deps.Telemetry)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}

	sysCtx.fatbinParsingEnabled = cfg.EnableFatbinParsing

	p := &Probe{
		cfg:       cfg,
		deps:      deps,
		sysCtx:    sysCtx,
		telemetry: newProbeTelemetry(deps.Telemetry),
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

	p.attacher, err = uprobes.NewUprobeAttacher(GpuModuleName, gpuAttacherName, attachCfg, p.m, nil, &uprobes.NativeBinaryInspector{}, deps.ProcessMonitor)
	if err != nil {
		return nil, fmt.Errorf("error creating uprobes attacher: %w", err)
	}

	p.streamHandlers = newStreamCollection(sysCtx, deps.Telemetry)
	p.consumer = newCudaEventConsumer(sysCtx, p.streamHandlers, p.eventHandler, p.cfg, deps.Telemetry)
	p.statsGenerator = newStatsGenerator(sysCtx, p.streamHandlers, deps.Telemetry)

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
	ddebpf.AddNameMappings(p.m.Manager, GpuModuleName)

	if err := p.attacher.Start(); err != nil {
		return fmt.Errorf("error starting uprobes attacher: %w", err)
	}
	return nil
}

// Close stops the probe
func (p *Probe) Close() {
	p.attacher.Stop()
	_ = p.m.Stop(manager.CleanAll)
	ddebpf.ClearNameMappings(GpuModuleName)
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
	p.telemetry.sentEntries.Add(float64(len(stats.Metrics)))
	p.cleanupFinished()

	return stats, nil
}

func (p *Probe) cleanupFinished() {
	p.statsGenerator.cleanupFinishedAggregators()
	p.streamHandlers.clean()
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
	err := ddebpf.LoadCOREAsset(asset, func(ar bytecode.AssetReader, o manager.Options) error {
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
			{Name: cudaAllocCacheMap},
			{Name: cudaSyncCacheMap},
			{Name: cudaSetDeviceCacheMap},
			{Name: cudaEventStreamMap},
			{Name: cudaEventQueryCacheMap},
		}}, "gpu", &ebpftelemetry.ErrorsTelemetryModifier{})

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	p.setupSharedBuffer(&opts)

	if err := p.m.InitWithOptions(buf, &opts); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
	}

	if err := p.setupMapCleaner(); err != nil {
		return fmt.Errorf("error setting up map cleaner: %w", err)
	}

	return nil
}

// setupSharedBuffer sets up the ringbuffer to handle CUDA events produces by ebpf uprobes
// it must be called BEFORE the InitWithOptions method of the manager is called
func (p *Probe) setupSharedBuffer(o *manager.Options) {
	rbHandler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaEventsRingbuf},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: rbHandler.RecordHandler,
			RecordGetter:  rbHandler.RecordGetter,
		},
	}

	ringBufferSize := toPowerOf2(defaultRingBufferSize)

	o.MapSpecEditors[cudaEventsRingbuf] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(ringBufferSize),
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	p.m.Manager.RingBuffers = append(p.m.Manager.RingBuffers, rb)
	p.eventHandler = rbHandler

	rb.TelemetryEnabled = true
	ebpftelemetry.ReportRingBufferTelemetry(rb)
}

// CollectConsumedEvents waits until the debug collector stores count events and returns them
func (p *Probe) CollectConsumedEvents(ctx context.Context, count int) ([][]byte, error) {
	p.consumer.debugCollector.enable(count)

	return p.consumer.debugCollector.wait(ctx)
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
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaSetDeviceProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaSetDeviceRetProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventRecordProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventQueryProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventQueryRetProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventSynchronizeProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventSynchronizeRetProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaEventDestroyProbe}},
						},
					},
				},
			},
		},
		EbpfConfig:                     &cfg.Config,
		PerformInitialScan:             cfg.InitialProcessSync,
		SharedLibsLibset:               sharedlibraries.LibsetGPU,
		ScanProcessesInterval:          cfg.ScanProcessesInterval,
		EnablePeriodicScanNewProcesses: true,
		EnableDetailedLogging:          true,
	}
}

func (p *Probe) setupMapCleaner() error {
	eventsMap, _, err := p.m.GetMap(cudaEventStreamMap)
	if err != nil {
		return fmt.Errorf("error getting %s map: %w", cudaEventStreamMap, err)
	}

	p.mapCleanerEvents, err = ddebpf.NewMapCleaner[gpuebpf.CudaEventKey, gpuebpf.CudaEventValue](eventsMap, defaultMapCleanerBatchSize, cudaEventStreamMap, GpuModuleName)
	if err != nil {
		return fmt.Errorf("error creating map cleaner: %w", err)
	}

	p.mapCleanerEvents.Clean(defaultMapCleanerInterval, nil, nil, func(now int64, _ gpuebpf.CudaEventKey, val gpuebpf.CudaEventValue) bool {
		return (now - int64(val.Access_ktime_ns)) > defaultEventTTL.Nanoseconds()
	})

	return nil
}

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}
