// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"math"
	"os"
	"slices"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/encoding"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// defaultPerfBufferSize controls the amount of memory in bytes used *per CPU*
// allocated for buffering perf event data
var defaultPerfEventBufferSize = 16 * os.Getpagesize()

// defaultPerfHandlerSize controls the size of the go channel that buffers perf
// events. All perf events handled by this library have
// fixed size (sizeof(batch_data_t)) which is ~4KB, so by choosing a value of
// 100 we'll be buffering up to ~400KB of data in *Go* heap memory.
const defaultPerfHandlerSize = 100

// Configure a given `*manager.Manager` for event processing
// This essentially instantiates the perf map/ring buffers and configure the
// eBPF maps where events are enqueued.
// Note this must be called *before* manager.InitWithOptions
func Configure(cfg *config.Config, proto string, m *manager.Manager, o *manager.Options) {
	if alreadySetUp(proto, m) {
		return
	}

	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		numCPUs = 96
		log.Error("unable to detect number of CPUs. assuming 96 cores")
	}

	configureBatchMaps(proto, o, numCPUs)

	callbackFn, callbackCh := ddsync.CallbackChannel[*batch](defaultPerfHandlerSize)
	mapName := eventMapName(proto)

	eopts := perf.EventHandlerOptions{
		MapName: mapName,
		Handler: encoding.BinaryUnmarshalCallback(batchPool.Get, func(b *batch, err error) {
			if err != nil {
				log.Debug(err.Error())
				return
			}
			callbackFn(b)
		}),
		TelemetryEnabled:  cfg.InternalTelemetryEnabled,
		UseRingBuffer:     cfg.EnableUSMRingBuffers,
		UpgradePerfBuffer: true,
		PerfOptions: perf.PerfBufferOptions{
			BufferSize:   defaultPerfEventBufferSize,
			Watermark:    1,
			WakeupEvents: 0,
		},
		RingBufOptions: perf.RingBufferOptions{
			BufferSize: toPowerOf2(numCPUs * defaultPerfEventBufferSize),
		},
	}

	eh, err := perf.NewEventHandler(eopts)
	if err != nil {
		log.Errorf("unable to create perf event handler: %v", err)
		return
	}
	if err := eh.Init(m, o); err != nil {
		log.Errorf("unable to initialize perf event handler: %v", err)
		return
	}
	utils.AddBoolConst(o, eh.MapType() == ebpf.RingBuf, "use_ring_buffer")
	// The map appears as we list it in the Protocol struct.
	m.Maps = slices.DeleteFunc(m.Maps, func(currentMap *manager.Map) bool {
		return currentMap.Name == mapName
	})
	setHandler(proto, callbackCh)
}

func configureBatchMaps(proto string, o *manager.Options, numCPUs int) {
	if o.MapSpecEditors == nil {
		o.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	o.MapSpecEditors[proto+batchMapSuffix] = manager.MapSpecEditor{
		MaxEntries: uint32(numCPUs * batchPagesPerCPU),
		EditorFlag: manager.EditMaxEntries,
	}
}

func eventMapName(proto string) string {
	return proto + eventsMapSuffix
}

func alreadySetUp(proto string, m *manager.Manager) bool {
	// check if we already have configured this perf map this can happen in the
	// context of a failed program load succeeded by another attempt
	mapName := eventMapName(proto)
	for _, perfMap := range m.PerfMaps {
		if perfMap.Map.Name == mapName {
			return true
		}
	}
	for _, perfMap := range m.RingBuffers {
		if perfMap.Map.Name == mapName {
			return true
		}
	}

	return false
}

// handlerByProtocol acts as registry holding a temporary reference to a
// `ddebpf.Handler` instance for a given protocol. This is done to simplify the
// usage of this package a little bit, so a call to `events.Configure` can be
// later linked to a call to `events.NewConsumer` without the need to explicitly
// propagate any values. The map is guarded by `handlerMux`.
var handlerByProtocol map[string]<-chan *batch
var handlerMux sync.Mutex

func getHandler(proto string) <-chan *batch {
	handlerMux.Lock()
	defer handlerMux.Unlock()
	if handlerByProtocol == nil {
		return nil
	}

	handler := handlerByProtocol[proto]
	delete(handlerByProtocol, proto)
	return handler
}

func setHandler(proto string, handler <-chan *batch) {
	handlerMux.Lock()
	defer handlerMux.Unlock()
	if handlerByProtocol == nil {
		handlerByProtocol = make(map[string]<-chan *batch)
	}
	handlerByProtocol[proto] = handler
}

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}
