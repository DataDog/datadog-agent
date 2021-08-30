// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var globalStore *Store

const (
	retryCollectorInterval = 30 * time.Second
	pullCollectorInterval  = 5 * time.Second
	eventBundleChTimeout   = 1 * time.Second
	eventChBufferSize      = 50
)

type subscriber struct {
	name   string
	ch     chan EventBundle
	filter *Filter
}

// Store contains metadata for workloads.
type Store struct {
	storeMut sync.RWMutex
	store    map[Kind]map[string]Entity

	subscribersMut sync.RWMutex
	subscribers    []subscriber

	candidates map[string]Collector
	collectors map[string]Collector

	eventCh chan []Event
}

// NewStore creates a new workload metadata store. Call Run to start it.
func NewStore() *Store {
	candidates := make(map[string]Collector)
	for id, c := range collectorCatalog {
		candidates[id] = c()
	}

	return &Store{
		store:       make(map[Kind]map[string]Entity),
		subscribers: []subscriber{},

		candidates: candidates,
		collectors: make(map[string]Collector),
		eventCh:    make(chan []Event, eventChBufferSize),
	}
}

// Run starts the workload metadata store.
func (s *Store) Run(ctx context.Context) {
	retryTicker := time.NewTicker(retryCollectorInterval)
	pullTicker := time.NewTicker(pullCollectorInterval)
	health := health.RegisterLiveness("workloadmeta-store")

	// Start collectors immediately
	s.startCandidates(ctx)

	// Start a pull immediately to fill the store without waiting for the
	// next tick.
	pullCtx, pullCancel := context.WithTimeout(ctx, pullCollectorInterval)
	s.pull(pullCtx)

	log.Info("workloadmeta store initialized successfully")

	go func() {
		for {
			select {
			case <-health.C:

			case <-pullTicker.C:
				// pullCtx will always be expired at this point
				// if pullTicker has the same duration as
				// pullCtx, so we cancel just as good practice
				pullCancel()

				pullCtx, pullCancel = context.WithTimeout(ctx, pullCollectorInterval)
				s.pull(pullCtx)

			case evs := <-s.eventCh:
				s.handleEvents(evs)

			case <-retryTicker.C:
				s.startCandidates(ctx)

				if len(s.candidates) == 0 {
					retryTicker.Stop()
				}

			case <-ctx.Done():
				retryTicker.Stop()
				pullTicker.Stop()

				err := health.Deregister()
				if err != nil {
					log.Warnf("error de-registering health check: %s", err)
				}

				return
			}
		}
	}()
}

// Subscribe returns a channel where workload metadata events will be streamed
// as they happen.
func (s *Store) Subscribe(name string, filter *Filter) chan EventBundle {
	// ch needs to be buffered since we'll send it events before the
	// subscriber has the chance to start receiving from it. if it's
	// unbuffered, it'll deadlock.
	sub := subscriber{
		name:   name,
		ch:     make(chan EventBundle, 1),
		filter: filter,
	}

	s.subscribersMut.Lock()
	s.subscribers = append(s.subscribers, sub)
	s.subscribersMut.Unlock()

	var events []Event

	s.storeMut.RLock()
	if len(s.store) > 0 {
		for kind, entitiesOfKind := range s.store {
			if !sub.filter.MatchKind(kind) {
				continue
			}

			// TODO(juliogreff): implement filtering by source once
			// each source has its own separate store. since at the
			// time of writing there's a single source, this should
			// not matter.

			for _, entity := range entitiesOfKind {
				events = append(events, Event{
					// TODO(juliogreff): insert Source here
					// after the above TODO has been
					// addressed.
					Type:   EventTypeSet,
					Entity: entity,
				})
			}
		}
	}
	s.storeMut.RUnlock()

	// notifyChannel should not wait when doing the first subscription, as
	// the subscriber is not ready to receive events yet
	notifyChannel(sub.name, sub.ch, events, false)

	return sub.ch
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *Store) Unsubscribe(ch chan EventBundle) {
	s.subscribersMut.Lock()
	defer s.subscribersMut.Unlock()

	for i, sub := range s.subscribers {
		if sub.ch == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			break
		}
	}

	close(ch)
}

// GetContainer returns metadata about a container.
func (s *Store) GetContainer(id string) (Container, error) {
	var c Container

	entity, err := s.getEntityByKind(KindContainer, id)
	if err != nil {
		return c, err
	}

	c = entity.(Container)

	return c, nil
}

