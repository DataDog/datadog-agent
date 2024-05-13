// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package workloadmeta provides the workloadmeta component for the Datadog Agent
package workloadmeta

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: container-platform

// Component is the component type.
type Component interface {
	// Subscribe subscribes the caller to events representing changes to the
	// store, limited to events matching the filter.  The name is used for
	// telemetry and debugging.
	//
	// The first message on the channel is special: it contains an EventTypeSet
	// event for each entity currently in the store.  If the Subscribe call
	// occurs at agent startup, then the first message approximates entities
	// that were running before the agent started.  This is an inherently racy
	// distinction, but may be useful for decisions such as whether to begin
	// logging at the head or tail of an entity's logs.
	//
	// Multiple EventTypeSet messages may be sent, either as the entity's state
	// evolves or as information about the entity is reported from multiple
	// sources (such as a container runtime and an orchestrator).
	//
	// See the documentation for EventBundle regarding appropropriate handling
	// for messages on this channel.
	Subscribe(name string, priority SubscriberPriority, filter *Filter) chan EventBundle

	// Unsubscribe closes the EventBundle channel. Note that it will emit a zero-value event.
	// Thus, it is important to check that the channel is not closed.
	Unsubscribe(ch chan EventBundle)

	// GetContainer returns metadata about a container.  It fetches the entity
	// with kind KindContainer and the given ID.
	GetContainer(id string) (*Container, error)

	// ListContainers returns metadata about all known containers, equivalent
	// to all entities with kind KindContainer.
	ListContainers() []*Container

	// ListContainersWithFilter returns all the containers for which the passed
	// filter evaluates to true.
	ListContainersWithFilter(filter ContainerFilterFunc) []*Container

	// GetKubernetesPod returns metadata about a Kubernetes pod.  It fetches
	// the entity with kind KindKubernetesPod and the given ID.
	GetKubernetesPod(id string) (*KubernetesPod, error)

	// GetKubernetesPodForContainer retrieves the ownership information for the
	// given container and returns the owner pod. This information might lag because
	// the kubelet check sets the `Owner` field but a container can also be stored by CRI
	// checks, which do not have ownership info. Thus, the function might return an error
	// when the pod actually exists.
	GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error)

	// GetKubernetesPodByName returns the first pod whose name and namespace matches those passed in
	// to this function.
	GetKubernetesPodByName(podName, podNamespace string) (*KubernetesPod, error)

	// GetKubernetesNode returns metadata about a Kubernetes node. It fetches
	// the entity with kind KindKubernetesNode and the given ID.
	GetKubernetesNode(id string) (*KubernetesNode, error)

	// GetKubernetesDeployment returns metadata about a Kubernetes deployment. It fetches
	// the entity with kind KindKubernetesDeployment and the given ID.
	GetKubernetesDeployment(id string) (*KubernetesDeployment, error)

	// GetKubernetesNamespace returns metadata about a Kubernetes namespace. It fetches
	// the entity with kind KindKubernetesNamespace and the given ID.
	GetKubernetesNamespace(id string) (*KubernetesNamespace, error)

	// ListECSTasks returns metadata about all ECS tasks, equivalent to all
	// entities with kind KindECSTask.
	ListECSTasks() []*ECSTask

	// GetECSTask returns metadata about an ECS task.  It fetches the entity with
	// kind KindECSTask and the given ID.
	GetECSTask(id string) (*ECSTask, error)

	// ListImages returns metadata about all known images, equivalent to all
	// entities with kind KindContainerImageMetadata.
	ListImages() []*ContainerImageMetadata

	// GetImage returns metadata about a container image. It fetches the entity
	// with kind KindContainerImageMetadata and the given ID.
	GetImage(id string) (*ContainerImageMetadata, error)

	// GetProcess returns metadata about a process.  It fetches the entity
	// with kind KindProcess and the given ID.
	GetProcess(pid int32) (*Process, error)

	// ListProcesses returns metadata about all known processes, equivalent
	// to all entities with kind KindProcess.
	ListProcesses() []*Process

	// ListProcessesWithFilter returns all the processes for which the passed
	// filter evaluates to true.
	ListProcessesWithFilter(filterFunc ProcessFilterFunc) []*Process

	// Notify notifies the store with a slice of events.  It should only be
	// used by workloadmeta collectors.
	Notify(events []CollectorEvent)

	// Dump lists the content of the store, for debugging purposes.
	Dump(verbose bool) WorkloadDumpResponse

	// ResetProcesses resets the state of the store so that newProcesses are the
	// only entites stored.
	ResetProcesses(newProcesses []Entity, source Source)

	// Reset resets the state of the store so that newEntities are the only
	// entities stored. This function sends events to the subscribers in the
	// following cases:
	// - EventTypeSet: one for each entity in newEntities that doesn't exist in
	// the store. Also, when the entity exists, but with different values.
	// - EventTypeUnset: one for each entity that exists in the store but is not
	// present in newEntities.
	Reset(newEntities []Entity, source Source)

	// Push allows external sources to push events to the metadata store.
	// Only EventTypeSet and EventTypeUnset event types are allowed.
	Push(source Source, events ...Event) error
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newWorkloadMeta,
		),
		fx.Provide(func(wmeta Component) optional.Option[Component] {
			return optional.NewOption(wmeta)
		}),
	)
}

// OptionalModule defines the fx options when workloadmeta should be used as an optional.
func OptionalModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newWorkloadMetaOptional,
		),
	)
}
