// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package events

import (
	"os"
	"sync"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/iovisor/gobpf/pkg/cpupossible"
)

var handlerMux sync.Mutex
var handlerByProtocol map[string]*ddebpf.PerfHandler

// Configure event processing
// Must be called *before* manager.InitWithOptions
func Configure(proto string, m *manager.Manager, o *manager.Options) {
	handler := ddebpf.NewPerfHandler(100)
	m.PerfMaps = append(m.PerfMaps, &manager.PerfMap{
		Map: manager.Map{Name: proto + eventsMapSuffix},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: 16 * os.Getpagesize(),
			Watermark:          1,
			RecordHandler:      handler.RecordHandler,
			LostHandler:        handler.LostHandler,
			RecordGetter:       handler.RecordGetter,
		},
	})

	handlerMux.Lock()
	if handlerByProtocol == nil {
		handlerByProtocol = make(map[string]*ddebpf.PerfHandler)
	}
	handlerByProtocol[proto] = handler
	handlerMux.Unlock()

	onlineCPUs, err := cpupossible.Get()
	if err != nil {
		onlineCPUs = make([]uint, 96)
		log.Error("unable to detect number of CPUs. assuming 96 cores")
	}

	if o.MapSpecEditors == nil {
		o.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	o.MapSpecEditors[proto+batchMapSuffix] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(len(onlineCPUs) * batchPagesPerCPU),
		EditorFlag: manager.EditMaxEntries,
	}
}
