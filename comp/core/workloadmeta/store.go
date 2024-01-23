// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"time"
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

// Start starts the workload metadata store.
func (w *workloadmeta) Start(ctx context.Context) {
	panic("not called")
}

// Subscribe returns a channel where workload metadata events will be streamed
// as they happen. On first subscription, it will also generate an EventTypeSet
// event for each entity present in the store that matches filter, unless the
// filter type is EventTypeUnset.
func (w *workloadmeta) Subscribe(name string, priority SubscriberPriority, filter *Filter) chan EventBundle {
	panic("not called")
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (w *workloadmeta) Unsubscribe(ch chan EventBundle) {
	panic("not called")
}

// GetContainer implements Store#GetContainer.
func (w *workloadmeta) GetContainer(id string) (*Container, error) {
	panic("not called")
}

// ListContainers implements Store#ListContainers.
func (w *workloadmeta) ListContainers() []*Container {
	panic("not called")
}

// ListContainersWithFilter implements Store#ListContainersWithFilter
func (w *workloadmeta) ListContainersWithFilter(filter ContainerFilterFunc) []*Container {
	panic("not called")
}

// GetKubernetesPod implements Store#GetKubernetesPod
func (w *workloadmeta) GetKubernetesPod(id string) (*KubernetesPod, error) {
	panic("not called")
}

// GetKubernetesPodByName implements Store#GetKubernetesPodByName
func (w *workloadmeta) GetKubernetesPodByName(podName, podNamespace string) (*KubernetesPod, error) {
	panic("not called")
}

// GetProcess implements Store#GetProcess.
func (w *workloadmeta) GetProcess(pid int32) (*Process, error) {
	panic("not called")
}

// ListProcesses implements Store#ListProcesses.
func (w *workloadmeta) ListProcesses() []*Process {
	panic("not called")
}

// ListProcessesWithFilter implements Store#ListProcessesWithFilter
func (w *workloadmeta) ListProcessesWithFilter(filter ProcessFilterFunc) []*Process {
	panic("not called")
}

// GetKubernetesPodForContainer implements Store#GetKubernetesPodForContainer
func (w *workloadmeta) GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error) {
	panic("not called")
}

// GetKubernetesNode implements Store#GetKubernetesNode
func (w *workloadmeta) GetKubernetesNode(id string) (*KubernetesNode, error) {
	panic("not called")
}

// GetKubernetesDeployment implements Store#GetKubernetesDeployment
func (w *workloadmeta) GetKubernetesDeployment(id string) (*KubernetesDeployment, error) {
	panic("not called")
}

// GetECSTask implements Store#GetECSTask
func (w *workloadmeta) GetECSTask(id string) (*ECSTask, error) {
	panic("not called")
}

// ListImages implements Store#ListImages
func (w *workloadmeta) ListImages() []*ContainerImageMetadata {
	panic("not called")
}

// GetImage implements Store#GetImage
func (w *workloadmeta) GetImage(id string) (*ContainerImageMetadata, error) {
	panic("not called")
}

// Notify implements Store#Notify
func (w *workloadmeta) Notify(events []CollectorEvent) {
	panic("not called")
}

// ResetProcesses implements Store#ResetProcesses
func (w *workloadmeta) ResetProcesses(newProcesses []Entity, source Source) {
	panic("not called")
}

// Reset implements Store#Reset
func (w *workloadmeta) Reset(newEntities []Entity, source Source) {
	panic("not called")
}

func (w *workloadmeta) validatePushEvents(events []Event) error {
	panic("not called")
}

// Push implements Store#Push
func (w *workloadmeta) Push(source Source, events ...Event) error {
	panic("not called")
}

func (w *workloadmeta) startCandidatesWithRetry(ctx context.Context) error {
	panic("not called")
}

func (w *workloadmeta) startCandidates(ctx context.Context) bool {
	panic("not called")
}

func (w *workloadmeta) pull(ctx context.Context) {
	panic("not called")
}

func (w *workloadmeta) handleEvents(evs []CollectorEvent) {
	panic("not called")
}

func (w *workloadmeta) getEntityByKind(kind Kind, id string) (Entity, error) {
	panic("not called")
}

func (w *workloadmeta) listEntitiesByKind(kind Kind) []Entity {
	panic("not called")
}

func (w *workloadmeta) unsubscribeAll() {
	panic("not called")
}

// call holding lock on w.subscribersMut
func (w *workloadmeta) notifyChannel(name string, ch chan EventBundle, events []Event, wait bool) {
	panic("not called")
}

func classifyByKindAndID(entities []Entity) map[Kind]map[string]Entity {
	panic("not called")
}
