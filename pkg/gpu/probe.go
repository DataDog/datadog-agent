// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// go:build linux

package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/exp/maps"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Set a minimum kernel version
var minimumKernelVersion = kernel.VersionCode(5, 0, 0)

const (
	cudaKernelLaunchMap = "cuda_kernel_launches"
	cudaMemEventMap     = "cuda_memory_events"
)

// Probe represents the GPU monitoring probe
type Probe struct {
	mgr                  *ddebpf.Manager
	cfg                  *Config
	kernelLaunchConsumer *gpuebpf.CudaLaunchKernelConsumer
	memEventConsumer     *gpuebpf.CudaMemEventConsumer
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

func startGPUProbe(buf bytecode.AssetReader, opts manager.Options, telemetryComponent telemetry.Component, cfg *Config) (*Probe, error) {
	mgr := &ddebpf.Manager{
		Manager: &manager.Manager{},
	}

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	opts.MapSpecEditors[cudaKernelLaunchMap] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: 4096,
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	opts.MapSpecEditors[cudaMemEventMap] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: 4096,
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	uprobeToLibrary := map[string]string{
		"uprobe_cudaLaunchKernel": "libcudart.so",
		"uprobe_cudaMalloc":       "libcudart.so",
	}

	for uprobe, library := range uprobeToLibrary {
		locations, err := locateLibrary(library)
		if err != nil {
			return nil, fmt.Errorf("error locating library %s: %w", library, err)
		}
		locations = append(locations, cfg.manualProbedBinaries...)

		if len(locations) == 0 {
			log.Warnf("[gpu] could not find library %s for uprobe %s", library, uprobe)
			continue
		}

		for _, location := range locations {
			log.Debugf("[gpu] attaching uprobe %s to library %s at %s", uprobe, library, location)
			probe := &manager.Probe{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: uprobe,
					UID:          fmt.Sprintf("%s_%s", uprobe, location),
				},
				BinaryPath: location,
			}
			mgr.Probes = append(mgr.Probes, probe)
		}
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}

	p := &Probe{
		mgr: mgr,
		cfg: cfg,
	}

	p.startKernelLaunchConsumer()
	p.startMemEventConsumer()

	if err := mgr.InitWithOptions(buf, &opts); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := mgr.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	log.Infof("[gpu] GPU monitoring probe started, loaded %d probes", len(mgr.Probes))

	return p, nil
}

// Close stops the probe
func (p *Probe) Close() {
	_ = p.mgr.Stop(manager.CleanAll)

	if p.kernelLaunchConsumer != nil {
		p.kernelLaunchConsumer.Stop()
	}
	if p.memEventConsumer != nil {
		p.memEventConsumer.Stop()
	}
}

// GetAndFlush returns the GPU stats
func (p *Probe) GetAndFlush() (results GPUStats) {
	return GPUStats{}
}

func (p *Probe) startKernelLaunchConsumer() {
	handler := ddebpf.NewRingBufferHandler(4096)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaKernelLaunchMap},
		RingBufferOptions: manager.RingBufferOptions{
			RingBufferSize: 4096,
			RecordHandler:  handler.RecordHandler,
			RecordGetter:   handler.RecordGetter,
		},
	}
	p.mgr.RingBuffers = append(p.mgr.RingBuffers, rb)
	p.kernelLaunchConsumer = gpuebpf.NewCudaLaunchKernelConsumer(handler)
	p.kernelLaunchConsumer.Start()
}

func (p *Probe) startMemEventConsumer() {
	handler := ddebpf.NewRingBufferHandler(4096)
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaMemEventMap},
		RingBufferOptions: manager.RingBufferOptions{
			RingBufferSize: 4096,
			RecordHandler:  handler.RecordHandler,
			RecordGetter:   handler.RecordGetter,
		},
	}
	p.mgr.RingBuffers = append(p.mgr.RingBuffers, rb)
	p.memEventConsumer = gpuebpf.NewCudaMemEventConsumer(handler)
	p.memEventConsumer.Start()
}
