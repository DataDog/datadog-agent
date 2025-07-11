// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/telemetry"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	retryCollectorInitialInterval = 1 * time.Second
	retryCollectorMaxInterval     = 30 * time.Second
	pullCollectorInterval         = 5 * time.Second
	maxCollectorPullTime          = 1 * time.Minute
	eventBundleChTimeout          = 1 * time.Second
	eventChBufferSize             = 50
)

type subscriber struct {
	name     string
	priority wmdef.SubscriberPriority
	ch       chan wmdef.EventBundle
	filter   *wmdef.Filter
}

// start starts the workload metadata store.
func (w *workloadmeta) start(ctx context.Context) {
	go func() {
		health := health.RegisterLiveness("workloadmeta-store")
		for {
			select {
			case <-health.C:

			case evs := <-w.eventCh:
				w.handleEvents(evs)

			case <-ctx.Done():
				err := health.Deregister()
				if err != nil {
					w.log.Warnf("error de-registering health check: %s", err)
				}
				return
			}
		}
	}()

	go func() {
		if err := w.startCandidatesWithRetry(ctx); err != nil {
			w.log.Errorf("error starting collectors: %s", err)
		}
	}()

	go func() {
		pullTicker := time.NewTicker(pullCollectorInterval)
		health := health.RegisterLiveness("workloadmeta-puller")

		// Start a pull immediately to fill the store without waiting for the
		// next tick.
		w.pull(ctx)
		w.updateCollectorStatus(wmdef.CollectorsInitialized)

		for {
			select {
			case <-health.C:

			case <-pullTicker.C:
				w.pull(ctx)

			case <-ctx.Done():
				pullTicker.Stop()

				err := health.Deregister()
				if err != nil {
					w.log.Warnf("error de-registering health check: %s", err)
				}

				w.unsubscribeAll()

				w.log.Infof("stopped workloadmeta store")

				return
			}
		}
	}()

	w.log.Info("workloadmeta store initialized successfully")
}

// Subscribe returns a channel where workload metadata events will be streamed
// as they happen. On first subscription, it will also generate an EventTypeSet
// event for each entity present in the store that matches filter, unless the
// filter type is EventTypeUnset.
func (w *workloadmeta) Subscribe(name string, priority wmdef.SubscriberPriority, filter *wmdef.Filter) chan wmdef.EventBundle {
	// ch needs to be buffered since we'll send it events before the
	// subscriber has the chance to start receiving from it. if it's
	// unbuffered, it'll deadlock.
	sub := subscriber{
		name:     name,
		priority: priority,
		ch:       make(chan wmdef.EventBundle, 1),
		filter:   filter,
	}

	var events []wmdef.Event

	// lock the store and only unlock once the subscriber has been added,
	// otherwise the subscriber can lose events. adding the subscriber
	// before locking the store can cause deadlocks if the store sends
	// events before this function returns.
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	if filter == nil || filter.EventType() != wmdef.EventTypeUnset {
		for kind, entitiesOfKind := range w.store {
			if !sub.filter.MatchKind(kind) {
				continue
			}

			for _, cachedEntity := range entitiesOfKind {
				entity := cachedEntity.get(sub.filter.Source())
				if entity != nil && sub.filter.MatchEntity(&entity) {
					events = append(events, wmdef.Event{
						Type:   wmdef.EventTypeSet,
						Entity: entity,
					})
				}
			}
		}

		// sort events by kind and ID for deterministic ordering
		sort.Slice(events, func(i, j int) bool {
			a := events[i].Entity.GetID()
			b := events[j].Entity.GetID()

			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}

			return a.ID < b.ID
		})
	}

	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	// notifyChannel should not wait when doing the first subscription, as
	// the subscriber is not ready to receive events yet
	w.notifyChannel(sub.name, sub.ch, events, false)

	w.subscribers = append(w.subscribers, sub)
	sort.SliceStable(w.subscribers, func(i, j int) bool {
		return w.subscribers[i].priority < w.subscribers[j].priority
	})

	telemetry.Subscribers.Inc()

	return sub.ch
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (w *workloadmeta) Unsubscribe(ch chan wmdef.EventBundle) {
	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	for i, sub := range w.subscribers {
		if sub.ch == ch {
			w.subscribers = slices.Delete(w.subscribers, i, i+1)
			telemetry.Subscribers.Dec()
			close(ch)
			return
		}
	}
}

