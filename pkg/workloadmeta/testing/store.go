// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testing

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Store is a testing store that satisfies the workloadmeta.Store interface.
type Store struct {
	mu    sync.RWMutex
	store map[workloadmeta.Kind]map[string]workloadmeta.Entity
}

var _ workloadmeta.Store = &Store{}

// NewStore creates a new workload metadata store for testing.
func NewStore() *Store {
	return &Store{
		store: make(map[workloadmeta.Kind]map[string]workloadmeta.Entity),
	}
}

// GetContainer returns metadata about a container.
func (s *Store) GetContainer(id string) (*workloadmeta.Container, error) {
	entity, err := s.getEntityByKind(workloadmeta.KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*workloadmeta.Container), nil
}

// ListContainers returns metadata about all known containers.
func (s *Store) ListContainers() ([]*workloadmeta.Container, error) {
	entities, err := s.listEntitiesByKind(workloadmeta.KindContainer)
	if err != nil {
		return nil, err
	}

	// Not very efficient
	containers := make([]*workloadmeta.Container, 0, len(entities))
	for _, entity := range entities {
		containers = append(containers, entity.(*workloadmeta.Container))
	}

	return containers, nil
}

// GetKubernetesPod returns metadata about a Kubernetes pod.
func (s *Store) GetKubernetesPod(id string) (*workloadmeta.KubernetesPod, error) {
	entity, err := s.getEntityByKind(workloadmeta.KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*workloadmeta.KubernetesPod), nil
}

// GetKubernetesPodForContainer returns a KubernetesPod that contains the
// specified containerID.
func (s *Store) GetKubernetesPodForContainer(containerID string) (*workloadmeta.KubernetesPod, error) {
	entities, ok := s.store[workloadmeta.KindKubernetesPod]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	for _, e := range entities {
		pod := e.(*workloadmeta.KubernetesPod)
		for _, podContainer := range pod.Containers {
			if podContainer.ID == containerID {
				return pod, nil
			}
		}
	}

	return nil, errors.NewNotFound(containerID)
}

// GetECSTask returns metadata about an ECS task.
func (s *Store) GetECSTask(id string) (*workloadmeta.ECSTask, error) {
	entity, err := s.getEntityByKind(workloadmeta.KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*workloadmeta.ECSTask), nil
}

// Set sets an entity in the store.
func (s *Store) Set(entity workloadmeta.Entity) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entityID := entity.GetID()

	if _, ok := s.store[entityID.Kind]; !ok {
		s.store[entityID.Kind] = make(map[string]workloadmeta.Entity)
	}

	s.store[entityID.Kind][entityID.ID] = entity
}

// Unset removes an entity from the store.
func (s *Store) Unset(entity workloadmeta.Entity) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entityID := entity.GetID()
	if _, ok := s.store[entityID.Kind]; !ok {
		return
	}

	delete(s.store[entityID.Kind], entityID.ID)
}

// Start is not implemented in the testing store.
func (s *Store) Start(ctx context.Context) {
	panic("not implemented")
}

// Subscribe is not implemented in the testing store.
func (s *Store) Subscribe(name string, _ workloadmeta.SubscriberPriority, filter *workloadmeta.Filter) chan workloadmeta.EventBundle {
	panic("not implemented")
}

// Unsubscribe is not implemented in the testing store.
func (s *Store) Unsubscribe(ch chan workloadmeta.EventBundle) {
	panic("not implemented")
}

// Notify is not implemented in the testing store.
func (s *Store) Notify(events []workloadmeta.CollectorEvent) {
	panic("not implemented")
}

// Dump is not implemented in the testing store.
func (s *Store) Dump(verbose bool) workloadmeta.WorkloadDumpResponse {
	panic("not implemented")
}

func (s *Store) getEntityByKind(kind workloadmeta.Kind, id string) (workloadmeta.Entity, error) {
	entitiesOfKind, ok := s.store[kind]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entity, ok := entitiesOfKind[id]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	return entity, nil
}

func (s *Store) listEntitiesByKind(kind workloadmeta.Kind) ([]workloadmeta.Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entitiesOfKind, ok := s.store[kind]
	if !ok {
		return nil, errors.NewNotFound(string(kind))
	}

	entities := make([]workloadmeta.Entity, 0, len(entitiesOfKind))
	for _, entity := range entitiesOfKind {
		entities = append(entities, entity)
	}

	return entities, nil
}
