// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadmeta

// team: container-platform

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
)

const (
	mockSource = wmdef.Source("mockSource")
)

// store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type workloadMetaMock struct {
	log    log.Component
	config config.Component

	mu             sync.RWMutex
	store          map[wmdef.Kind]map[string]wmdef.Entity
	notifiedEvents []wmdef.CollectorEvent
	eventsChan     chan wmdef.CollectorEvent
}

// NewWorkloadMetaMock returns a new workloadMetaMock.
func NewWorkloadMetaMock(deps dependencies) wmmock.Mock {

	mock := &workloadMetaMock{
		log:    deps.Log,
		config: deps.Config,
		store:  make(map[wmdef.Kind]map[string]wmdef.Entity),
	}

	return mock
}

func (w *workloadMetaMock) GetContainer(id string) (*wmdef.Container, error) {
	entity, err := w.getEntityByKind(wmdef.KindContainer, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.Container), nil
}

// GetKubernetesMetadata implements workloadMetaMock#GetKubernetesMetadata.
func (w *workloadMetaMock) GetKubernetesMetadata(id string) (*wmdef.KubernetesMetadata, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesMetadata), nil
}

// ListContainers returns metadata about all known containers.
func (w *workloadMetaMock) ListContainers() []*wmdef.Container {
	entities := w.listEntitiesByKind(wmdef.KindContainer)

	// Not very efficient
	containers := make([]*wmdef.Container, 0, len(entities))
	for _, entity := range entities {
		containers = append(containers, entity.(*wmdef.Container))
	}

	return containers
}

// ListContainersWithFilter returns metadata about the containers that pass the given filter.
func (w *workloadMetaMock) ListContainersWithFilter(filter wmdef.EntityFilterFunc[*wmdef.Container]) []*wmdef.Container {
	var res []*wmdef.Container

	for _, container := range w.ListContainers() {
		if filter(container) {
			res = append(res, container)
		}
	}

	return res
}

// GetProcess implements workloadMetaMock#GetProcess.
func (w *workloadMetaMock) GetProcess(pid int32) (*wmdef.Process, error) {
	id := string(pid)
	entity, err := w.getEntityByKind(wmdef.KindProcess, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.Process), nil
}

// ListProcesses implements workloadMetaMock#ListProcesses.
func (w *workloadMetaMock) ListProcesses() []*wmdef.Process {
	entities := w.listEntitiesByKind(wmdef.KindProcess)

	processes := make([]*wmdef.Process, 0, len(entities))
	for i := range entities {
		processes = append(processes, entities[i].(*wmdef.Process))
	}

	return processes
}

// ListProcessesWithFilter implements workloadMetaMock#ListProcessesWithFilter.
func (w *workloadMetaMock) ListProcessesWithFilter(filter wmdef.EntityFilterFunc[*wmdef.Process]) []*wmdef.Process {
	var res []*wmdef.Process

	for _, process := range w.ListProcesses() {
		if filter(process) {
			res = append(res, process)
		}
	}

	return res
}

// GetKubernetesPod returns metadata about a Kubernetes pod.
func (w *workloadMetaMock) GetKubernetesPod(id string) (*wmdef.KubernetesPod, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesPod), nil
}

// GetKubernetesPodForContainer returns a KubernetesPod that contains the
// specified containerID.
func (w *workloadMetaMock) GetKubernetesPodForContainer(containerID string) (*wmdef.KubernetesPod, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	containerEntities, ok := w.store[wmdef.KindContainer]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	containerEntity, ok := containerEntities[containerID]
	if !ok {
		return nil, errors.NewNotFound(containerID)
	}

	container := containerEntity.(*wmdef.Container)
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

	return pod.(*wmdef.KubernetesPod), nil
}

// GetKubernetesPodByName implements Store#GetKubernetesPodByName
func (w *workloadMetaMock) GetKubernetesPodByName(podName, podNamespace string) (*wmdef.KubernetesPod, error) {
	entities := w.listEntitiesByKind(wmdef.KindKubernetesPod)

	// Not very efficient
	for k := range entities {
		entity := entities[k].(*wmdef.KubernetesPod)
		if entity.Name == podName && entity.Namespace == podNamespace {
			return entity, nil
		}
	}

	return nil, errors.NewNotFound(podName)
}

func (w *workloadMetaMock) ListKubernetesNodes() []*wmdef.KubernetesNode {
	entities := w.listEntitiesByKind(wmdef.KindKubernetesNode)

	nodes := make([]*wmdef.KubernetesNode, 0, len(entities))
	for i := range entities {
		nodes = append(nodes, entities[i].(*wmdef.KubernetesNode))
	}

	return nodes
}

// GetKubernetesNode returns metadata about a Kubernetes node.
func (w *workloadMetaMock) GetKubernetesNode(id string) (*wmdef.KubernetesNode, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesPod, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesNode), nil
}

// GetKubernetesDeployment implements Component#GetKubernetesDeployment
func (w *workloadMetaMock) GetKubernetesDeployment(id string) (*wmdef.KubernetesDeployment, error) {
	entity, err := w.getEntityByKind(wmdef.KindKubernetesDeployment, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.KubernetesDeployment), nil
}