// GetContainer implements Store#GetContainer.
func (w *workloadmeta) GetContainer(id string) (*wmdef.Container, error) {
	entity, err := w.getEntityByKind(wmdef.KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.Container), nil
}

// ListContainers implements Store#ListContainers.
func (w *workloadmeta) ListContainers() []*wmdef.Container {
	return w.ListContainersWithFilter(nil)
}

// ListContainersWithFilter implements Store#ListContainersWithFilter
func (w *workloadmeta) ListContainersWithFilter(filter wmdef.EntityFilterFunc[*wmdef.Container]) []*wmdef.Container {
	entities := w.listEntitiesByKind(wmdef.KindContainer)

	// Not very efficient
	containers := make([]*wmdef.Container, 0, len(entities))
	for _, entity := range entities {
		container := entity.(*wmdef.Container)

		if filter == nil || filter(container) {
			containers = append(containers, container)
		}
	}

	return containers
}

// GetKubernetesPod implements Store#GetKubernetesPod
func (w *workloadmeta) GetKubernetesPod(id string) (*wmdef.KubernetesPod, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesPod), nil
}

// GetKubernetesPodByName implements Store#GetKubernetesPodByName
func (w *workloadmeta) GetKubernetesPodByName(podName, podNamespace string) (*wmdef.KubernetesPod, error) {
	entities := w.listEntitiesByKind(wmdef.KindKubernetesPod)

	// TODO race condition with statefulsets
	// If a statefulset pod is recreated, the pod name and namespace would be identical, but the pod UID would be
	// different. There is the possibility that the new pod is added to the workloadmeta store before the old one is
	// removed, so there is a chance that the wrong pod is returned.
	for k := range entities {
		entity := entities[k].(*wmdef.KubernetesPod)
		if entity.Name == podName && entity.Namespace == podNamespace {
			return entity, nil
		}
	}

	return nil, errors.NewNotFound(podName)
}

// GetProcess implements Store#GetProcess.
func (w *workloadmeta) GetProcess(pid int32) (*wmdef.Process, error) {
	id := strconv.Itoa(int(pid))

	entity, err := w.getEntityByKind(wmdef.KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.Process), nil
}

// ListProcesses implements Store#ListProcesses.
func (w *workloadmeta) ListProcesses() []*wmdef.Process {
	entities := w.listEntitiesByKind(wmdef.KindProcess)

	processes := make([]*wmdef.Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*wmdef.Process))
	}

	return processes
}

// ListProcessesWithFilter implements Store#ListProcessesWithFilter
func (w *workloadmeta) ListProcessesWithFilter(filter wmdef.EntityFilterFunc[*wmdef.Process]) []*wmdef.Process {
	entities := w.listEntitiesByKind(wmdef.KindProcess)

	processes := make([]*wmdef.Process, 0, len(entities))
	for i := range entities {
		process := entities[i].(*wmdef.Process)

		if filter == nil || filter(process) {
			processes = append(processes, process)
		}
	}

	return processes
}

// GetContainerForProcess implements Store#GetContainerForProcess
func (w *workloadmeta) GetContainerForProcess(processID string) (*wmdef.Container, error) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	processEntities, ok := w.store[wmdef.KindProcess]
	if !ok {
		return nil, errors.NewNotFound(string(wmdef.KindProcess))
	}

	processEntity, ok := processEntities[processID]
	if !ok {
		return nil, errors.NewNotFound(processID)
	}

	process := processEntity.cached.(*wmdef.Process)
	if process.Owner == nil || process.Owner.Kind != wmdef.KindContainer {
		return nil, errors.NewNotFound(processID)
	}

	containerEntities, ok := w.store[wmdef.KindContainer]
	if !ok {
		return nil, errors.NewNotFound(process.Owner.ID)
	}

	container, ok := containerEntities[process.Owner.ID]
	if !ok {
		return nil, errors.NewNotFound(process.Owner.ID)
	}

	return container.cached.(*wmdef.Container), nil
}

