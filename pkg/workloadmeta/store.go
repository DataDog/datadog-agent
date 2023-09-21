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
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
	"github.com/cenkalti/backoff"
)

var (
	globalStore Store
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

// store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type store struct {
	storeMut sync.RWMutex
	store    map[Kind]map[string]*cachedEntity // store[entity.Kind][entity.ID] = &cachedEntity{}

	subscribersMut sync.RWMutex
	subscribers    []subscriber

	collectorMut sync.RWMutex
	candidates   map[string]Collector
	collectors   map[string]Collector

	eventCh chan []CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started
}

var _ Store = &store{}

// NewStore creates a new workload metadata store, building a new instance of
// each collector in the catalog. Call Start to start the store and its
// collectors.
func NewStore(catalog CollectorCatalog) Store {
	return newStore(catalog)
}

func newStore(catalog CollectorCatalog) *store {
	candidates := make(map[string]Collector)
	for id, c := range catalog {
		candidates[id] = c()
	}

	return &store{
		store:        make(map[Kind]map[string]*cachedEntity),
		candidates:   candidates,
		collectors:   make(map[string]Collector),
		eventCh:      make(chan []CollectorEvent, eventChBufferSize),
		ongoingPulls: make(map[string]time.Time),
	}
}

// Start starts the workload metadata store.
func (s *store) Start(ctx context.Context) {
	go func() {
		health := health.RegisterLiveness("workloadmeta-store")
		for {
			select {
			case <-health.C:

			case evs := <-s.eventCh:
				s.handleEvents(evs)

			case <-ctx.Done():
				err := health.Deregister()
				if err != nil {
					log.Warnf("error de-registering health check: %s", err)
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
		s.pull(ctx)

		for {
			select {
			case <-health.C:

			case <-pullTicker.C:
				s.pull(ctx)

			case <-ctx.Done():
				pullTicker.Stop()

				err := health.Deregister()
				if err != nil {
					log.Warnf("error de-registering health check: %s", err)
				}

				s.unsubscribeAll()

				log.Infof("stopped workloadmeta store")

				return
			}
		}
	}()

	go func() {
		if err := s.startCandidatesWithRetry(ctx); err != nil {
			log.Errorf("error starting collectors: %s", err)
		}
	}()

	log.Info("workloadmeta store initialized successfully")
}

// Subscribe returns a channel where workload metadata events will be streamed
// as they happen. On first subscription, it will also generate an EventTypeSet
// event for each entity present in the store that matches filter, unless the
// filter type is EventTypeUnset.
func (s *store) Subscribe(name string, priority SubscriberPriority, filter *Filter) chan EventBundle {
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
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	if filter == nil || (filter != nil && filter.EventType() != EventTypeUnset) {
		for kind, entitiesOfKind := range s.store {
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

	// notifyChannel should not wait when doing the first subscription, as
	// the subscriber is not ready to receive events yet
	s.notifyChannel(sub.name, sub.ch, events, false)

	s.subscribersMut.Lock()
	defer s.subscribersMut.Unlock()

	s.subscribers = append(s.subscribers, sub)
	sort.SliceStable(s.subscribers, func(i, j int) bool {
		return s.subscribers[i].priority < s.subscribers[j].priority
	})

	telemetry.Subscribers.Inc()

	return sub.ch
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *store) Unsubscribe(ch chan EventBundle) {
	s.subscribersMut.Lock()
	defer s.subscribersMut.Unlock()

	for i, sub := range s.subscribers {
		if sub.ch == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			telemetry.Subscribers.Dec()
			close(ch)
			return
		}
	}
}

// GetContainer implements Store#GetContainer.
func (s *store) GetContainer(id string) (*Container, error) {
	entity, err := s.getEntityByKind(KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Container), nil
}

// ListContainers implements Store#ListContainers.
func (s *store) ListContainers() []*Container {
	return s.ListContainersWithFilter(nil)
}

// ListContainersWithFilter implements Store#ListContainersWithFilter
func (s *store) ListContainersWithFilter(filter ContainerFilterFunc) []*Container {
	entities := s.listEntitiesByKind(KindContainer)

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
func (s *store) GetKubernetesPod(id string) (*KubernetesPod, error) {
	entity, err := s.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesPod), nil
}

// GetProcess implements Store#GetProcess.
func (s *store) GetProcess(pid int32) (*Process, error) {
	id := strconv.Itoa(int(pid))

	entity, err := s.getEntityByKind(KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Process), nil
}

// ListProcesses implements Store#ListProcesses.
func (s *store) ListProcesses() []*Process {
	entities := s.listEntitiesByKind(KindProcess)

	processes := make([]*Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*Process))
	}

	return processes
}

// ListProcessesWithFilter implements Store#ListProcessesWithFilter
func (s *store) ListProcessesWithFilter(filter ProcessFilterFunc) []*Process {
	entities := s.listEntitiesByKind(KindProcess)

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
func (s *store) GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error) {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	containerEntities, ok := s.store[KindContainer]
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

	podEntities, ok := s.store[KindKubernetesPod]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	pod, ok := podEntities[container.Owner.ID]
	if !ok {
		return nil, errors.NewNotFound(container.Owner.ID)
	}

	return pod.cached.(*KubernetesPod), nil
}

// GetKubernetesNode implements Store#GetKubernetesNode
func (s *store) GetKubernetesNode(id string) (*KubernetesNode, error) {
	entity, err := s.getEntityByKind(KindKubernetesNode, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNode), nil
}

// GetKubernetesDeployment implements Store#GetKubernetesDeployment
func (s *store) GetKubernetesDeployment(id string) (*KubernetesDeployment, error) {
	entity, err := s.getEntityByKind(KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesDeployment), nil
}

// GetECSTask implements Store#GetECSTask
func (s *store) GetECSTask(id string) (*ECSTask, error) {
	entity, err := s.getEntityByKind(KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ECSTask), nil
}

// ListImages implements Store#ListImages
func (s *store) ListImages() []*ContainerImageMetadata {
	entities := s.listEntitiesByKind(KindContainerImageMetadata)

	images := make([]*ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements Store#GetImage
func (s *store) GetImage(id string) (*ContainerImageMetadata, error) {
	entity, err := s.getEntityByKind(KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ContainerImageMetadata), nil
}

// Notify implements Store#Notify
func (s *store) Notify(events []CollectorEvent) {
	if len(events) > 0 {
		s.eventCh <- events
	}
}

// ResetProcesses implements Store#ResetProcesses
func (s *store) ResetProcesses(newProcesses []Entity, source Source) {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	var events []CollectorEvent
	newProcessEntities := classifyByKindAndID(newProcesses)[KindProcess]

	processStore := s.store[KindProcess]
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

	s.Notify(events)
}

// Reset implements Store#Reset
func (s *store) Reset(newEntities []Entity, source Source) {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

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
	for kind, storedEntitiesOfKind := range s.store {
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

	s.Notify(events)
}

func (s *store) startCandidatesWithRetry(ctx context.Context) error {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = retryCollectorInitialInterval
	expBackoff.MaxInterval = retryCollectorMaxInterval
	expBackoff.MaxElapsedTime = 0 // Don't stop trying

	return backoff.Retry(func() error {
		select {
		case <-ctx.Done():
			return &backoff.PermanentError{Err: fmt.Errorf("stopped before all collectors were able to start")}
		default:
		}

		if s.startCandidates(ctx) {
			return nil
		}

		return fmt.Errorf("some collectors failed to start. Will retry")
	}, expBackoff)
}

func (s *store) startCandidates(ctx context.Context) bool {
	s.collectorMut.Lock()
	defer s.collectorMut.Unlock()

	for id, c := range s.candidates {
		err := c.Start(ctx, s)

		// Leave candidates that returned a retriable error to be
		// re-started in the next tick
		if err != nil && retry.IsErrWillRetry(err) {
			log.Debugf("workloadmeta collector %q could not start, but will retry. error: %s", id, err)
			continue
		}

		// Store successfully started collectors for future reference
		if err == nil {
			log.Infof("workloadmeta collector %q started successfully", id)
			s.collectors[id] = c
		} else {
			log.Infof("workloadmeta collector %q could not start. error: %s", id, err)
		}

		// Remove non-retriable and successfully started collectors
		// from the list of candidates so they're not retried in the
		// next tick
		delete(s.candidates, id)
	}

	return len(s.candidates) == 0
}

func (s *store) pull(ctx context.Context) {
	s.collectorMut.RLock()
	defer s.collectorMut.RUnlock()

	for id, c := range s.collectors {
		s.ongoingPullsMut.Lock()
		ongoingPullStartTime := s.ongoingPulls[id]
		alreadyRunning := !ongoingPullStartTime.IsZero()
		if alreadyRunning {
			timeRunning := time.Since(ongoingPullStartTime)
			if timeRunning > maxCollectorPullTime {
				log.Errorf("collector %q has been running for too long (%d seconds)", id, timeRunning/time.Second)
			} else {
				log.Debugf("collector %q is still running. Will not pull again for now", id)
			}
			s.ongoingPullsMut.Unlock()
			continue
		} else {
			s.ongoingPulls[id] = time.Now()
			s.ongoingPullsMut.Unlock()
		}

		// Run each pull in its own separate goroutine to reduce
		// latency and unlock the main goroutine to do other work.
		go func(id string, c Collector) {
			pullCtx, pullCancel := context.WithTimeout(ctx, maxCollectorPullTime)
			defer pullCancel()

			err := c.Pull(pullCtx)
			if err != nil {
				log.Warnf("error pulling from collector %q: %s", id, err.Error())
				telemetry.PullErrors.Inc(id)
			}

			s.ongoingPullsMut.Lock()
			pullDuration := time.Since(s.ongoingPulls[id])
			telemetry.PullDuration.Observe(pullDuration.Seconds(), id)
			s.ongoingPulls[id] = time.Time{}
			s.ongoingPullsMut.Unlock()
		}(id, c)
	}
}

func (s *store) handleEvents(evs []CollectorEvent) {
	s.storeMut.Lock()
	s.subscribersMut.RLock()

	filteredEvents := make(map[subscriber][]Event, len(s.subscribers))
	for _, sub := range s.subscribers {
		filteredEvents[sub] = make([]Event, 0, len(evs))
	}

	for _, ev := range evs {
		entityID := ev.Entity.GetID()

		telemetry.EventsReceived.Inc(string(entityID.Kind), string(ev.Source))

		entitiesOfKind, ok := s.store[entityID.Kind]
		if !ok {
			s.store[entityID.Kind] = make(map[string]*cachedEntity)
			entitiesOfKind = s.store[entityID.Kind]
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
			log.Errorf("cannot handle event of type %d. event dump: %+v", ev.Type, ev)
		}

		for _, sub := range s.subscribers {
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
					log.Errorf("cannot merge %+v into %+v: %s", entity, ev.Entity, err)
					continue
				}

				filteredEvents[sub] = append(filteredEvents[sub], Event{
					Type:   EventTypeUnset,
					Entity: entity,
				})
			}
		}
	}

	s.subscribersMut.RUnlock()

	// unlock the store before notifying subscribers, as they might need to
	// read it for related entities (such as a pod's containers) while they
	// process an event.
	s.storeMut.Unlock()

	for sub, evs := range filteredEvents {
		if len(evs) == 0 {
			continue
		}

		s.notifyChannel(sub.name, sub.ch, evs, true)
	}
}

func (s *store) getEntityByKind(kind Kind, id string) (Entity, error) {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	entitiesOfKind, ok := s.store[kind]
	if !ok {
		return nil, errors.NewNotFound(string(kind))
	}

	entity, ok := entitiesOfKind[id]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	return entity.cached, nil
}

func (s *store) listEntitiesByKind(kind Kind) []Entity {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	entitiesOfKind, ok := s.store[kind]
	if !ok {
		return nil
	}

	entities := make([]Entity, 0, len(entitiesOfKind))
	for _, entity := range entitiesOfKind {
		entities = append(entities, entity.cached)
	}

	return entities
}

func (s *store) unsubscribeAll() {
	s.subscribersMut.Lock()
	defer s.subscribersMut.Unlock()

	for _, sub := range s.subscribers {
		close(sub.ch)
	}

	s.subscribers = nil

	telemetry.Subscribers.Set(0)
}

func (s *store) notifyChannel(name string, ch chan EventBundle, events []Event, wait bool) {
	bundle := EventBundle{
		Ch:     make(chan struct{}),
		Events: events,
	}
	s.subscribersMut.Lock()
	ch <- bundle
	s.subscribersMut.Unlock()

	if wait {
		timer := time.NewTimer(eventBundleChTimeout)

		select {
		case <-bundle.Ch:
			timer.Stop()
			telemetry.NotificationsSent.Inc(name, telemetry.StatusSuccess)
		case <-timer.C:
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

// CreateGlobalStore creates a workloadmeta store, sets it as the default
// global one, and returns it. Start() needs to be called before any data
// collection happens.
func CreateGlobalStore(catalog CollectorCatalog) Store {
	if globalStore != nil {
		panic("global workloadmeta store already set, should only happen once")
	}

	globalStore = NewStore(catalog)

	return globalStore
}

// GetGlobalStore returns a global instance of the workloadmeta store. It does
// not create one if it's not already set (see CreateGlobalStore) and returns
// nil in that case.
func GetGlobalStore() Store {
	return globalStore
}

// ResetGlobalStore resets the global store back to nil. This is useful in lifecycle
// tests that start and stop parts of the agent multiple times.
func ResetGlobalStore() {
	globalStore = nil
}
