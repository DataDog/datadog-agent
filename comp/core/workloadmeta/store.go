// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"

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
	priority SubscriberPriority
	ch       chan EventBundle
	filter   *Filter
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
		pullTicker := time.NewTicker(pullCollectorInterval)
		health := health.RegisterLiveness("workloadmeta-puller")

		// Start a pull immediately to fill the store without waiting for the
		// next tick.
		w.pull(ctx)

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

	go func() {
		if err := w.startCandidatesWithRetry(ctx); err != nil {
			w.log.Errorf("error starting collectors: %s", err)
		}
	}()

	w.log.Info("workloadmeta store initialized successfully")
}

// Subscribe returns a channel where workload metadata events will be streamed
// as they happen. On first subscription, it will also generate an EventTypeSet
// event for each entity present in the store that matches filter, unless the
// filter type is EventTypeUnset.
func (w *workloadmeta) Subscribe(name string, priority SubscriberPriority, filter *Filter) chan EventBundle {
	// ch needs to be buffered since we'll send it events before the
	// subscriber has the chance to start receiving from it. if it's
	// unbuffered, it'll deadlock.
	sub := subscriber{
		name:     name,
		priority: priority,
		ch:       make(chan EventBundle, 1),
		filter:   filter,
	}

	var events []Event

	// lock the store and only unlock once the subscriber has been added,
	// otherwise the subscriber can lose events. adding the subscriber
	// before locking the store can cause deadlocks if the store sends
	// events before this function returns.
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	if filter == nil || (filter != nil && filter.EventType() != EventTypeUnset) {
		for kind, entitiesOfKind := range w.store {
			if !sub.filter.MatchKind(kind) {
				continue
			}

			for _, cachedEntity := range entitiesOfKind {
				entity := cachedEntity.get(sub.filter.Source())
				if entity != nil {
					events = append(events, Event{
						Type:   EventTypeSet,
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
func (w *workloadmeta) Unsubscribe(ch chan EventBundle) {
	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	for i, sub := range w.subscribers {
		if sub.ch == ch {
			w.subscribers = append(w.subscribers[:i], w.subscribers[i+1:]...)
			telemetry.Subscribers.Dec()
			close(ch)
			return
		}
	}
}

// GetContainer implements Store#GetContainer.
func (w *workloadmeta) GetContainer(id string) (*Container, error) {
	entity, err := w.getEntityByKind(KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Container), nil
}

// ListContainers implements Store#ListContainers.
func (w *workloadmeta) ListContainers() []*Container {
	return w.ListContainersWithFilter(nil)
}

// ListContainersWithFilter implements Store#ListContainersWithFilter
func (w *workloadmeta) ListContainersWithFilter(filter ContainerFilterFunc) []*Container {
	entities := w.listEntitiesByKind(KindContainer)

	// Not very efficient
	containers := make([]*Container, 0, len(entities))
	for _, entity := range entities {
		container := entity.(*Container)

		if filter == nil || filter(container) {
			containers = append(containers, container)
		}
	}

	return containers
}

// GetKubernetesPod implements Store#GetKubernetesPod
func (w *workloadmeta) GetKubernetesPod(id string) (*KubernetesPod, error) {
	entity, err := w.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesPod), nil
}

// GetKubernetesPodByName implements Store#GetKubernetesPodByName
func (w *workloadmeta) GetKubernetesPodByName(podName, podNamespace string) (*KubernetesPod, error) {
	entities := w.listEntitiesByKind(KindKubernetesPod)

	// TODO race condition with statefulsets
	// If a statefulset pod is recreated, the pod name and namespace would be identical, but the pod UID would be
	// different. There is the possibility that the new pod is added to the workloadmeta store before the old one is
	// removed, so there is a chance that the wrong pod is returned.
	for k := range entities {
		entity := entities[k].(*KubernetesPod)
		if entity.Name == podName && entity.Namespace == podNamespace {
			return entity, nil
		}
	}

	return nil, errors.NewNotFound(podName)
}

// GetProcess implements Store#GetProcess.
func (w *workloadmeta) GetProcess(pid int32) (*Process, error) {
	id := strconv.Itoa(int(pid))

	entity, err := w.getEntityByKind(KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Process), nil
}

// ListProcesses implements Store#ListProcesses.
func (w *workloadmeta) ListProcesses() []*Process {
	entities := w.listEntitiesByKind(KindProcess)

	processes := make([]*Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*Process))
	}

	return processes
}

// ListProcessesWithFilter implements Store#ListProcessesWithFilter
func (w *workloadmeta) ListProcessesWithFilter(filter ProcessFilterFunc) []*Process {
	entities := w.listEntitiesByKind(KindProcess)

	processes := make([]*Process, 0, len(entities))
	for i := range entities {
		process := entities[i].(*Process)

		if filter == nil || filter(process) {
			processes = append(processes, process)
		}
	}

	return processes
}

// GetKubernetesPodForContainer implements Store#GetKubernetesPodForContainer
func (w *workloadmeta) GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	containerEntities, ok := w.store[KindContainer]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	containerEntity, ok := containerEntities[containerID]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	container := containerEntity.cached.(*Container)
	if container.Owner == nil || container.Owner.Kind != KindKubernetesPod {
		return nil, errors.NewNotFound(containerID)
	}

	podEntities, ok := w.store[KindKubernetesPod]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	pod, ok := podEntities[container.Owner.ID]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	return pod.cached.(*KubernetesPod), nil
}

// ListKubernetesNodes implements Store#ListKubernetesNodes
func (w *workloadmeta) ListKubernetesNodes() []*KubernetesNode {
	entities := w.listEntitiesByKind(KindKubernetesNode)

	nodes := make([]*KubernetesNode, 0, len(entities))
	for i := range entities {
		nodes = append(nodes, entities[i].(*KubernetesNode))
	}

	return nodes
}

// GetKubernetesNode implements Store#GetKubernetesNode
func (w *workloadmeta) GetKubernetesNode(id string) (*KubernetesNode, error) {
	entity, err := w.getEntityByKind(KindKubernetesNode, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNode), nil
}

// GetKubernetesDeployment implements Store#GetKubernetesDeployment
func (w *workloadmeta) GetKubernetesDeployment(id string) (*KubernetesDeployment, error) {
	entity, err := w.getEntityByKind(KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesDeployment), nil
}

// GetKubernetesNamespace implements Store#GetKubernetesNamespace
func (w *workloadmeta) GetKubernetesNamespace(id string) (*KubernetesNamespace, error) {
	entity, err := w.getEntityByKind(KindKubernetesNamespace, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNamespace), nil
}

// ListECSTasks implements Store#ListECSTasks
func (w *workloadmeta) ListECSTasks() []*ECSTask {
	entities := w.listEntitiesByKind(KindECSTask)

	tasks := make([]*ECSTask, 0, len(entities))
	for _, entity := range entities {
		task := entity.(*ECSTask)
		tasks = append(tasks, task)
	}

	return tasks
}

// GetECSTask implements Store#GetECSTask
func (w *workloadmeta) GetECSTask(id string) (*ECSTask, error) {
	entity, err := w.getEntityByKind(KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ECSTask), nil
}

// ListImages implements Store#ListImages
func (w *workloadmeta) ListImages() []*ContainerImageMetadata {
	entities := w.listEntitiesByKind(KindContainerImageMetadata)

	images := make([]*ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements Store#GetImage
func (w *workloadmeta) GetImage(id string) (*ContainerImageMetadata, error) {
	entity, err := w.getEntityByKind(KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ContainerImageMetadata), nil
}

// Notify implements Store#Notify
func (w *workloadmeta) Notify(events []CollectorEvent) {
	if len(events) > 0 {
		w.eventCh <- events
	}
}

// ResetProcesses implements Store#ResetProcesses
func (w *workloadmeta) ResetProcesses(newProcesses []Entity, source Source) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	var events []CollectorEvent
	newProcessEntities := classifyByKindAndID(newProcesses)[KindProcess]

	processStore := w.store[KindProcess]
	// Remove outdated stored processes
	for ID, storedProcess := range processStore {
		if newP, found := newProcessEntities[ID]; !found || storedProcess.cached != newP {
			events = append(events, CollectorEvent{
				Type:   EventTypeUnset,
				Source: source,
				Entity: storedProcess.cached,
			})
		}
	}

	// Add new processes
	for _, newP := range newProcesses {
		events = append(events, CollectorEvent{
			Type:   EventTypeSet,
			Source: source,
			Entity: newP,
		})
	}

	w.Notify(events)
}

// Reset implements Store#Reset
func (w *workloadmeta) Reset(newEntities []Entity, source Source) {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	var events []CollectorEvent

	// Create a "set" event for each entity that need to be in the store.
	// The store will takes care of not sending events for entities that already
	// exist and haven't changed.
	for _, newEntity := range newEntities {
		events = append(events, CollectorEvent{
			Type:   EventTypeSet,
			Source: source,
			Entity: newEntity,
		})
	}

	// Create an "unset" event for each entity that needs to be deleted
	newEntitiesByKindAndID := classifyByKindAndID(newEntities)
	for kind, storedEntitiesOfKind := range w.store {
		initialEntitiesOfKind, found := newEntitiesByKindAndID[kind]
		if !found {
			initialEntitiesOfKind = make(map[string]Entity)
		}

		for ID, storedEntity := range storedEntitiesOfKind {
			if _, found = initialEntitiesOfKind[ID]; !found {
				events = append(events, CollectorEvent{
					Type:   EventTypeUnset,
					Source: source,
					Entity: storedEntity.cached,
				})
			}
		}
	}

	w.Notify(events)
}

func (w *workloadmeta) validatePushEvents(events []Event) error {
	for _, event := range events {
		if event.Type != EventTypeSet && event.Type != EventTypeUnset {
			return fmt.Errorf("unsupported Event type: only EventTypeSet and EventTypeUnset types are allowed for push events")
		}
	}
	return nil
}

// Push implements Store#Push
func (w *workloadmeta) Push(source Source, events ...Event) error {
	err := w.validatePushEvents(events)
	if err != nil {
		return err
	}

	collectorEvents := make([]CollectorEvent, len(events))
	for index, event := range events {
		collectorEvents[index] = CollectorEvent{
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

	return len(w.candidates) == 0
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
		go func(id string, c Collector) {
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

func (w *workloadmeta) handleEvents(evs []CollectorEvent) {
	w.storeMut.Lock()
	w.subscribersMut.Lock()
	defer w.subscribersMut.Unlock()

	filteredEvents := make(map[subscriber][]Event, len(w.subscribers))
	for _, sub := range w.subscribers {
		filteredEvents[sub] = make([]Event, 0, len(evs))
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
		case EventTypeSet:
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
		case EventTypeUnset:
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
			if !filter.MatchKind(entityID.Kind) || !filter.MatchSource(ev.Source) || !filter.MatchEventType(ev.Type) {
				// event should be filtered out because it
				// doesn't match the filter
				continue
			}

			var isEventTypeSet bool
			if ev.Type == EventTypeSet {
				isEventTypeSet = true
			} else if filter.Source() == SourceAll {
				isEventTypeSet = len(cachedEntity.sources) > 1
			} else {
				isEventTypeSet = false
			}

			entity := cachedEntity.get(filter.Source())
			if isEventTypeSet {
				filteredEvents[sub] = append(filteredEvents[sub], Event{
					Type:   EventTypeSet,
					Entity: entity,
				})
			} else {
				entity = entity.DeepCopy()
				err := entity.Merge(ev.Entity)
				if err != nil {
					w.log.Errorf("cannot merge %+v into %+v: %s", entity, ev.Entity, err)
					continue
				}

				filteredEvents[sub] = append(filteredEvents[sub], Event{
					Type:   EventTypeUnset,
					Entity: entity,
				})
			}
		}
	}

	// unlock the store before notifying subscribers, as they might need to
	// read it for related entities (such as a pod's containers) while they
	// process an event.
	w.storeMut.Unlock()

	for sub, evs := range filteredEvents {
		if len(evs) == 0 {
			continue
		}

		w.notifyChannel(sub.name, sub.ch, evs, true)
	}
}

func (w *workloadmeta) getEntityByKind(kind Kind, id string) (Entity, error) {
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

func (w *workloadmeta) listEntitiesByKind(kind Kind) []Entity {
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil
	}

	entities := make([]Entity, 0, len(entitiesOfKind))
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
func (w *workloadmeta) notifyChannel(name string, ch chan EventBundle, events []Event, wait bool) {
	bundle := EventBundle{
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

func classifyByKindAndID(entities []Entity) map[Kind]map[string]Entity {
	res := make(map[Kind]map[string]Entity)

	for _, entity := range entities {
		kind := entity.GetID().Kind
		entityID := entity.GetID().ID

		_, found := res[kind]
		if !found {
			res[kind] = make(map[string]Entity)
		}
		res[kind][entityID] = entity
	}

	return res
}