// GetKubernetesPodForContainer implements Store#GetKubernetesPodForContainer
func (w *workloadmeta) GetKubernetesPodForContainer(containerID string) (*wmdef.KubernetesPod, error) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	containerEntities, ok := w.store[wmdef.KindContainer]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	containerEntity, ok := containerEntities[containerID]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	container := containerEntity.cached.(*wmdef.Container)
	if container.Owner == nil || container.Owner.Kind != wmdef.KindKubernetesPod {
		return nil, errors.NewNotFound(containerID)
	}

	podEntities, ok := w.store[wmdef.KindKubernetesPod]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	pod, ok := podEntities[container.Owner.ID]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	return pod.cached.(*wmdef.KubernetesPod), nil
}

// GetKubernetesDeployment implements Store#GetKubernetesDeployment
func (w *workloadmeta) GetKubernetesDeployment(id string) (*wmdef.KubernetesDeployment, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesDeployment), nil
}

// ListECSTasks implements Store#ListECSTasks
func (w *workloadmeta) ListECSTasks() []*wmdef.ECSTask {
	entities := w.listEntitiesByKind(wmdef.KindECSTask)

	tasks := make([]*wmdef.ECSTask, 0, len(entities))
	for _, entity := range entities {
		task := entity.(*wmdef.ECSTask)
		tasks = append(tasks, task)
	}

	return tasks
}

// GetECSTask implements Store#GetECSTask
func (w *workloadmeta) GetECSTask(id string) (*wmdef.ECSTask, error) {
	entity, err := w.getEntityByKind(wmdef.KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.ECSTask), nil
}

// ListImages implements Store#ListImages
func (w *workloadmeta) ListImages() []*wmdef.ContainerImageMetadata {
	entities := w.listEntitiesByKind(wmdef.KindContainerImageMetadata)

	images := make([]*wmdef.ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*wmdef.ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements Store#GetImage
func (w *workloadmeta) GetImage(id string) (*wmdef.ContainerImageMetadata, error) {
	entity, err := w.getEntityByKind(wmdef.KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.ContainerImageMetadata), nil
}

// GetKubernetesMetadata implements Store#GetKubernetesMetadata.
func (w *workloadmeta) GetKubernetesMetadata(id wmdef.KubeMetadataEntityID) (*wmdef.KubernetesMetadata, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesMetadata, string(id))
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesMetadata), nil
}

// ListKubernetesMetadata implements Store#ListKubernetesMetadata.
func (w *workloadmeta) ListKubernetesMetadata(filterFunc wmdef.EntityFilterFunc[*wmdef.KubernetesMetadata]) []*wmdef.KubernetesMetadata {
	entities := w.listEntitiesByKind(wmdef.KindKubernetesMetadata)

	var metadata []*wmdef.KubernetesMetadata
	for _, entity := range entities {
		kubeMetadata := entity.(*wmdef.KubernetesMetadata)

		if filterFunc == nil || filterFunc(kubeMetadata) {
			metadata = append(metadata, kubeMetadata)
		}
	}

	return metadata
}

// GetGPU implements Store#GetGPU.
func (w *workloadmeta) GetGPU(id string) (*wmdef.GPU, error) {
	entity, err := w.getEntityByKind(wmdef.KindGPU, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.GPU), nil
}

// ListGPUs implements Store#ListGPUs.
func (w *workloadmeta) ListGPUs() []*wmdef.GPU {
	entities := w.listEntitiesByKind(wmdef.KindGPU)

	gpuList := make([]*wmdef.GPU, 0, len(entities))
	for i := range entities {
		gpuList = append(gpuList, entities[i].(*wmdef.GPU))
	}

	return gpuList
}

// Notify implements Store#Notify
func (w *workloadmeta) Notify(events []wmdef.CollectorEvent) {
	if len(events) > 0 {
		w.eventCh <- events
	}
}

// ResetProcesses implements Store#ResetProcesses
func (w *workloadmeta) ResetProcesses(newProcesses []wmdef.Entity, source wmdef.Source) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	var events []wmdef.CollectorEvent
	newProcessEntities := classifyByKindAndID(newProcesses)[wmdef.KindProcess]

	processStore := w.store[wmdef.KindProcess]
	// Remove outdated stored processes
	for ID, storedProcess := range processStore {
		if newP, found := newProcessEntities[ID]; !found || storedProcess.cached != newP {
			events = append(events, wmdef.CollectorEvent{
				Type:   wmdef.EventTypeUnset,
				Source: source,
				Entity: storedProcess.cached,
			})
		}
	}

	// Add new processes
	for _, newP := range newProcesses {
		events = append(events, wmdef.CollectorEvent{
			Type:   wmdef.EventTypeSet,
			Source: source,
			Entity: newP,
		})
	}

	w.Notify(events)
}

