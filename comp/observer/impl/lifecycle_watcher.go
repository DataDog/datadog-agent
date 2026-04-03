// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"time"

	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// containerMeta caches container metadata from Set events so it's available
// when the Unset (delete) event arrives — Unset only carries the EntityID.
type containerMeta struct {
	name       string
	image      string
	runtime    string
	finishedAt int64  // from last Set event before Unset
	exitCode   *int64 // from last Set event before Unset (workloadmeta uses int64)
}

// lifecycleWatcher subscribes to workloadmeta container events and pushes
// lifecycle observations (create, start, delete) into the observer pipeline.
type lifecycleWatcher struct {
	wmeta  workloadmetadef.Component
	handle observerdef.Handle
	stopCh chan struct{}
	cache  map[string]containerMeta // keyed by container ID
}

// newLifecycleWatcher creates a new lifecycleWatcher.
func newLifecycleWatcher(wmeta workloadmetadef.Component, handle observerdef.Handle) *lifecycleWatcher {
	return &lifecycleWatcher{
		wmeta:  wmeta,
		handle: handle,
		stopCh: make(chan struct{}),
		cache:  make(map[string]containerMeta),
	}
}

// Start launches a goroutine that subscribes to workloadmeta container events
// and forwards them as lifecycle observations.
func (w *lifecycleWatcher) Start() {
	filter := workloadmetadef.NewFilterBuilder().AddKind(workloadmetadef.KindContainer).Build()
	ch := w.wmeta.Subscribe("observer-lifecycle", workloadmetadef.NormalPriority, filter)

	go func() {
		defer w.wmeta.Unsubscribe(ch)

		for {
			select {
			case <-w.stopCh:
				return
			case eventBundle, ok := <-ch:
				if !ok {
					return
				}
				eventBundle.Acknowledge()

				for _, event := range eventBundle.Events {
					w.processEvent(event)
				}
			}
		}
	}()
}

// Stop signals the watcher goroutine to exit.
func (w *lifecycleWatcher) Stop() {
	close(w.stopCh)
}

// processEvent converts a workloadmeta event into a lifecycle observation.
func (w *lifecycleWatcher) processEvent(event workloadmetadef.Event) {
	switch event.Type {
	case workloadmetadef.EventTypeSet:
		container, ok := event.Entity.(*workloadmetadef.Container)
		if !ok {
			return
		}
		w.handleSetEvent(container)

	case workloadmetadef.EventTypeUnset:
		// Unset only carries EntityID — use cached metadata from the Set event.
		entityID := event.Entity.GetID()
		cached := w.cache[entityID.ID]
		delete(w.cache, entityID.ID)
		var exitCode *int32
		if cached.exitCode != nil {
			ec := int32(*cached.exitCode)
			exitCode = &ec
		}
		ts := cached.finishedAt
		if ts == 0 {
			ts = time.Now().Unix()
		}
		w.handle.ObserveLifecycle(&lifecycleEvent{
			containerID:   entityID.ID,
			eventType:     "delete",
			timestamp:     ts,
			exitCode:      exitCode,
			containerName: cached.name,
			image:         cached.image,
			runtime:       cached.runtime,
		})

	default:
		pkglog.Debugf("lifecycle_watcher: ignoring unknown event type %d", event.Type)
	}
}

// handleSetEvent emits a "start" or "create" lifecycle event from a container entity.
func (w *lifecycleWatcher) handleSetEvent(container *workloadmetadef.Container) {
	var imageName string
	if container.Image.Name != "" {
		imageName = container.Image.Name
	}

	// Cache metadata so delete events have full context.
	w.cache[container.EntityID.ID] = containerMeta{
		name:    container.Name,
		image:   imageName,
		runtime: string(container.Runtime),
	}

	// Cache exit info too — the last Set event before Unset has FinishedAt/ExitCode.
	if !container.State.FinishedAt.IsZero() {
		ts := container.State.FinishedAt.Unix()
		meta := w.cache[container.EntityID.ID]
		meta.finishedAt = ts
		meta.exitCode = container.State.ExitCode
		w.cache[container.EntityID.ID] = meta
	}

	if container.State.Running && !container.State.StartedAt.IsZero() {
		w.handle.ObserveLifecycle(&lifecycleEvent{
			containerID:   container.EntityID.ID,
			eventType:     "start",
			timestamp:     container.State.StartedAt.Unix(),
			containerName: container.Name,
			image:         imageName,
			runtime:       string(container.Runtime),
		})
	} else if !container.State.CreatedAt.IsZero() {
		w.handle.ObserveLifecycle(&lifecycleEvent{
			containerID:   container.EntityID.ID,
			eventType:     "create",
			timestamp:     container.State.CreatedAt.Unix(),
			containerName: container.Name,
			image:         imageName,
			runtime:       string(container.Runtime),
		})
	}
}

// lifecycleEvent implements observerdef.LifecycleView for events produced by
// the lifecycle watcher.
type lifecycleEvent struct {
	containerID   string
	eventType     string // "create", "start", "delete"
	timestamp     int64
	exitCode      *int32
	containerName string
	image         string
	runtime       string
}

// Ensure lifecycleEvent implements observerdef.LifecycleView.
var _ observerdef.LifecycleView = (*lifecycleEvent)(nil)

func (e *lifecycleEvent) GetContainerID() string   { return e.containerID }
func (e *lifecycleEvent) GetEventType() string     { return e.eventType }
func (e *lifecycleEvent) GetTimestampUnix() int64  { return e.timestamp }
func (e *lifecycleEvent) GetExitCode() *int32      { return e.exitCode }
func (e *lifecycleEvent) GetContainerName() string { return e.containerName }
func (e *lifecycleEvent) GetImage() string         { return e.image }
func (e *lifecycleEvent) GetRuntime() string       { return e.runtime }
