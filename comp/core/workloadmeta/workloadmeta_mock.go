// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package workloadmeta

// team: container-platform

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
type workloadMetaMock struct {
	log    log.Component
	config config.Component

	mu             sync.RWMutex
	store          map[Kind]map[string]Entity
	notifiedEvents []CollectorEvent
	eventsChan     chan CollectorEvent
}

func newWorkloadMetaMock(deps dependencies) Mock {

	mock := &workloadMetaMock{
		log:    deps.Log,
		config: deps.Config,
		store:  make(map[Kind]map[string]Entity),
	}

	return mock
}

func (w *workloadMetaMock) GetContainer(id string) (*Container, error) {
	entity, err := w.getEntityByKind(KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Container), nil
}

// ListContainers returns metadata about all known containers.
func (w *workloadMetaMock) ListContainers() []*Container {
	entities := w.listEntitiesByKind(KindContainer)

	// Not very efficient
	containers := make([]*Container, 0, len(entities))
	for _, entity := range entities {
		containers = append(containers, entity.(*Container))
	}

	return containers
}

// ListContainersWithFilter returns metadata about the containers that pass the given filter.
func (w *workloadMetaMock) ListContainersWithFilter(filter ContainerFilterFunc) []*Container {
	var res []*Container

	for _, container := range w.ListContainers() {
		if filter(container) {
			res = append(res, container)
		}
	}

	return res
}

// GetProcess implements workloadMetaMock#GetProcess.
func (w *workloadMetaMock) GetProcess(pid int32) (*Process, error) {
	id := string(pid)
	entity, err := w.getEntityByKind(KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*Process), nil
}

// ListProcesses implements workloadMetaMock#ListProcesses.
func (w *workloadMetaMock) ListProcesses() []*Process {
	entities := w.listEntitiesByKind(KindProcess)

	processes := make([]*Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*Process))
	}

	return processes
}

// ListProcessesWithFilter implements workloadMetaMock#ListProcessesWithFilter.
func (w *workloadMetaMock) ListProcessesWithFilter(filter ProcessFilterFunc) []*Process {
	var res []*Process

	for _, process := range w.ListProcesses() {
		if filter(process) {
			res = append(res, process)
		}
	}

	return res
}

// GetKubernetesPod returns metadata about a Kubernetes pod.
func (w *workloadMetaMock) GetKubernetesPod(id string) (*KubernetesPod, error) {
	entity, err := w.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesPod), nil
}

