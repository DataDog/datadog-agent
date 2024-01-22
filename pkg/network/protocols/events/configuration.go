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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// handlerByProtocol holds a temporary reference to a `ddebpf.PerfHandler`
// instance for a given protocol. This is done to simplify the usage of this
// package a little bit, so a call to `events.Configure` can be later linked
// to a call to `events.NewConsumer` without the need to explicitly propagate
// any values. The map is guarded by `handlerMux`.
var handlerByProtocol map[string]*ddebpf.PerfHandler
var handlerMux sync.Mutex

// Configure event processing
// Must be called *before* manager.InitWithOptions
func Configure(proto string, m *manager.Manager, o *manager.Options) {
	setupPerfMap(proto, m)
	onlineCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		onlineCPUs = 96
		log.Error("unable to detect number of CPUs. assuming 96 cores")
	}

	if o.MapSpecEditors == nil {
		o.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	o.MapSpecEditors[proto+batchMapSuffix] = manager.MapSpecEditor{
		MaxEntries: uint32(onlineCPUs * batchPagesPerCPU),
		EditorFlag: manager.EditMaxEntries,
	}
}

func getHandler(proto string) *ddebpf.PerfHandler {
	handlerMux.Lock()
	defer handlerMux.Unlock()
	if handlerByProtocol == nil {
		return nil
	}

	handler := handlerByProtocol[proto]
	delete(handlerByProtocol, proto)
	return handler
}

func setupPerfMap(proto string, m *manager.Manager) {
	// check if we already have configured this perf map
	// this can happen in the context of a failed program load succeeded by another attempt
	mapName := proto + eventsMapSuffix
	for _, perfMap := range m.PerfMaps {
		if perfMap.Map.Name == mapName {
			return
		}
	}

	handler := ddebpf.NewPerfHandler(100)
	pm := &manager.PerfMap{
		Map: manager.Map{Name: mapName},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: 16 * os.Getpagesize(),
			Watermark:          1,
			RecordHandler:      handler.RecordHandler,
			LostHandler:        handler.LostHandler,
			RecordGetter:       handler.RecordGetter,
		},
	}
	m.PerfMaps = append(m.PerfMaps, pm)

	handlerMux.Lock()
	if handlerByProtocol == nil {
		handlerByProtocol = make(map[string]*ddebpf.PerfHandler)
	}
	handlerByProtocol[proto] = handler
	handlerMux.Unlock()
}
