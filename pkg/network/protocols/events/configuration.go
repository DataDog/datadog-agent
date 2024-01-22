// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"os"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultPerfBufferSize controls the amount of memory in bytes used *per CPU*
// allocated for buffering perf event data
var defaultPerfEventBufferSize = 16 * os.Getpagesize()

// defaultPerfHandlerSize controls the size of the go channel that buffers perf
// events (*ddebpf.PerfHandler). All perf events handled by this library have
// fixed size (sizeof(batch_data_t)) which is ~4KB, so by choosing a value of
// 100 we'll be buffering up to ~400KB of data in *Go* heap memory.
const defaultPerfHandlerSize = 100

// Configure a given `*manager.Manager` for event processing
// This essentially instantiates the perf map/ring buffers and configure the
// eBPF maps where events are enqueued.
// Note this must be called *before* manager.InitWithOptions
func Configure(proto string, m *manager.Manager, o *manager.Options) {
	if alreadySetUp(proto, m) {
		return
	}

	onlineCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		onlineCPUs = 96
		log.Error("unable to detect number of CPUs. assuming 96 cores")
	}

	configureBatchMaps(proto, o, onlineCPUs)

	// TODO: add a feature flag so we can optionally disable it
	useRingBuffer := features.HaveMapType(ebpf.RingBuf) == nil
	utils.AddBoolConst(o, useRingBuffer, "use_ring_buffer")

	if useRingBuffer {
		setupPerfRing(proto, m, o, onlineCPUs)
	} else {
		setupPerfMap(proto, m)
	}
}

func setupPerfMap(proto string, m *manager.Manager) {
	handler := ddebpf.NewPerfHandler(defaultPerfHandlerSize)
	mapName := eventMapName(proto)
	pm := &manager.PerfMap{
		Map: manager.Map{Name: mapName},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: defaultPerfEventBufferSize,

			// Our events are already batched on the kernel side, so it's
			// desirable to have Watermark set to 1
			Watermark: 1,

			RecordHandler: handler.RecordHandler,
			LostHandler:   handler.LostHandler,
			RecordGetter:  handler.RecordGetter,
		},
	}
	m.PerfMaps = append(m.PerfMaps, pm)
	setHandler(proto, handler)
}

func setupPerfRing(proto string, m *manager.Manager, o *manager.Options, numCPUs int) {
	handler := ddebpf.NewRingBufferHandler(defaultPerfHandlerSize)
	mapName := eventMapName(proto)
	ringBufferSize := numCPUs * defaultPerfEventBufferSize
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: mapName},
		RingBufferOptions: manager.RingBufferOptions{
			RingBufferSize: ringBufferSize,
			RecordHandler:  handler.RecordHandler,
			RecordGetter:   handler.RecordGetter,
		},
	}

	o.MapSpecEditors[mapName] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(ringBufferSize),
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}

	m.RingBuffers = append(m.RingBuffers, rb)
	setHandler(proto, handler)
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
var handlerByProtocol map[string]ddebpf.EventHandler
var handlerMux sync.Mutex

func getHandler(proto string) ddebpf.EventHandler {
	handlerMux.Lock()
	defer handlerMux.Unlock()
	if handlerByProtocol == nil {
		return nil
	}

	handler := handlerByProtocol[proto]
	delete(handlerByProtocol, proto)
	return handler
}

func setHandler(proto string, handler ddebpf.EventHandler) {
	handlerMux.Lock()
	defer handlerMux.Unlock()
	if handlerByProtocol == nil {
		handlerByProtocol = make(map[string]ddebpf.EventHandler)
	}
	handlerByProtocol[proto] = handler
}