// Reset implements Store#Reset
func (w *workloadmeta) Reset(newEntities []wmdef.Entity, source wmdef.Source) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	var events []wmdef.CollectorEvent

	// Create a "set" event for each entity that need to be in the store.
	// The store will takes care of not sending events for entities that already
	// exist and haven't changed.
	for _, newEntity := range newEntities {
		events = append(events, wmdef.CollectorEvent{
			Type:   wmdef.EventTypeSet,
			Source: source,
			Entity: newEntity,
		})
	}

	// Create an "unset" event for each entity that needs to be deleted
	newEntitiesByKindAndID := classifyByKindAndID(newEntities)
	for kind, storedEntitiesOfKind := range w.store {
		initialEntitiesOfKind, found := newEntitiesByKindAndID[kind]
		if !found {
			initialEntitiesOfKind = make(map[string]wmdef.Entity)
		}

		for ID, storedEntity := range storedEntitiesOfKind {
			if _, found = initialEntitiesOfKind[ID]; !found {
				events = append(events, wmdef.CollectorEvent{
					Type:   wmdef.EventTypeUnset,
					Source: source,
					Entity: storedEntity.cached,
				})
			}
		}
	}

	w.Notify(events)
}

// IsInitialized: If startCandidates is run at least once, return true.
func (w *workloadmeta) IsInitialized() bool {
	w.collectorMut.RLock()
	defer w.collectorMut.RUnlock()
	return w.collectorsInitialized == wmdef.CollectorsInitialized
}

func (w *workloadmeta) validatePushEvents(events []wmdef.Event) error {
	for _, event := range events {
		if event.Type != wmdef.EventTypeSet && event.Type != wmdef.EventTypeUnset {
			return fmt.Errorf("unsupported Event type: only EventTypeSet and EventTypeUnset types are allowed for push events")
		}
	}
	return nil
}

// Push implements Store#Push
func (w *workloadmeta) Push(source wmdef.Source, events ...wmdef.Event) error {
	err := w.validatePushEvents(events)
	if err != nil {
		return err
	}

	collectorEvents := make([]wmdef.CollectorEvent, len(events))
	for index, event := range events {
		collectorEvents[index] = wmdef.CollectorEvent{
			Type:   event.Type,
			Source: source,
			Entity: event.Entity,
		}
	}

	w.Notify(collectorEvents)
	return nil
}

func (w *workloadmeta) startCandidatesWithRetry(ctx context.Context) error {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = retryCollectorInitialInterval
	expBackoff.MaxInterval = retryCollectorMaxInterval
	expBackoff.MaxElapsedTime = 0 // Don't stop trying

	if len(w.candidates) == 0 {
		// TODO: this should actually probably just be an error?
		return nil
	}

	return backoff.Retry(func() error {
		select {
		case <-ctx.Done():
			return &backoff.PermanentError{Err: fmt.Errorf("stopped before all collectors were able to start: %v", w.candidates)}
		default:
		}

		if w.startCandidates(ctx) {
			return nil
		}

		return fmt.Errorf("some collectors failed to start. Will retry")
	}, expBackoff)
}

func (w *workloadmeta) startCandidates(ctx context.Context) bool {
	w.collectorMut.Lock()
	defer w.collectorMut.Unlock()

	for id, c := range w.candidates {
		err := c.Start(ctx, w)

		// Leave candidates that returned a retriable error to be
		// re-started in the next tick
		if err != nil && retry.IsErrWillRetry(err) {
			w.log.Debugf("workloadmeta collector %q could not start, but will retry. error: %s", id, err)
			continue
		}

		// Store successfully started collectors for future reference
		if err == nil {
			w.log.Infof("workloadmeta collector %q started successfully", id)
			w.collectors[id] = c
		} else {
			w.log.Infof("workloadmeta collector %q could not start. error: %s", id, err)
		}

		// Remove non-retriable and successfully started collectors
		// from the list of candidates so they're not retried in the
		// next tick
		delete(w.candidates, id)
	}
	if w.collectorsInitialized == wmdef.CollectorsNotStarted {
		w.collectorsInitialized = wmdef.CollectorsStarting
	}
	return len(w.candidates) == 0
}

