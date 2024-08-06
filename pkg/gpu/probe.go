// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// go:build linux

package gpu

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/exp/maps"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Set a minimum kernel version
var minimumKernelVersion = kernel.VersionCode(5, 0, 0)

const (
	cudaEventMap      = "cuda_events"
	cudaAllocCacheMap = "cuda_alloc_cache"
)

// Probe represents the GPU monitoring probe
type Probe struct {
	mgr      *ddebpf.Manager
	cfg      *Config
	consumer *CudaEventConsumer
	attacher *uprobes.UprobeAttacher
}

// NewProbe starts the GPU monitoring probe
func NewProbe(cfg *Config, telemetryComponent telemetry.Component) (*Probe, error) {
	log.Debugf("[gpu] loading GPU monitoring probe")
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
		log.Debugf("[gpu] loading GPU monitoring probe 2")
		probe, err = startGPUProbe(buf, opts, telemetryComponent, cfg)
		if err != nil {
			log.Errorf("[gpu] starting GPU monitoring probe: %s", err)
			return fmt.Errorf("starting GPU monitoring probe: %s", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading asset: %s", err)
	}
	log.Debugf("[gpu] successfully loaded GPU monitoring probe")

	return probe, nil
}

func locateLibrary(name string) ([]string, error) {
	// Use a map to deduplicate results due to symlinks
	locations := make(map[string]struct{})

	candidateFolders := []string{
		"/usr/lib",
		"/usr/local",
	}

	for _, folder := range candidateFolders {
		err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
			// Ignore permission denied errors
			if err != nil && os.IsPermission(err) {
				return nil
			} else if err != nil {
				return err
			}

			if strings.HasPrefix(info.Name(), name) && strings.Contains(info.Name(), ".so") {
				path, err = filepath.EvalSymlinks(path)
				if err != nil {
					return fmt.Errorf("error resolving symlink %s: %w", path, err)
				}
				locations[path] = struct{}{}
			}

			return nil
		})

		if err != nil {
			return []string{}, fmt.Errorf("error browsing %s searching for library %s: %w", folder, name, err)
		}
	}

	return maps.Keys(locations), nil
}

func buildProbeUID(uprobe string, library string) (string, error) {
	hash := fnv.New64a()
	if _, err := hash.Write([]byte(uprobe + library)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum64()), nil
}

func startGPUProbe(buf bytecode.AssetReader, opts manager.Options, telemetryComponent telemetry.Component, cfg *Config) (*Probe, error) {
	mgr := ddebpf.NewManagerWithDefault(&manager.Manager{
		Maps: []*manager.Map{
			{Name: cudaAllocCacheMap},
		}})

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	opts.MapSpecEditors[cudaEventMap] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: 4096,
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	attachCfg := &uprobes.AttacherConfig{
		Rules: []*uprobes.AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile("libcudart\\.so"),
				Targets:          uprobes.AttachToExecutable | uprobes.AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.AllOf{
						Selectors: []manager.ProbesSelector{
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaLaunchKernel"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaMalloc"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uretprobe__cudaMalloc"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaStreamSynchronize"}},
							&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__cudaFree"}},
						},
					},
				},
			},
		},
		EbpfConfig:            cfg.Config,
		PerformInitialScan:    true,
		EnableDetailedLogging: true,
	}

	attacher, err := uprobes.NewUprobeAttacher("gpu", attachCfg, mgr, nil, &uprobes.NativeBinaryInspector{})
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
func (p *Probe) GetAndFlush() (*GPUStats, error) {
	now, err := ddebpf.NowNanoseconds()
	if err != nil {
		return nil, fmt.Errorf("getting current time: %w", err)
	}

	stats := GPUStats{}
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
	}

	return &stats, nil
}

func (p *Probe) startEventConsumer() {
	handler := ddebpf.NewRingBufferHandler(4096)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaEventMap},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: handler.RecordHandler,
			RecordGetter:  handler.RecordGetter,
		},
	}
	p.mgr.RingBuffers = append(p.mgr.RingBuffers, rb)
	p.consumer = NewCudaEventConsumer(handler)
	p.consumer.Start()
}
