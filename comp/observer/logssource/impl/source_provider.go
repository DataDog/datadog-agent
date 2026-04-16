// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package logssourceimpl

import (
	"context"
	"strings"
	"sync"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterutil "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// sourceProvider translates workloadmeta container events into LogSources,
// publishing them to the provided LogSources instance.
type sourceProvider struct {
	wmeta       workloadmeta.Component
	logSources  *sources.LogSources
	pauseFilter workloadfilter.FilterBundle

	mu            sync.Mutex
	activeSources map[string]*sources.LogSource // keyed by container ID

	stopped sync.WaitGroup
}

func newSourceProvider(wmeta workloadmeta.Component, logSources *sources.LogSources, pauseFilter workloadfilter.FilterBundle) *sourceProvider {
	return &sourceProvider{
		wmeta:         wmeta,
		logSources:    logSources,
		pauseFilter:   pauseFilter,
		activeSources: make(map[string]*sources.LogSource),
	}
}

// run subscribes to workloadmeta and processes container events until ctx is cancelled.
// Call wait() after cancelling ctx to ensure no AddSource/RemoveSource calls are in
// flight before stopping the launcher.
func (sp *sourceProvider) run(ctx context.Context) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		Build()
	ch := sp.wmeta.Subscribe("observer-logssource", workloadmeta.NormalPriority, filter)

	sp.stopped.Add(1)
	go func() {
		defer sp.stopped.Done()
		defer sp.wmeta.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-ch:
				if !ok {
					return
				}
				bundle.Acknowledge()
				for _, event := range bundle.Events {
					container, ok := event.Entity.(*workloadmeta.Container)
					if !ok {
						continue
					}
					switch event.Type {
					case workloadmeta.EventTypeSet:
						sp.handleSet(container)
					case workloadmeta.EventTypeUnset:
						sp.handleUnset(container)
					}
				}
			}
		}
	}()
}

func (sp *sourceProvider) handleSet(c *workloadmeta.Container) {
	if !c.State.Running {
		return
	}
	if sp.isPauseContainer(c) || isAgentContainer(c) {
		return
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, exists := sp.activeSources[c.EntityID.ID]; exists {
		return // idempotent: already tracked
	}
	src := sources.NewLogSource(c.EntityID.ID, &logsconfig.LogsConfig{
		Type:       string(c.Runtime),
		Identifier: c.EntityID.ID,
	})
	sp.activeSources[c.EntityID.ID] = src
	sp.logSources.AddSource(src)
}

func (sp *sourceProvider) handleUnset(c *workloadmeta.Container) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	src, exists := sp.activeSources[c.EntityID.ID]
	if !exists {
		return
	}
	delete(sp.activeSources, c.EntityID.ID)
	sp.logSources.RemoveSource(src)
}

// wait blocks until the event loop goroutine has fully exited.
func (sp *sourceProvider) wait() {
	sp.stopped.Wait()
}

// isPauseContainer uses the workloadfilter bundle when available, falling back
// to an image-name heuristic for builds where the filter store is absent.
func (sp *sourceProvider) isPauseContainer(c *workloadmeta.Container) bool {
	if sp.pauseFilter != nil {
		return sp.pauseFilter.IsExcluded(workloadfilterutil.CreateContainer(c, nil))
	}
	return strings.Contains(strings.ToLower(c.Image.ShortName), "pause")
}

func isAgentContainer(c *workloadmeta.Container) bool {
	name := strings.ToLower(c.Image.ShortName)
	return strings.Contains(name, "datadog-agent") || strings.Contains(name, "dd-agent")
}