func (w *workloadmeta) updateCollectorStatus(status wmdef.CollectorStatus) {
	w.collectorMut.Lock()
	defer w.collectorMut.Unlock()
	if w.collectorsInitialized == wmdef.CollectorsInitialized {
		return // already initialized
	} else if status == wmdef.CollectorsInitialized && w.collectorsInitialized == wmdef.CollectorsNotStarted {
		return // no collectors to initialize yet
	}
	w.collectorsInitialized = status
}

func (w *workloadmeta) pull(ctx context.Context) {
	w.collectorMut.RLock()
	defer w.collectorMut.RUnlock()

	for id, c := range w.collectors {
		w.ongoingPullsMut.Lock()
		ongoingPullStartTime := w.ongoingPulls[id]
		alreadyRunning := !ongoingPullStartTime.IsZero()
		if alreadyRunning {
			timeRunning := time.Since(ongoingPullStartTime)
			if timeRunning > maxCollectorPullTime {
				w.log.Errorf("collector %q has been running for too long (%d seconds)", id, timeRunning/time.Second)
			} else {
				w.log.Debugf("collector %q is still running. Will not pull again for now", id)
			}
			w.ongoingPullsMut.Unlock()
			continue
		}

		w.ongoingPulls[id] = time.Now()
		w.ongoingPullsMut.Unlock()

		// Run each pull in its own separate goroutine to reduce
		// latency and unlock the main goroutine to do other work.
		go func(id string, c wmdef.Collector) {
			pullCtx, pullCancel := context.WithTimeout(ctx, maxCollectorPullTime)
			defer pullCancel()

			err := c.Pull(pullCtx)
			if err != nil {
				w.log.Warnf("error pulling from collector %q: %s", id, err.Error())
				telemetry.PullErrors.Inc(id)
			}

			w.ongoingPullsMut.Lock()
			pullDuration := time.Since(w.ongoingPulls[id])
			telemetry.PullDuration.Observe(pullDuration.Seconds(), id)
			w.ongoingPulls[id] = time.Time{}
			w.ongoingPullsMut.Unlock()
		}(id, c)
	}
}