// GetECSTask returns metadata about an ECS task.
func (w *workloadMetaMock) GetECSTask(id string) (*wmdef.ECSTask, error) {
	entity, err := w.getEntityByKind(wmdef.KindECSTask, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.ECSTask), nil
}

// ListECSTasks implements workloadMetaMock#ListECSTasks
func (w *workloadMetaMock) ListECSTasks() []*wmdef.ECSTask {
	entities := w.listEntitiesByKind(wmdef.KindECSTask)

	tasks := make([]*wmdef.ECSTask, 0, len(entities))
	for _, entity := range entities {
		task := entity.(*wmdef.ECSTask)
		tasks = append(tasks, task)
	}

	return tasks
}

// ListImages implements workloadMetaMock#ListImages
func (w *workloadMetaMock) ListImages() []*wmdef.ContainerImageMetadata {
	entities := w.listEntitiesByKind(wmdef.KindContainerImageMetadata)

	images := make([]*wmdef.ContainerImageMetadata, 0, len(entities))
	for _, entity := range entities {
		image := entity.(*wmdef.ContainerImageMetadata)
		images = append(images, image)
	}

	return images
}

// GetImage implements workloadMetaMock#GetImage
func (w *workloadMetaMock) GetImage(id string) (*wmdef.ContainerImageMetadata, error) {
	entity, err := w.getEntityByKind(wmdef.KindContainerImageMetadata, id)
	if err != nil {
		return nil, err
	}

	return entity.(*wmdef.ContainerImageMetadata), nil
}

// Set sets an entity in the store.
func (w *workloadMetaMock) Set(entity wmdef.Entity) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entityID := entity.GetID()

	if _, ok := w.store[entityID.Kind]; !ok {
		w.store[entityID.Kind] = make(map[string]wmdef.Entity)
	}

	w.store[entityID.Kind][entityID.ID] = entity
}

// Unset removes an entity from the store.
func (w *workloadMetaMock) Unset(entity wmdef.Entity) {
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
func (w *workloadMetaMock) Subscribe(_ string, _ wmdef.SubscriberPriority, _ *wmdef.Filter) chan wmdef.EventBundle {
	panic("not implemented")
}

// Unsubscribe is not implemented in the testing store.
func (w *workloadMetaMock) Unsubscribe(_ chan wmdef.EventBundle) {
	panic("not implemented")
}

// Notify is not implemented in the testing store.
func (w *workloadMetaMock) Notify(events []wmdef.CollectorEvent) {
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
func (w *workloadMetaMock) Push(source wmdef.Source, events ...wmdef.Event) error {
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

// GetNotifiedEvents returns all registered notification events.
func (w *workloadMetaMock) GetNotifiedEvents() []wmdef.CollectorEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var events []wmdef.CollectorEvent
	events = append(events, w.notifiedEvents...)

	return events
}

// SubscribeToEvents returns a channel that receives events
func (w *workloadMetaMock) SubscribeToEvents() chan wmdef.CollectorEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	w.eventsChan = make(chan wmdef.CollectorEvent, 100)
	return w.eventsChan
}

// Dump is not implemented in the testing store.
func (w *workloadMetaMock) Dump(_ bool) wmdef.WorkloadDumpResponse {
	panic("not implemented")
}

// Reset is not implemented in the testing store.
func (w *workloadMetaMock) Reset(_ []wmdef.Entity, _ wmdef.Source) {
	panic("not implemented")
}

func (w *workloadMetaMock) ResetProcesses(_ []wmdef.Entity, _ wmdef.Source) {
	panic("not implemented")
}

func (w *workloadMetaMock) getEntityByKind(kind wmdef.Kind, id string) (wmdef.Entity, error) {
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

func (w *workloadMetaMock) listEntitiesByKind(kind wmdef.Kind) []wmdef.Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entitiesOfKind, ok := w.store[kind]
	if !ok {
		return nil
	}

	entities := make([]wmdef.Entity, 0, len(entitiesOfKind))
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

// NewWorkloadMetaMockV2 returns a Mock
func NewWorkloadMetaMockV2(deps dependencies) wmmock.Mock {
	w := &workloadMetaMockV2{
		workloadmeta: NewWorkloadMeta(deps).Comp.(*workloadmeta),
	}
	return w
}

// Notify overrides store to allow for synchronous event processing
func (w *workloadMetaMockV2) Notify(events []wmdef.CollectorEvent) {
	w.handleEvents(events)
}

// GetNotifiedEvents is not implemented for V2 mocks.
func (w *workloadMetaMockV2) GetNotifiedEvents() []wmdef.CollectorEvent {
	panic("not implemented")
}

// SubscribeToEvents is not implemented for V2 mocks.
func (w *workloadMetaMockV2) SubscribeToEvents() chan wmdef.CollectorEvent {
	panic("not implemented")
}

// SetEntity generates a Set event
func (w *workloadMetaMockV2) Set(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
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
func (w *workloadMetaMockV2) Unset(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: mockSource,
			Entity: e,
		},
	})
}
