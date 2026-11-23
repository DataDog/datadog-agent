// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package windowsevent launches tailers for the Windows event log
package windowsevent

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	publishermetadatacachedef "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	publishermetadatacache "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"
)

type tailer interface {
	Start()
	startstop.Stoppable
	Identifier() string
}

// Launcher is in charge of starting and stopping windows event logs tailers
type Launcher struct {
	sources                chan *sources.LogSource
	sourcesDone            chan struct{}
	pipelineProvider       pipeline.Provider
	registry               auditor.Registry
	tailers                map[string]tailer
	pendingSources         map[string]*sources.LogSource
	sourceGenerations      map[string]uint64
	nextGeneration         atomic.Uint64
	stop                   chan struct{}
	publisherMetadataCache publishermetadatacachedef.Component
	stopOnce               sync.Once
}

// NewLauncher returns a new Launcher.
func NewLauncher() *Launcher {
	cache := publishermetadatacache.New(winevtapi.New())
	return &Launcher{
		tailers:                make(map[string]tailer),
		pendingSources:         make(map[string]*sources.LogSource),
		sourceGenerations:      make(map[string]uint64),
		sourcesDone:            make(chan struct{}),
		stop:                   make(chan struct{}),
		publisherMetadataCache: cache,
	}
}

// Start starts the launcher by setting up Windows event log sources and beginning to tail them.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, _ *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.sources = sourceProvider.GetAddedForType(config.WindowsEventType, l.sourcesDone)
	l.registry = registry
	availableChannels, err := EnumerateChannels()
	if err != nil {
		log.Debug("Could not list windows event log channels: ", err)
	} else {
		log.Debug("Found available windows event log channels: ", availableChannels)
	}
	go l.run()
}

// run starts new tailers and retries any transiently failed registrations.
func (l *Launcher) run() {
	retryTicker := time.NewTicker(time.Second)
	defer retryTicker.Stop()
	for {
		select {
		case source := <-l.sources:
			sanitized := l.sanitizedConfig(source.Config)
			identifier := windowsevent.Identifier(sanitized.ChannelPath, sanitized.Query)
			if _, exists := l.tailers[identifier]; exists {
				// tailer already setup
				continue
			}
			tailer, err := l.setupTailer(source, sanitized, identifier)
			if err != nil {
				log.Info("Could not set up windows event log tailer (will retry): ", err)
				l.pendingSources[identifier] = source
			} else {
				delete(l.pendingSources, identifier)
				l.tailers[identifier] = tailer
			}
		case <-retryTicker.C:
			for identifier, source := range l.pendingSources {
				if _, exists := l.tailers[identifier]; exists {
					delete(l.pendingSources, identifier)
					continue
				}
				sanitized := l.sanitizedConfig(source.Config)
				tailer, err := l.setupTailer(source, sanitized, identifier)
				if err != nil {
					log.Debug("Retrying windows event log tailer setup failed: ", err)
					continue
				}
				delete(l.pendingSources, identifier)
				l.tailers[identifier] = tailer
			}
		case <-l.stop:
			return
		}
	}
}

// Stop stops all active tailers
func (l *Launcher) Stop() {
	l.stopOnce.Do(func() {
		close(l.sourcesDone)
		l.stop <- struct{}{}
		stopper := startstop.NewParallelStopper()
		for _, tailer := range l.tailers {
			stopper.Add(tailer)
			delete(l.tailers, tailer.Identifier())
		}
		stopper.Stop()
		if efp, ok := l.pipelineProvider.(*pipeline.EndpointFilteredProvider); ok {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			for identifier, gen := range l.sourceGenerations {
				_ = efp.RetireSource(ctx, identifier, gen)
				delete(l.sourceGenerations, identifier)
			}
		}
		l.publisherMetadataCache.Flush()
	})
}

// sanitizedConfig sets default values for the config
func (l *Launcher) sanitizedConfig(sourceConfig *config.LogsConfig) *windowsevent.Config {
	config := &windowsevent.Config{
		ChannelPath:       sourceConfig.ChannelPath,
		Query:             sourceConfig.Query,
		ProcessRawMessage: sourceConfig.ShouldProcessRawMessage(),
	}
	if config.Query == "" {
		config.Query = "*"
	}
	return config
}

// setupTailer configures and starts a new tailer
func (l *Launcher) setupTailer(source *sources.LogSource, sanitizedConfig *windowsevent.Config, identifier string) (tailer, error) {
	config := &windowsevent.Config{
		ChannelPath:       sanitizedConfig.ChannelPath,
		Query:             sanitizedConfig.Query,
		ProcessRawMessage: sanitizedConfig.ProcessRawMessage,
	}
	outputChan := l.pipelineProvider.NextPipelineChan()
	if efp, ok := l.pipelineProvider.(*pipeline.EndpointFilteredProvider); ok {
		gen := l.nextGeneration.Add(1)
		ch, err := efp.SourceInput(identifier, gen)
		if err != nil {
			return nil, err
		}
		outputChan = ch
		l.sourceGenerations[identifier] = gen
	}
	t := windowsevent.NewTailer(nil, source, config, outputChan, l.registry, l.publisherMetadataCache)
	t.Start()
	return t, nil
}
