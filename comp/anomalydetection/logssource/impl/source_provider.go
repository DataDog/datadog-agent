// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// wmetaStore is the subset of workloadmeta.Component used by sourceProvider.
// Keeping the dependency narrow makes the type testable with a simple stub.
type wmetaStore interface {
	Subscribe(name string, priority workloadmeta.SubscriberPriority, filter *workloadmeta.Filter) chan workloadmeta.EventBundle
	Unsubscribe(ch chan workloadmeta.EventBundle)
	GetContainer(id string) (*workloadmeta.Container, error)
}

// sourceProvider translates workloadmeta container events into LogSources,
// publishing them to the provided LogSources instance.
type sourceProvider struct {
	wmeta       wmetaStore
	logSources  *sources.LogSources
	pauseFilter workloadfilter.FilterBundle

	mu            sync.Mutex
	activeSources map[string]*sources.LogSource // keyed by container ID
	suppressedIDs map[string]struct{}           // container IDs with an active AD source

	stopped sync.WaitGroup
}

func newSourceProvider(wmeta wmetaStore, logSources *sources.LogSources, pauseFilter workloadfilter.FilterBundle) *sourceProvider {
	return &sourceProvider{
		wmeta:         wmeta,
		logSources:    logSources,
		pauseFilter:   pauseFilter,
		activeSources: make(map[string]*sources.LogSource),
		suppressedIDs: make(map[string]struct{}),
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
	// For containerd/non-Docker runtimes on Kubernetes, wait for kubelet
	// enrichment before emitting a source. The containerd collector publishes
	// container entities before the kubelet collector attaches the
	// KubernetesPod owner; if we emit the source now, the container
	// launcher's tailerfactory fails with "cannot find pod for container" and
	// has no retry. Workloadmeta re-notifies subscribers with the merged
	// entity once kubelet enriches it, so skipping here is safe.
	// Docker containers are tailed via the socket and have no pod owner, so
	// this guard does not apply to them.
	if c.Runtime != workloadmeta.ContainerRuntimeDocker {
		if c.Owner == nil || c.Owner.Kind != workloadmeta.KindKubernetesPod {
			return
		}
	}

	runtimeSource := string(c.Runtime)

	sp.mu.Lock()
	if _, exists := sp.activeSources[c.EntityID.ID]; exists {
		sp.mu.Unlock()
		return // idempotent: already tracked
	}
	if _, suppressed := sp.suppressedIDs[c.EntityID.ID]; suppressed {
		sp.mu.Unlock()
		return // an AD source already owns this container; skip generic source
	}
	src := sources.NewLogSource(c.EntityID.ID, &logsconfig.LogsConfig{
		Type:       string(c.Runtime),
		Source:     runtimeSource, // enables msg.Origin.Source() for log filter matching
		Identifier: c.EntityID.ID,
	})
	sp.activeSources[c.EntityID.ID] = src
	sp.mu.Unlock()

	sp.logSources.AddSource(src)

	// Guard against suppressIdentifier arriving in the window between sp.mu.Unlock()
	// and AddSource: if activeSources no longer holds this entry, the AD source
	// already attempted (and missed) RemoveSource, so undo the add here.
	sp.mu.Lock()
	_, stillTracked := sp.activeSources[c.EntityID.ID]
	sp.mu.Unlock()
	if !stillTracked {
		sp.logSources.RemoveSource(src)
		return
	}

	log.Infof("[observer/logssource] added container source: %s (runtime=%s)", c.Image.ShortName, c.Runtime)
}

func (sp *sourceProvider) handleUnset(c *workloadmeta.Container) {
	sp.mu.Lock()
	src, exists := sp.activeSources[c.EntityID.ID]
	if !exists {
		sp.mu.Unlock()
		return
	}
	delete(sp.activeSources, c.EntityID.ID)
	sp.mu.Unlock()

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

// isAgentContainer returns true for containers that appear to be Datadog agents,
// preventing the observer from tailing its own logs — which would create an
// unbounded feedback loop (agent logs → observer → more agent logs → ...).
func isAgentContainer(c *workloadmeta.Container) bool {
	return strings.Contains(strings.ToLower(c.Image.ShortName), "agent")
}

// isAgentContainerID returns true when workloadmeta identifies containerID as
// an Agent container. Lookup failures are treated as non-Agent so AD collection
// is not accidentally disabled while workloadmeta is still catching up.
func (sp *sourceProvider) isAgentContainerID(containerID string) bool {
	if sp == nil || sp.wmeta == nil || containerID == "" {
		return false
	}
	container, err := sp.wmeta.GetContainer(containerID)
	if err != nil {
		return false
	}
	return isAgentContainer(container)
}

// suppressIdentifier marks containerID as owned by an AD source.
// If a generic source for that container is already active, it is removed so
// the AD source becomes the sole collector for this container.
// Must be called before adding the AD source to LogSources.
func (sp *sourceProvider) suppressIdentifier(containerID string) {
	sp.mu.Lock()
	sp.suppressedIDs[containerID] = struct{}{}
	evicted, exists := sp.activeSources[containerID]
	if exists {
		delete(sp.activeSources, containerID)
	}
	sp.mu.Unlock()

	if exists {
		sp.logSources.RemoveSource(evicted)
		log.Debugf("[observer/logssource] removed generic container source %s: AD source takes priority", containerID)
	}
}

// unsuppressIdentifier releases the suppression for containerID and immediately
// re-creates the generic source if the container is still running in workloadmeta.
// The workloadmeta Set event that originally triggered the generic source was
// consumed before the AD source suppressed it; without the re-add here the
// container would go untailed until a future state change fires a new Set event.
// Must be called after removing the AD source from LogSources.
func (sp *sourceProvider) unsuppressIdentifier(containerID string) {
	sp.mu.Lock()
	delete(sp.suppressedIDs, containerID)
	sp.mu.Unlock()

	if container, err := sp.wmeta.GetContainer(containerID); err == nil {
		sp.handleSet(container)
	}
}
