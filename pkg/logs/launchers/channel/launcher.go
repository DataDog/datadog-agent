// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package channel provides channel-based log launchers
package channel

import (
	"context"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/channel"
)

// Launcher reacts to sources with Config.Type = Channel, by creating a tailer
// reading from that channel.
//
// WARNING: removing a source does not stop the corresponding tailer.
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          chan *sources.LogSource
	sourcesDone      chan struct{}
	// tailers is only accessed by the run goroutine. Other goroutines get a copy through tailersReq.
	tailers    []*tailer.Tailer
	tailersReq chan chan []*tailer.Tailer
	stop       chan struct{}
	stopOnce   sync.Once
}

// NewLauncher returns an initialized Launcher
func NewLauncher() *Launcher {
	return &Launcher{
		sourcesDone: make(chan struct{}),
		tailersReq:  make(chan chan []*tailer.Tailer),
		stop:        make(chan struct{}),
	}
}

// Start starts the launcher.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.sources = sourceProvider.GetAddedForType(config.StringChannelType, l.sourcesDone)
	go l.run()
}

func (l *Launcher) startNewTailer(source *sources.LogSource) {
	outputChan := l.pipelineProvider.NextPipelineChan()
	tailer := tailer.NewTailer(source, source.Config.Channel, outputChan)
	l.tailers = append(l.tailers, tailer)
	tailer.Start()
}

func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			l.startNewTailer(source)
			source.Status().Success()
		case reply := <-l.tailersReq:
			// A ready sources channel is chosen at random with this case, so
			// start tailers for sources already being sent before replying.
			l.startPendingTailers()
			reply <- slices.Clone(l.tailers)
		case <-l.stop:
			return
		}
	}
}

// startPendingTailers starts a tailer for every source already being sent.
// It does not wait for sources that are not yet sent.
func (l *Launcher) startPendingTailers() {
	for {
		select {
		case source := <-l.sources:
			l.startNewTailer(source)
			source.Status().Success()
		default:
			return
		}
	}
}

// Drain flushes every channel tailer into the pipeline, or returns when ctx is
// done. It closes the source channels, so it is only used at shutdown.
func (l *Launcher) Drain(ctx context.Context) {
	reply := make(chan []*tailer.Tailer, 1)
	select {
	case l.tailersReq <- reply:
	case <-ctx.Done():
		return
	}
	for _, tailer := range <-reply {
		tailer.WaitFlush(ctx)
	}
}

// Stop waits for any running tailer to be flushed.
func (l *Launcher) Stop() {
	l.stopOnce.Do(func() {
		l.Drain(context.Background())
		close(l.sourcesDone)
		l.stop <- struct{}{}
	})
}
