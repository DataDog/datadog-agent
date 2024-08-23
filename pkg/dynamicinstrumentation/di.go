// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dynamicinstrumentation provides the main entrypoint into running the
// dynamic instrumentation for Go product
package dynamicinstrumentation

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diconfig"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ebpf"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/uploader"
)

// GoDI is the central controller representation of the Dynamic Instrumentation
// implementation for Go services
type GoDI struct {
	cm diconfig.ConfigManager

	lu uploader.LogUploader
	du uploader.DiagnosticUploader

	processEvent ditypes.EventCallback
	Close        func()

	stats GoDIStats
}

// GoDIStats is used to track various metrics relevant to the health of the
// Dynamic Instrumentation process
type GoDIStats struct {
	PIDEventsCreatedCount   map[uint32]uint64 // pid : count
	ProbeEventsCreatedCount map[string]uint64 // probeID : count
}

func newGoDIStats() GoDIStats {
	return GoDIStats{
		PIDEventsCreatedCount:   make(map[uint32]uint64),
		ProbeEventsCreatedCount: make(map[string]uint64),
	}
}

// DIOptions is used to configure the running Dynamic Instrumentation process
type DIOptions struct {
	Offline bool

	ProbesFilePath   string
	SnapshotOutput   string
	DiagnosticOutput string

	ditypes.EventCallback
}

// RunDynamicInstrumentation is the main entry point into running the Dynamic
// Instrumentation project for Go.
func RunDynamicInstrumentation(opts *DIOptions) (*GoDI, error) {
	var goDI *GoDI

	ebpf.SetupRingBufferAndHeaders()

	if opts.Offline {
		cm, err := diconfig.NewFileConfigManager(opts.ProbesFilePath)
		if err != nil {
			return nil, fmt.Errorf("couldn't create new file config manager: %w", err)
		}
		lu, err := uploader.NewOfflineLogSerializer(opts.SnapshotOutput)
		if err != nil {
			return nil, fmt.Errorf("couldn't create new offline log serializer: %w", err)
		}
		du, err := uploader.NewOfflineDiagnosticSerializer(diagnostics.Diagnostics, opts.DiagnosticOutput)
		if err != nil {
			return nil, fmt.Errorf("couldn't create new offline diagnostic serializer: %w", err)
		}
		goDI = &GoDI{
			cm:    cm,
			lu:    lu,
			du:    du,
			stats: newGoDIStats(),
		}
	} else {
		cm, err := diconfig.NewRCConfigManager()
		if err != nil {
			return nil, fmt.Errorf("couldn't create new RC config manager: %w", err)
		}
		goDI = &GoDI{
			cm:    cm,
			lu:    uploader.NewLogUploader(),
			du:    uploader.NewDiagnosticUploader(),
			stats: newGoDIStats(),
		}
	}
	if opts.EventCallback != nil {
		goDI.processEvent = opts.EventCallback
	} else {
		goDI.processEvent = goDI.uploadSnapshot
	}

	closeRingbuffer, err := goDI.startRingbufferConsumer()
	if err != nil {
		return nil, fmt.Errorf("couldn't set up new ringbuffer consumer: %w", err)
	}

	goDI.Close = func() {
		goDI.cm.Stop()
		closeRingbuffer()
	}

	return goDI, nil
}

func (goDI *GoDI) printSnapshot(event *ditypes.DIEvent) {
	if event == nil {
		return
	}
	procInfo := goDI.cm.GetProcInfos()[event.PID]
	diLog := uploader.NewDILog(procInfo, event)

	var bs []byte
	var err error

	if diLog != nil {
		bs, err = json.MarshalIndent(diLog, "", " ")
	} else {
		bs, err = json.MarshalIndent(event, "", " ")
	}

	if err != nil {
		log.Info(err)
	}
	log.Debug(string(bs))
}

func (goDI *GoDI) uploadSnapshot(event *ditypes.DIEvent) {
	goDI.printSnapshot(event)
	procInfo := goDI.cm.GetProcInfos()[event.PID]
	diLog := uploader.NewDILog(procInfo, event)
	if diLog != nil {
		goDI.lu.Enqueue(diLog)
	}
}

// GetStats returns the maps of various statitics for
// runtime health of dynamic instrumentation
func (goDI *GoDI) GetStats() GoDIStats {
	return goDI.stats
}