// GetKubernetesPod returns metadata about a Kubernetes pod.
func (s *Store) GetKubernetesPod(id string) (KubernetesPod, error) {
	var p KubernetesPod

	entity, err := s.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return p, err
	}

	p = entity.(KubernetesPod)

	return p, nil
}

// GetECSTask returns metadata about an ECS task.
func (s *Store) GetECSTask(id string) (ECSTask, error) {
	var t ECSTask

	entity, err := s.getEntityByKind(KindECSTask, id)
	if err != nil {
		return t, err
	}

	t = entity.(ECSTask)

	return t, nil
}

// Notify notifies the store with a slice of events.
func (s *Store) Notify(events []Event) {
	if len(events) > 0 {
		s.eventCh <- events
	}
}

func (s *Store) startCandidates(ctx context.Context) {
	// NOTE: s.candidates is not guarded by a mutex as it's only called by
	// the store itself, and the store runs on a single goroutine
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
			log.Info("workloadmeta collector %q could not start. error: %s", id, err)
		}

		// Remove non-retriable and successfully started collectors
		// from the list of candidates so they're not retried in the
		// next tick
		delete(s.candidates, id)
	}
}

func (s *Store) pull(ctx context.Context) {
	// NOTE: s.collectors is not guarded by a mutex as it's only called by
	// the store itself, and the store runs on a single goroutine. If this
	// method is made public in the future, we need to guard it.
	for id, c := range s.collectors {
		// Run each pull in its own separate goroutine to reduce
		// latency and unlock the main goroutine to do other work.
		go func(id string, c Collector) {
			err := c.Pull(ctx)
			if err != nil {
				log.Warnf("error pulling from collector %q: %s", id, err.Error())
			}
		}(id, c)
	}
}

func (s *Store) handleEvents(evs []Event) {
	s.storeMut.Lock()

	for _, ev := range evs {
		meta := ev.Entity.GetID()

		entitiesOfKind, ok := s.store[meta.Kind]
		if !ok {
			s.store[meta.Kind] = make(map[string]Entity)
			entitiesOfKind = s.store[meta.Kind]
		}

		switch ev.Type {
		case EventTypeSet:
			entitiesOfKind[meta.ID] = ev.Entity
		case EventTypeUnset:
			delete(entitiesOfKind, meta.ID)
		default:
			log.Errorf("cannot handle event of type %d. event dump: %+v", ev)
		}
	}

	// unlock the store before notifying subscribers, as they might need to
	// read it for related entities (such as a pod's containers) while they
	// process an event.
	s.storeMut.Unlock()

	// copy the list of subscribers to hold locks for as little as
	// possible, since notifyChannel is a blocking operation.
	s.subscribersMut.RLock()
	subscribers := append([]subscriber{}, s.subscribers...)
	s.subscribersMut.RUnlock()

	for _, sub := range subscribers {
		filter := sub.filter
		filteredEvents := make([]Event, 0, len(evs))

		for _, ev := range evs {
			if filter.Match(ev) {
				filteredEvents = append(filteredEvents, ev)
			}
		}

		notifyChannel(sub.name, sub.ch, filteredEvents, true)
	}
}

func (s *Store) getEntityByKind(kind Kind, id string) (Entity, error) {
	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	entitiesOfKind, ok := s.store[kind]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	entity, ok := entitiesOfKind[id]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	return entity, nil
}

func notifyChannel(name string, ch chan EventBundle, events []Event, wait bool) {
	if len(events) == 0 {
		return
	}

	bundle := EventBundle{
		Ch:     make(chan struct{}),
		Events: events,
	}

	ch <- bundle

	if wait {
		timer := time.NewTimer(eventBundleChTimeout)

		select {
		case <-bundle.Ch:
			timer.Stop()
		case <-timer.C:
			log.Warnf("collector %q did not close the event bundle channel in time, continuing with downstream collectors. bundle dump: %+v", name, bundle)
		}
	}
}

// GetGlobalStore returns a global instance of the workloadmeta store,
// creating one if it doesn't exist. Run() needs to be called before any data
// collection happens.
func GetGlobalStore() *Store {
	if globalStore == nil {
		globalStore = NewStore()
	}

	return globalStore
}