// GetKubernetesPodForContainer returns a KubernetesPod that contains the
// specified containerID.
func (w *workloadMetaMock) GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error) {
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

// GetKubernetesPodByName implements Store#GetKubernetesPodByName
func (w *workloadMetaMock) GetKubernetesPodByName(podName, podNamespace string) (*KubernetesPod, error) {
	entities := w.listEntitiesByKind(KindKubernetesPod)

	// Not very efficient
	for k := range entities {
		entity := entities[k].(*KubernetesPod)
		if entity.Name == podName && entity.Namespace == podNamespace {
			return entity, nil
		}
	}

	return nil, errors.NewNotFound(podName)
}

func (w *workloadMetaMock) ListKubernetesNodes() []*KubernetesNode {
	entities := w.listEntitiesByKind(KindKubernetesNode)

	nodes := make([]*KubernetesNode, 0, len(entities))
	for i := range entities {
		nodes = append(nodes, entities[i].(*KubernetesNode))
	}

	return nodes
}

// GetKubernetesNode returns metadata about a Kubernetes node.
func (w *workloadMetaMock) GetKubernetesNode(id string) (*KubernetesNode, error) {
	entity, err := w.getEntityByKind(KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNode), nil
}

// GetKubernetesDeployment implements Component#GetKubernetesDeployment
func (w *workloadMetaMock) GetKubernetesDeployment(id string) (*KubernetesDeployment, error) {
	entity, err := w.getEntityByKind(KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesDeployment), nil
}

// GetKubernetesNamespace implements Component#GetKubernetesNamespace
func (w *workloadMetaMock) GetKubernetesNamespace(id string) (*KubernetesNamespace, error) {
	entity, err := w.getEntityByKind(KindKubernetesNamespace, id)
	if err != nil {
		return nil, err
	}

	return entity.(*KubernetesNamespace), nil
}

// GetECSTask returns metadata about an ECS task.
func (w *workloadMetaMock) GetECSTask(id string) (*ECSTask, error) {
	entity, err := w.getEntityByKind(KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ECSTask), nil
}

// ListECSTasks implements workloadMetaMock#ListECSTasks
func (w *workloadMetaMock) ListECSTasks() []*ECSTask {
	entities := w.listEntitiesByKind(KindECSTask)

	tasks := make([]*ECSTask, 0, len(entities))
	for _, entity := range entities {
		task := entity.(*ECSTask)
		tasks = append(tasks, task)
	}

	return tasks
}

// ListImages implements workloadMetaMock#ListImages
func (w *workloadMetaMock) ListImages() []*ContainerImageMetadata {
	entities := w.listEntitiesByKind(KindContainerImageMetadata)

	images := make([]*ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements workloadMetaMock#GetImage
func (w *workloadMetaMock) GetImage(id string) (*ContainerImageMetadata, error) {
	entity, err := w.getEntityByKind(KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*ContainerImageMetadata), nil
}

// Set sets an entity in the store.
func (w *workloadMetaMock) Set(entity Entity) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entityID := entity.GetID()

	if _, ok := w.store[entityID.Kind]; !ok {
		w.store[entityID.Kind] = make(map[string]Entity)
	}

	w.store[entityID.Kind][entityID.ID] = entity
}

// Unset removes an entity from the store.
func (w *workloadMetaMock) Unset(entity Entity) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entityID := entity.GetID()
	if _, ok := w.store[entityID.Kind]; !ok {
		return
	}

	delete(w.store[entityID.Kind], entityID.ID)
}

// Start is not implemented in the testing store.
func (w *workloadMetaMock) Start(_ context.Context) {
	panic("not implemented")
}

// Subscribe is not implemented in the testing store.
func (w *workloadMetaMock) Subscribe(_ string, _ SubscriberPriority, _ *Filter) chan EventBundle {
	panic("not implemented")
}

// Unsubscribe is not implemented in the testing store.
func (w *workloadMetaMock) Unsubscribe(_ chan EventBundle) {
	panic("not implemented")
}

// Notify is not implemented in the testing store.
func (w *workloadMetaMock) Notify(events []CollectorEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.eventsChan != nil {
		for _, event := range events {
			w.eventsChan <- event
		}
	}

	w.notifiedEvents = append(w.notifiedEvents, events...)
}

// Push pushes events from an external source into workloadmeta store
// This mock implementation does not check the event types
func (w *workloadMetaMock) Push(source Source, events ...Event) error {
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

// GetNotifiedEvents returns all registered notification events.
func (w *workloadMetaMock) GetNotifiedEvents() []CollectorEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var events []CollectorEvent
	events = append(events, w.notifiedEvents...)

	return events
}

// SubscribeToEvents returns a channel that receives events
func (w *workloadMetaMock) SubscribeToEvents() chan CollectorEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	w.eventsChan = make(chan CollectorEvent, 100)
	return w.eventsChan
}

// Dump is not implemented in the testing store.
func (w *workloadMetaMock) Dump(_ bool) WorkloadDumpResponse {
	panic("not implemented")
}

// Reset is not implemented in the testing store.
func (w *workloadMetaMock) Reset(_ []Entity, _ Source) {
	panic("not implemented")
}

func (w *workloadMetaMock) ResetProcesses(_ []Entity, _ Source) {
	panic("not implemented")
}

func (w *workloadMetaMock) getEntityByKind(kind Kind, id string) (Entity, error) {
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

func (w *workloadMetaMock) listEntitiesByKind(kind Kind) []Entity {
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

// GetConfig returns a Config Reader for the internal injected config
func (w *workloadMetaMock) GetConfig() pkgconfig.Reader {
	return w.config
}

// MockStore is a store designed to be used in unit tests
type workloadMetaMockV2 struct {
	*workloadmeta
}

// newWorkloadMetaMockV2 returns a Mock
func newWorkloadMetaMockV2(deps dependencies) Mock {
	w := &workloadMetaMockV2{
		workloadmeta: newWorkloadMeta(deps).Comp.(*workloadmeta),
	}
	return w
}

// Notify overrides store to allow for synchronous event processing
func (w *workloadMetaMockV2) Notify(events []CollectorEvent) {
	w.handleEvents(events)
}

// GetNotifiedEvents is not implemented for V2 mocks.
func (w *workloadMetaMockV2) GetNotifiedEvents() []CollectorEvent {
	panic("not implemented")
}

// SubscribeToEvents is not implemented for V2 mocks.
func (w *workloadMetaMockV2) SubscribeToEvents() chan CollectorEvent {
	panic("not implemented")
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

// GetConfig returns a Config Reader for the internal injected config
func (w *workloadMetaMockV2) GetConfig() pkgconfig.Reader {
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
