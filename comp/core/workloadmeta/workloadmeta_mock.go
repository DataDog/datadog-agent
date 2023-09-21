// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package workloadmeta

// team: container-integrations

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
)

const (
	mockSource = Source("mockSource")
)

// store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type workloadmetamock struct {
	log    log.Component
	config config.Component

	mu    sync.RWMutex
	store map[Kind]map[string]Entity
}

func newWorkloadMetaMock(deps dependencies) Mock {

	mock := &workloadmetamock{
		log:    deps.Log,
		config: deps.Config,
		store:  make(map[Kind]map[string]Entity),
	}

	return mock
}

func (w *workloadmetamock) GetContainer(id string) (*Container, error) {
	entity, err := w.getEntityByKind(KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Container), nil
}

// ListContainers returns metadata about all known containers.
func (w *workloadmetamock) ListContainers() []*Container {
	entities := w.listEntitiesByKind(KindContainer)

	// Not very efficient
	containers := make([]*Container, 0, len(entities))
	for _, entity := range entities {
		containers = append(containers, entity.(*Container))
	}

	return containers
}

// ListContainersWithFilter returns metadata about the containers that pass the given filter.
func (w *workloadmetamock) ListContainersWithFilter(filter ContainerFilterFunc) []*Container {
	var res []*Container

	for _, container := range w.ListContainers() {
		if filter(container) {
			res = append(res, container)
		}
	}

	return res
}

// GetProcess implements workloadmetamock#GetProcess.
func (w *workloadmetamock) GetProcess(pid int32) (*Process, error) {
	id := string(pid)
	entity, err := w.getEntityByKind(KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Process), nil
}

// ListProcesses implements workloadmetamock#ListProcesses.
func (w *workloadmetamock) ListProcesses() []*Process {
	entities := w.listEntitiesByKind(KindProcess)

	processes := make([]*Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*Process))
	}

	return processes
}

// ListProcessesWithFilter implements workloadmetamock#ListProcessesWithFilter.
func (w *workloadmetamock) ListProcessesWithFilter(filter ProcessFilterFunc) []*Process {
	var res []*Process

	for _, process := range w.ListProcesses() {
		if filter(process) {
			res = append(res, process)
		}
	}

	return res
}

// GetKubernetesPod returns metadata about a Kubernetes pod.
func (w *workloadmetamock) GetKubernetesPod(id string) (*KubernetesPod, error) {
	entity, err := w.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesPod), nil
}

// GetKubernetesPodForContainer returns a KubernetesPod that contains the
// specified containerID.
func (w *workloadmetamock) GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	containerEntities, ok := w.store[KindContainer]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	containerEntity, ok := containerEntities[containerID]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	container := containerEntity.(*Container)
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

	return pod.(*KubernetesPod), nil
}

// GetKubernetesNode returns metadata about a Kubernetes node.
func (w *workloadmetamock) GetKubernetesNode(id string) (*KubernetesNode, error) {
	entity, err := w.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNode), nil
}

// GetKubernetesDeployment implements Component#GetKubernetesDeployment
func (w *workloadmetamock) GetKubernetesDeployment(id string) (*KubernetesDeployment, error) {
	entity, err := w.getEntityByKind(KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesDeployment), nil
}

// GetECSTask returns metadata about an ECS task.
func (w *workloadmetamock) GetECSTask(id string) (*ECSTask, error) {
	entity, err := w.getEntityByKind(KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ECSTask), nil
}

// ListImages implements workloadmetamock#ListImages
func (w *workloadmetamock) ListImages() []*ContainerImageMetadata {
	entities := w.listEntitiesByKind(KindContainerImageMetadata)

	images := make([]*ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements workloadmetamock#GetImage
func (w *workloadmetamock) GetImage(id string) (*ContainerImageMetadata, error) {
	entity, err := w.getEntityByKind(KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ContainerImageMetadata), nil
}

// Set sets an entity in the store.
func (w *workloadmetamock) Set(entity Entity) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entityID := entity.GetID()

	if _, ok := w.store[entityID.Kind]; !ok {
		w.store[entityID.Kind] = make(map[string]Entity)
	}

	w.store[entityID.Kind][entityID.ID] = entity
}

// Unset removes an entity from the store.
func (w *workloadmetamock) Unset(entity Entity) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entityID := entity.GetID()
	if _, ok := w.store[entityID.Kind]; !ok {
		return
	}

	delete(w.store[entityID.Kind], entityID.ID)
}

// Start is not implemented in the testing store.
func (w *workloadmetamock) Start(ctx context.Context) {
	panic("not implemented")
}

// Subscribe is not implemented in the testing store.
func (w *workloadmetamock) Subscribe(name string, _ SubscriberPriority, filter *Filter) chan EventBundle {
	panic("not implemented")
}

// Unsubscribe is not implemented in the testing store.
func (w *workloadmetamock) Unsubscribe(ch chan EventBundle) {
	panic("not implemented")
}

// Notify is not implemented in the testing store.
func (w *workloadmetamock) Notify(events []CollectorEvent) {
	panic("not implemented")
}

// Dump is not implemented in the testing store.
func (w *workloadmetamock) Dump(verbose bool) WorkloadDumpResponse {
	panic("not implemented")
}

// Reset is not implemented in the testing store.
func (w *workloadmetamock) Reset(newEntities []Entity, source Source) {
	panic("not implemented")
}

func (w *workloadmetamock) ResetProcesses(newProcesses []Entity, source Source) {
	panic("not implemented")
}

func (w *workloadmetamock) getEntityByKind(kind Kind, id string) (Entity, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	entity, ok := entitiesOfKind[id]
	if !ok {
		return nil, errors.NewNotFound(id)
	}

	return entity, nil
}

func (w *workloadmetamock) listEntitiesByKind(kind Kind) []Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil
	}

	entities := make([]Entity, 0, len(entitiesOfKind))
	for _, entity := range entitiesOfKind {
		entities = append(entities, entity)
	}

	return entities
}

// GetConfig returns a ConfigReader for the internal injected config
func (w *workloadmetamock) GetConfig() pkgconfig.ConfigReader {
	return w.config
}

// MockStore is a store designed to be used in unit tests
type workloadMetaMockV2 struct {
	*workloadmeta
}

// newWorkloadMetaMockV2 returns a Mock
func newWorkloadMetaMockV2(deps dependencies) Mock {
	wm := newWorkloadMeta(deps)

	w := &workloadMetaMockV2{
		workloadmeta: wm.(*workloadmeta),
	}

	return w

}

// Notify overrides store to allow for synchronous event processing
func (w *workloadMetaMockV2) Notify(events []CollectorEvent) {
	w.handleEvents(events)
}

// SetEntity generates a Set event
func (w *workloadMetaMockV2) Set(e Entity) {
	w.Notify([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: mockSource,
			Entity: e,
		},
	})
}

// GetConfig returns a ConfigReader for the internal injected config
func (w *workloadMetaMockV2) GetConfig() pkgconfig.ConfigReader {
	return w.config
}

// UnsetEntity generates an Unset event
func (w *workloadMetaMockV2) Unset(e Entity) {
	w.Notify([]CollectorEvent{
		{
			Type:   EventTypeUnset,
			Source: mockSource,
			Entity: e,
		},
	})
}
