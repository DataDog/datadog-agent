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
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Set a minimum kernel version
var minimumKernelVersion = kernel.VersionCode(5, 8, 0)

const (
	cudaEventMap      = "cuda_events"
	cudaAllocCacheMap = "cuda_alloc_cache"
	gpuAttacherName   = "gpu"
)

const consumerChannelSize = 4096

// Probe represents the GPU monitoring probe
type Probe struct {
	mgr      *ddebpf.Manager
	cfg      *Config
	consumer *cudaEventConsumer
	attacher *uprobes.UprobeAttacher
	sysCtx   *systemContext
}

// NewProbe starts the GPU monitoring probe
func NewProbe(cfg *Config, telemetryComponent telemetry.Component) (*Probe, error) {
	log.Debugf("starting GPU monitoring probe...")
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	var probe *Probe
	filename := "gpu.o"
	if cfg.BPFDebug {
		filename = "gpu-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		var err error
		probe, err = startGPUProbe(buf, opts, telemetryComponent, cfg)
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

func startGPUProbe(buf bytecode.AssetReader, opts manager.Options, _ telemetry.Component, cfg *Config) (*Probe, error) {
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
		EbpfConfig:         cfg.Config,
		PerformInitialScan: cfg.InitialProcessSync,
	}

	attacher, err := uprobes.NewUprobeAttacher(gpuAttacherName, attachCfg, mgr, nil, &uprobes.NativeBinaryInspector{})
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
	}

	var sysCtxOpts []systemContextOpts
	if cfg.DisableGpuDeviceQuery {
		sysCtxOpts = append(sysCtxOpts, systemContextOptDisableGpuQuery)
	}

	p.sysCtx, err = getSystemContext(sysCtxOpts...)
	if err != nil {
		return nil, fmt.Errorf("error getting GPU system info: %w", err)
	}

	p.startEventConsumer()

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
		return nil, fmt.Errorf("getting current time: %w", err)
	}

	stats := model.GPUStats{}
	for key, handler := range p.consumer.streamHandlers {
		currData := handler.getCurrentData(uint64(now))
		pastData := handler.getPastData(true)

		if currData != nil {
			currData.Key = key
			stats.CurrentData = append(stats.CurrentData, currData)
		}

		if pastData != nil {
			pastData.Key = key
			stats.PastData = append(stats.PastData, pastData)
		}

		if handler.processEnded {
			delete(p.consumer.streamHandlers, key)
		}
	}

	return &stats, nil
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
	p.consumer = NewCudaEventConsumer(handler, p.cfg, p.sysCtx)
	p.consumer.Start()
}
