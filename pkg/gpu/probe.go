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
	"slices"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	usmutils "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logLimitProbe is used to limit the number of times we log messages about streams and cuda events, as that can be very verbose
var logLimitProbe = log.NewLogLimit(20, 10*time.Minute)

const (
	// consumerChannelSize controls the size of the go channel that buffers ringbuffer
	// events (*ddebpf.RingBufferHandler).
	// This value must be multiplied by the single event size and the result will represent the heap memory pre-allocated in Go runtime
	// TODO: probably we need to reduce this value (see pkg/network/protocols/events/configuration.go for reference)
	consumerChannelSize = 4096

	defaultMapCleanerInterval  = 5 * time.Minute
	defaultMapCleanerBatchSize = 100
	defaultEventTTL            = defaultMapCleanerInterval
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
	cudaMemcpyCacheMap     bpfMapName = "cuda_memcpy_cache"
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
	cudaMemcpyProbe              probeFuncName = "uprobe__cudaMemcpy"
	cudaMemcpyRetProbe           probeFuncName = "uretprobe__cudaMemcpy"
	setenvProbe                  probeFuncName = "uprobe__setenv"
)

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	// Telemetry is the telemetry component
	Telemetry telemetry.Component

	// ProcessMonitor is the process monitor interface
	ProcessMonitor uprobes.ProcessMonitor

	// WorkloadMeta used to retrieve data about workloads (containers, processes) running
	// on the host
	WorkloadMeta workloadmeta.Component
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
	lastCheck        atomic.Int64
}

type probeTelemetry struct {
	sentEntries telemetry.Counter
}

func newProbeTelemetry(tm telemetry.Component) *probeTelemetry {
	subsystem := consts.GpuTelemetryModule + "__probe"

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

	sysCtx, err := getSystemContext(
		withProcRoot(cfg.ProcRoot),
		withWorkloadMeta(deps.WorkloadMeta),
		withTelemetry(deps.Telemetry),
		withFatbinParsingEnabled(cfg.EnableFatbinParsing),
		withConfig(cfg),
	)
	if err != nil {
		return nil, fmt.Errorf("error getting system context: %w", err)
	}

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

	attachCfg := getAttacherConfig(cfg)
	p.attacher, err = uprobes.NewUprobeAttacher(consts.GpuModuleName, consts.GpuAttacherName, attachCfg, p.m, nil, &uprobes.NativeBinaryInspector{}, deps.ProcessMonitor)
	if err != nil {
		return nil, fmt.Errorf("error creating uprobes attacher: %w", err)
	}

	p.streamHandlers = newStreamCollection(sysCtx, deps.Telemetry, cfg)
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
	ddebpf.AddNameMappings(p.m.Manager, consts.GpuModuleName)

	if err := p.attacher.Start(); err != nil {
		return fmt.Errorf("error starting uprobes attacher: %w", err)
	}

	ddebpf.AddProbeFDMappings(p.m.Manager)

	return nil
}

// Close stops the probe
func (p *Probe) Close() {
	p.attacher.Stop()
	_ = p.m.Stop(manager.CleanAll)
	ddebpf.ClearProgramIDMappings(consts.GpuModuleName)
	p.consumer.Stop()
	p.eventHandler.Stop()
}

// GetAndFlush returns the GPU stats
func (p *Probe) GetAndFlush() (*model.GPUStats, error) {
	p.lastCheck.Store(time.Now().Unix())

	now, err := ddebpf.NowNanoseconds()
	if err != nil {
		return nil, fmt.Errorf("error getting current time: %w", err)
	}

	stats, err := p.statsGenerator.getStats(now)
	if err != nil {
		return nil, err
	}

	p.telemetry.sentEntries.Add(float64(len(stats.Metrics)))
	p.cleanupFinished(now)

	return stats, nil
}

func (p *Probe) cleanupFinished(nowKtime int64) {
	p.statsGenerator.cleanupFinishedAggregators()
	p.streamHandlers.clean(nowKtime)
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
			{Name: cudaMemcpyCacheMap},
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

	devCount := p.sysCtx.deviceCache.Count()
	if devCount == 0 {
		devCount = 1 // Don't let the buffer size be 0
	}

	// The activity of eBPF events will scale with the number of devices, unlike in other
	// eBPF modules where the activity is bound to the number of CPUs.
	numPages := p.cfg.RingBufferSizePagesPerDevice * devCount
	ringBufferSize := toPowerOf2(numPages * os.Getpagesize())

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
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaMemcpyProbe}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: cudaMemcpyRetProbe}},
						},
					},
				},
			},
			{
				LibraryNameRegex: regexp.MustCompile(`libc\.so`),
				Targets:          uprobes.AttachToSharedLibraries | uprobes.AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.AllOf{
						Selectors: []manager.ProbesSelector{
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: setenvProbe}},
						},
					},
				},
			},
		},
		EbpfConfig:                     &cfg.Config,
		PerformInitialScan:             cfg.InitialProcessSync,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetGPU, sharedlibraries.LibsetLibc},
		ScanProcessesInterval:          cfg.ScanProcessesInterval,
		EnablePeriodicScanNewProcesses: true,
		EnableDetailedLogging:          false,
		ExcludeTargets:                 uprobes.ExcludeInternal | uprobes.ExcludeSelf,
	}
}

func (p *Probe) setupMapCleaner() error {
	eventsMap, _, err := p.m.GetMap(cudaEventStreamMap)
	if err != nil {
		return fmt.Errorf("error getting %s map: %w", cudaEventStreamMap, err)
	}

	p.mapCleanerEvents, err = ddebpf.NewMapCleaner[gpuebpf.CudaEventKey, gpuebpf.CudaEventValue](eventsMap, defaultMapCleanerBatchSize, cudaEventStreamMap, consts.GpuModuleName)
	if err != nil {
		return fmt.Errorf("error creating map cleaner: %w", err)
	}

	p.mapCleanerEvents.Start(defaultMapCleanerInterval, nil, nil, func(now int64, _ gpuebpf.CudaEventKey, val gpuebpf.CudaEventValue) bool {
		return (now - int64(val.Access_ktime_ns)) > defaultEventTTL.Nanoseconds()
	})

	return nil
}

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}

// GetDebugStats returns the debug stats for the GPU monitoring probe
func (p *Probe) GetDebugStats() map[string]interface{} {
	var activeGpus []map[string]interface{}

	for _, gpu := range p.sysCtx.deviceCache.All() {
		info := gpu.GetDeviceInfo()
		wmetaGpu, err := p.sysCtx.workloadmeta.GetGPU(info.UUID)
		_, isMIG := gpu.(*safenvml.MIGDevice)

		activeGpus = append(activeGpus, map[string]interface{}{
			"uuid":       info.UUID,
			"name":       info.Name,
			"index":      info.Index,
			"sm_version": info.SMVersion,
			"is_mig":     isMIG,
			"in_wmeta":   err == nil && wmetaGpu != nil,
		})
	}

	healthStatus := health.GetLive()

	return map[string]interface{}{
		"active_gpus":  activeGpus,
		"kernel_cache": p.sysCtx.cudaKernelCache.GetStats(),
		"attacher": map[string]interface{}{
			"active_processes":  usmutils.GetTracedProgramList(consts.GpuModuleName),
			"blocked_processes": usmutils.GetBlockedPathIDsList(consts.GpuModuleName),
		},
		"consumer_healthy": slices.Contains(healthStatus.Healthy, consts.GpuConsumerHealthName),
		"last_check":       p.lastCheck.Load(),
	}
}
