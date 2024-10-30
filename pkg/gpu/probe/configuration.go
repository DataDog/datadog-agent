// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package probe

import (
	"fmt"
	"io"
	"math"
	"os"
	"regexp"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
)

// defaultRingBufferSize controls the amount of memory in bytes used for buffering perf event data
var defaultRingBufferSize = os.Getpagesize()

// consumerChannelSize controls the size of the go channel that buffers ringbuffer
// events (*ddebpf.RingBufferHandler).
// This value must be multiplied by the single event size and the result will represent the heap memory pre-allocated in Go runtime
// TODO: probably we need to reduce this value (see pkg/network/protocols/events/configuration.go for reference)
const consumerChannelSize = 4096

var EventHandler = ddebpf.NewRingBufferHandler(consumerChannelSize)

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

// GetAttacherConfig returns the configuration for the uprobeAttacher
func GetAttacherConfig(cfg *config.Config) uprobes.AttacherConfig {
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

func GetManager(buf io.ReaderAt, opts manager.Options) (*ddebpf.Manager, error) {
	m := ddebpf.NewManagerWithDefault(&manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaLaunchKernelProbe,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaMallocProbe,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaMallocRetProbe,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaStreamSyncProbe,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaStreamSyncRetProbe,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: cudaFreeProbe,
				},
			},
		},
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

	setupSharedBuffer(m.Manager, &opts)

	if err := m.InitWithOptions(buf, &opts); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	return m, nil
}

// setupSharedBuffer sets up the ringbuffer to handle CUDA events produces by ebpf uprobes
// it must be called BEFORE the InitWithOptions method of the manager is called
func setupSharedBuffer(m *manager.Manager, o *manager.Options) {
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: cudaEventsMap},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: EventHandler.RecordHandler,
			RecordGetter:  EventHandler.RecordGetter,
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

	m.RingBuffers = append(m.RingBuffers, rb)
}

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}
