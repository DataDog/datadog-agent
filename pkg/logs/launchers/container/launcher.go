// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

//nolint:revive // TODO(AML) Fix revive linter
package container

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	//nolint:revive // TODO(AML) Fix revive linter
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// containerSourceTypes are the values of source.Config.Type for which this
// launcher will respond.
var containerSourceTypes = map[string]struct{}{
	"docker":     {},
	"containerd": {},
	"podman":     {},
	"cri-o":      {},
}

// A Launcher starts and stops new tailers for every new containers discovered
// by autodiscovery.
//
// This launcher supports several container runtimes (as defined by
// source.Config.Type), and emulates the old behavior of the kubernetes
// launcher (when LogWhat == LogPods) and docker launcher (when LogWhat ==
// LogContainers).
type Launcher struct {
	// cancel will cause the launcher loop to stop
	cancel context.CancelFunc

	// once the loop stops, this channel will be closed
	stopped chan struct{}

	// sources allows adding new sources to the agent, for child file sources
	// (temporary)
	sources *sourcesPkg.LogSources

	// tailerFactory builds tailers for sources
	tailerFactory tailerfactory.Factory

	// tailers contains the tailer for each source
	tailers map[*sourcesPkg.LogSource]tailerfactory.Tailer

	wmeta optional.Option[workloadmeta.Component]
}

// NewLauncher returns a new launcher
func NewLauncher(sources *sourcesPkg.LogSources, wmeta optional.Option[workloadmeta.Component]) *Launcher {
	launcher := &Launcher{
		sources: sources,
		tailers: make(map[*sourcesPkg.LogSource]tailerfactory.Tailer),
		wmeta:   wmeta,
	}
	return launcher
}

// Start starts the Launcher
//
//nolint:revive // TODO(AML) Fix revive linter
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	// only start this launcher once it's determined that we should be logging containers, and not pods.
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.stopped = make(chan struct{})

	l.tailerFactory = tailerfactory.New(l.sources, pipelineProvider, registry, l.wmeta)
	go l.run(ctx, sourceProvider)
}

// Stop stops the Launcher. This call returns when the launcher has stopped.
func (l *Launcher) Stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
		<-l.stopped
		l.stopped = nil
	}
}

// run is the main loop for this launcher.  It monitors for sources added or
// removed to the agent and starts or stops tailers appropriately.
func (l *Launcher) run(ctx context.Context, sourceProvider launchers.SourceProvider) {
	log.Info("Starting Container launcher")

	addedSources, removedSources := sourceProvider.SubscribeAll()

	for {
		if !l.loop(ctx, addedSources, removedSources) {
			break
		}
	}
}

// loop runs one iteration of the launcher's main loop, and returns true if it should be run again.
func (l *Launcher) loop(ctx context.Context, addedSources, removedSources chan *sourcesPkg.LogSource) bool {
	select {
	case source := <-addedSources:
		l.startSource(source)

	case source := <-removedSources:
		l.stopSource(source)

	case <-ctx.Done():
		l.stop()
		close(l.stopped)
		return false
	}
	return true
}

// startSource starts tailing from a source.
func (l *Launcher) startSource(source *sourcesPkg.LogSource) {
	containerID := source.Config.Identifier

	// if this is not of a supported container type, ignore it
	if _, ok := containerSourceTypes[source.Config.Type]; !ok {
		return
	}

	// sanity check; this should never be true for types in containerSourceTypes
	if containerID == "" {
		log.Warnf("Source %s has no container identifier", source.Name)
		return
	}

	if _, exists := l.tailers[source]; exists {
		return
	}

	tailer, err := l.tailerFactory.MakeTailer(source)
	if err != nil {
		source.Status.Error(err)
		return
	}

	err = tailer.Start()
	if err != nil {
		source.Status.Error(err)
		return
	}
	source.AddInput(source.Config.Identifier)

	l.tailers[source] = tailer
}

// stopSource stops tailing from a source.
func (l *Launcher) stopSource(source *sourcesPkg.LogSource) {
	if tailer, exists := l.tailers[source]; exists {
		tailer.Stop()
		delete(l.tailers, source)
	}
}

// stop stops the launcher's run loop, returning when all running tailers have
// stopped.
func (l *Launcher) stop() {
	count := 0
	stopper := startstop.NewParallelStopper()
	for _, tailer := range l.tailers {
		count++
		stopper.Add(tailer)
	}
	log.Infof("Stopping container launcher - stopping %d tailers", count)
	stopper.Stop()
	log.Info("Stopping container launcher")

	l.tailers = make(map[*sources.LogSource]tailerfactory.Tailer)
}
