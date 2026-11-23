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
	removedSources         chan *sources.LogSource
	sourcesDone            chan struct{}
	removedSourcesDone     chan struct{}
	pipelineProvider       pipeline.Provider
	registry               auditor.Registry
	tailers                map[string]tailer
	activeSources          map[string]*sources.LogSource
	pendingSources         map[string][]*sources.LogSource
	removedSet             map[*sources.LogSource]struct{}
	sourceGenerations      map[string]uint64
	nextGeneration         atomic.Uint64
	stoppingWg             sync.WaitGroup
	stop                   chan struct{}
	publisherMetadataCache publishermetadatacachedef.Component
	stopOnce               sync.Once
}

// NewLauncher returns a new Launcher.
func NewLauncher() *Launcher {
	cache := publishermetadatacache.New(winevtapi.New())
	return &Launcher{
		tailers:                make(map[string]tailer),
		activeSources:          make(map[string]*sources.LogSource),
		pendingSources:         make(map[string][]*sources.LogSource),
		removedSet:             make(map[*sources.LogSource]struct{}),
		sourceGenerations:      make(map[string]uint64),
		sourcesDone:            make(chan struct{}),
		removedSourcesDone:     make(chan struct{}),
		stop:                   make(chan struct{}),
		publisherMetadataCache: cache,
	}
}

// Start starts the launcher by setting up Windows event log sources and beginning to tail them.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, _ *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.sources, l.removedSources = sourceProvider.SubscribeForType(config.WindowsEventType, l.sourcesDone, l.removedSourcesDone)
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
	enqueuePending := func(identifier string, src *sources.LogSource) {
		for _, existing := range l.pendingSources[identifier] {
			if existing == src {
				return
			}
		}
		l.pendingSources[identifier] = append(l.pendingSources[identifier], src)
	}
	removeSource := func(removed *sources.LogSource) {
		l.removedSet[removed] = struct{}{}
		for identifier, queue := range l.pendingSources {
			kept := queue[:0]
			for _, pending := range queue {
				if pending != removed {
					kept = append(kept, pending)
				}
			}
			if len(kept) == 0 {
				delete(l.pendingSources, identifier)
			} else {
				l.pendingSources[identifier] = kept
			}
		}
		for identifier, active := range l.activeSources {
			if active == removed {
				t, hasTailer := l.tailers[identifier]
				delete(l.tailers, identifier)
				delete(l.activeSources, identifier)
				gen, hasGen := l.sourceGenerations[identifier]
				delete(l.sourceGenerations, identifier)
				if hasTailer {
					// Stop and retire off the notification loop so a blocked
					// tailer cannot stall other source adds, removals, or retries.
					efp, isEFP := l.pipelineProvider.(*pipeline.EndpointFilteredProvider)
					l.stoppingWg.Add(1)
					go func(id string, stopTailer tailer, retireGen uint64, retire bool) {
						defer l.stoppingWg.Done()
						stopTailer.Stop()
						if retire && isEFP {
							ctx, cancel := context.WithTimeout(context.Background(), time.Second)
							_ = efp.RetireSource(ctx, id, retireGen)
							cancel()
						}
					}(identifier, t, gen, hasGen)
				}
			}
		}
	}
	for {
		select {
		case source := <-l.sources:
			if _, wasRemoved := l.removedSet[source]; wasRemoved {
				delete(l.removedSet, source)
				continue
			}
			sanitized := l.sanitizedConfig(source.Config)
			identifier := windowsevent.Identifier(sanitized.ChannelPath, sanitized.Query)
			if _, exists := l.tailers[identifier]; exists {
				// Retain additional configured source for this identifier so it
				// can take over if the current owner is removed.
				if l.activeSources[identifier] != source {
					enqueuePending(identifier, source)
				}
				continue
			}
			tailer, err := l.setupTailer(source, sanitized, identifier)
			if err != nil {
				log.Info("Could not set up windows event log tailer (will retry): ", err)
				enqueuePending(identifier, source)
			} else {
				l.tailers[identifier] = tailer
				l.activeSources[identifier] = source
			}
		case removed := <-l.removedSources:
			removeSource(removed)
		case <-retryTicker.C:
			// Drain any pending removals before retrying failed registrations.
		drainRemovals:
			for {
				select {
				case removed := <-l.removedSources:
					removeSource(removed)
				default:
					break drainRemovals
				}
			}
			for identifier, queue := range l.pendingSources {
				if _, exists := l.tailers[identifier]; exists || len(queue) == 0 {
					continue
				}
				source := queue[0]
				sanitized := l.sanitizedConfig(source.Config)
				tailer, err := l.setupTailer(source, sanitized, identifier)
				if err != nil {
					log.Debug("Retrying windows event log tailer setup failed: ", err)
					continue
				}
				if len(queue) == 1 {
					delete(l.pendingSources, identifier)
				} else {
					l.pendingSources[identifier] = queue[1:]
				}
				l.tailers[identifier] = tailer
				l.activeSources[identifier] = source
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
		close(l.removedSourcesDone)
		l.stop <- struct{}{}
		stopper := startstop.NewParallelStopper()
		for _, tailer := range l.tailers {
			stopper.Add(tailer)
			delete(l.tailers, tailer.Identifier())
		}
		stopper.Stop()
		l.stoppingWg.Wait()
		if efp, ok := l.pipelineProvider.(*pipeline.EndpointFilteredProvider); ok {
			clear(l.sourceGenerations)
			_ = efp
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