func (w *workloadmeta) handleEvents(evs []wmdef.CollectorEvent) {
	w.storeMut.Lock()
	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	filteredEvents := make(map[subscriber][]wmdef.Event, len(w.subscribers))
	for _, sub := range w.subscribers {
		filteredEvents[sub] = make([]wmdef.Event, 0, len(evs))
	}

	for _, ev := range evs {
		entityID := ev.Entity.GetID()

		telemetry.EventsReceived.Inc(string(entityID.Kind), string(ev.Source))

		entitiesOfKind, ok := w.store[entityID.Kind]
		if !ok {
			w.store[entityID.Kind] = make(map[string]*cachedEntity)
			entitiesOfKind = w.store[entityID.Kind]
		}

		cachedEntity, ok := entitiesOfKind[entityID.ID]

		switch ev.Type {
		case wmdef.EventTypeSet:
			if !ok {
				entitiesOfKind[entityID.ID] = newCachedEntity()
				cachedEntity = entitiesOfKind[entityID.ID]
			}

			found, changed := cachedEntity.set(ev.Source, ev.Entity)

			if !found {
				telemetry.StoredEntities.Inc(
					string(entityID.Kind),
					string(ev.Source),
				)
			}

			if !changed {
				continue
			}
		case wmdef.EventTypeUnset:
			// if the entity we're trying to remove was not
			// present in the store, skip generating any
			// events for downstream subscribers. this
			// fixes an issue where collectors emit
			// EventTypeUnset events for pause containers
			// they never emitted a EventTypeSet for.

			if !ok {
				continue
			}

			_, sourceOk := cachedEntity.sources[ev.Source]
			if !sourceOk {
				continue
			}

			// keep a copy of cachedEntity before removing sources,
			// as we may need to merge it later
			c := cachedEntity
			cachedEntity = c.copy()

			c.unset(ev.Source)

			telemetry.StoredEntities.Dec(
				string(entityID.Kind),
				string(ev.Source),
			)

			if len(c.sources) == 0 {
				delete(entitiesOfKind, entityID.ID)
			}
		default:
			w.log.Errorf("cannot handle event of type %d. event dump: %+v", ev.Type, ev)
		}

		for _, sub := range w.subscribers {
			filter := sub.filter

			// Notice that we cannot call filter.MatchEntity() here because
			// the entity included in the event might be incomplete if it's
			// an unset event. Some collectors only send the entity ID in
			// unset events, for example. The call to filter.MatchEntity()
			// is done below using the cached entity.
			if !filter.MatchSource(ev.Source) || !filter.MatchEventType(ev.Type) {
				// event should be filtered out because it
				// doesn't match the filter.
				continue
			}

			var isEventTypeSet bool
			if ev.Type == wmdef.EventTypeSet {
				isEventTypeSet = true
			} else if filter.Source() == wmdef.SourceAll {
				isEventTypeSet = len(cachedEntity.sources) > 1
			} else {
				isEventTypeSet = false
			}

			entity := cachedEntity.get(filter.Source())
			if !filter.MatchEntity(&entity) {
				continue
			}

			if isEventTypeSet {
				filteredEvents[sub] = append(filteredEvents[sub], wmdef.Event{
					Type:   wmdef.EventTypeSet,
					Entity: entity,
				})
			} else {
				entity = entity.DeepCopy()
				err := entity.Merge(ev.Entity)
				if err != nil {
					w.log.Errorf("cannot merge %+v into %+v: %s", entity, ev.Entity, err)
					continue
				}

				filteredEvents[sub] = append(filteredEvents[sub], wmdef.Event{
					Type:   wmdef.EventTypeUnset,
					Entity: entity,
				})
			}
		}
	}

	// unlock the store before notifying subscribers, as they might need to
	// read it for related entities (such as a pod's containers) while they
	// process an event.
	w.storeMut.Unlock()

	for _, sub := range w.subscribers {
		if evs, found := filteredEvents[sub]; found {
			if len(evs) == 0 {
				continue
			}

			w.notifyChannel(sub.name, sub.ch, evs, true)
		}
	}
}

func (w *workloadmeta) getEntityByKind(kind wmdef.Kind, id string) (wmdef.Entity, error) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil, errors.NewNotFound(string(kind))
	}

	entity, ok := entitiesOfKind[id]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	return entity.cached, nil
}

func (w *workloadmeta) listEntitiesByKind(kind wmdef.Kind) []wmdef.Entity {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil
	}

	entities := make([]wmdef.Entity, 0, len(entitiesOfKind))
	for _, entity := range entitiesOfKind {
		entities = append(entities, entity.cached)
	}

	return entities
}

func (w *workloadmeta) unsubscribeAll() {
	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	for _, sub := range w.subscribers {
		close(sub.ch)
	}

	w.subscribers = nil

	telemetry.Subscribers.Set(0)
}

// call holding lock on w.subscribersMut
func (w *workloadmeta) notifyChannel(name string, ch chan wmdef.EventBundle, events []wmdef.Event, wait bool) {
	bundle := wmdef.EventBundle{
		Ch:     make(chan struct{}),
		Events: events,
	}
	ch <- bundle

	if wait {
		select {
		case <-bundle.Ch:
			telemetry.NotificationsSent.Inc(name, telemetry.StatusSuccess)
		case <-time.After(eventBundleChTimeout):
			log.Warnf("collector %q did not close the event bundle channel in time, continuing with downstream collectors. bundle size: %d", name, len(bundle.Events))
			telemetry.NotificationsSent.Inc(name, telemetry.StatusError)
		}
	}
}

func classifyByKindAndID(entities []wmdef.Entity) map[wmdef.Kind]map[string]wmdef.Entity {
	res := make(map[wmdef.Kind]map[string]wmdef.Entity)

	for _, entity := range entities {
		kind := entity.GetID().Kind
		entityID := entity.GetID().ID

		_, found := res[kind]
		if !found {
			res[kind] = make(map[string]wmdef.Entity)
		}
		res[kind][entityID] = entity
	}

	return res
}
