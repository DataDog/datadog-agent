// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/imdario/mergo"
	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type Store interface {
	Start(ctx context.Context)
	Subscribe(name string, filter *Filter) chan EventBundle
	Unsubscribe(ch chan EventBundle)
	GetContainer(id string) (*Container, error)
	GetKubernetesPod(id string) (*KubernetesPod, error)
	GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error)
	GetECSTask(id string) (*ECSTask, error)
	Notify(events []CollectorEvent)
}

// Kind is the kind of an entity.
type Kind string

// Source is the source name of an entity.
type Source string

// ContainerRuntime is the container runtime used by a container.
type ContainerRuntime string

// ECSLaunchType is the launch type of an ECS task.
type ECSLaunchType string

// EventType is the type of an event.
type EventType int

// List of enumerable constants for the types above.
const (
	KindContainer     Kind = "container"
	KindKubernetesPod Kind = "kubernetes_pod"
	KindECSTask       Kind = "ecs_task"

	SourceDocker       Source = "docker"
	SourceContainerd   Source = "containerd"
	SourceECS          Source = "ecs"
	SourceECSFargate   Source = "ecs_fargate"
	SourceKubelet      Source = "kubelet"
	SourceKubeMetadata Source = "kube_metadata"

	ContainerRuntimeDocker     ContainerRuntime = "docker"
	ContainerRuntimeContainerd ContainerRuntime = "containerd"

	ECSLaunchTypeEC2     ECSLaunchType = "ec2"
	ECSLaunchTypeFargate ECSLaunchType = "fargate"

	EventTypeSet EventType = iota
	EventTypeUnset
)

// Entity is an item in the metadata store. It exists as an interface to avoid
// usage of interface{}.
type Entity interface {
	GetID() EntityID
	Merge(Entity) error
	DeepCopy() Entity
}

// EntityID represents the ID of an Entity.
type EntityID struct {
	Kind Kind
	ID   string
}

// GetID satisfies the Entity interface for EntityID to allow a standalone
// EntityID to be passed in events of type EventTypeUnset without the need to
// build a full, concrete entity.
func (i EntityID) GetID() EntityID {
	return i
}

// Merge returns an error because EntityID is not expected to be merged with another Entity, because it's used
// as an identifier.
func (i EntityID) Merge(e Entity) error {
	return errors.New("cannot merge EntityID with another entity")
}

// DeepCopy returns a deep copy of EntityID.
func (i EntityID) DeepCopy() Entity {
	return i
}

var _ Entity = EntityID{}

// EntityMeta represents generic metadata about an Entity.
type EntityMeta struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Labels      map[string]string
}

// ContainerImage is the an image used by a container.
type ContainerImage struct {
	ID        string
	RawName   string
	Name      string
	ShortName string
	Tag       string
}

// NewContainerImage builds a ContainerImage from an image name
func NewContainerImage(imageName string) (ContainerImage, error) {
	image := ContainerImage{
		RawName: imageName,
		Name:    imageName,
	}

	name, shortName, tag, err := containers.SplitImageName(imageName)
	if err != nil {
		return image, err
	}

	if tag == "" {
		tag = "latest"
	}

	image.Name = name
	image.ShortName = shortName
	image.Tag = tag

	return image, nil
}

// ContainerState is the state of a container.
type ContainerState struct {
	Running    bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// ContainerPort is a port open in the container.
type ContainerPort struct {
	Name     string
	Port     int
	Protocol string
}

// OrchestratorContainer is a reference to a Container with
// orchestrator-specific data attached to it.
type OrchestratorContainer struct {
	ID    string
	Name  string
	Image ContainerImage
}

// Container is a containerized workload.
type Container struct {
	EntityID
	EntityMeta
	EnvVars    map[string]string
	Hostname   string
	Image      ContainerImage
	NetworkIPs map[string]string
	PID        int
	Ports      []ContainerPort
	Runtime    ContainerRuntime
	State      ContainerState
}

// GetID returns the Container's EntityID.
func (c Container) GetID() EntityID {
	return c.EntityID
}

// Merge merges a Container with another. Returns an error if trying to merge
// with another kind.
func (c *Container) Merge(e Entity) error {
	cc, ok := e.(*Container)
	if !ok {
		return fmt.Errorf("cannot merge Container with different kind %T", e)
	}

	return mergo.Merge(c, cc)
}

// DeepCopy returns a deep copy of the container.
func (c Container) DeepCopy() Entity {
	cp := deepcopy.Copy(c).(Container)
	return &cp
}

var _ Entity = &Container{}

// KubernetesPod is a Kubernetes Pod.
type KubernetesPod struct {
	EntityID
	EntityMeta
	Owners                     []KubernetesPodOwner
	PersistentVolumeClaimNames []string
	Containers                 []OrchestratorContainer
	Ready                      bool
	Phase                      string
	IP                         string
	PriorityClass              string
	KubeServices               []string
	NamespaceLabels            map[string]string
}

// GetID returns the KubernetesPod's EntityID.
func (p KubernetesPod) GetID() EntityID {
	return p.EntityID
}

// Merge merges a KubernetesPod with another. Returns an error if trying to merge
// with another kind.
func (p *KubernetesPod) Merge(e Entity) error {
	pp, ok := e.(*KubernetesPod)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesPod with different kind %T", e)
	}

	return mergo.Merge(p, pp)
}

// DeepCopy returns a deep copy of the pod.
func (p KubernetesPod) DeepCopy() Entity {
	cp := deepcopy.Copy(p).(KubernetesPod)
	return &cp
}

var _ Entity = &KubernetesPod{}

// KubernetesPodOwner is extracted from a pod's owner references.
type KubernetesPodOwner struct {
	Kind string
	Name string
	ID   string
}

// ECSTask is an ECS Task.
type ECSTask struct {
	EntityID
	EntityMeta
	Tags                  map[string]string
	ContainerInstanceTags map[string]string
	ClusterName           string
	Region                string
	AvailabilityZone      string
	Family                string
	Version               string
	LaunchType            ECSLaunchType
	Containers            []OrchestratorContainer
}

// GetID returns an ECSTasks's EntityID.
func (t ECSTask) GetID() EntityID {
	return t.EntityID
}

// Merge merges a ECSTask with another. Returns an error if trying to merge
// with another kind.
func (t *ECSTask) Merge(e Entity) error {
	tt, ok := e.(*ECSTask)
	if !ok {
		return fmt.Errorf("cannot merge ECSTask with different kind %T", e)
	}

	return mergo.Merge(t, tt)
}

// DeepCopy returns a deep copy of the task.
func (t ECSTask) DeepCopy() Entity {
	cp := deepcopy.Copy(t).(ECSTask)
	return &cp
}

var _ Entity = &ECSTask{}

// CollectorEvent is an event generated by a metadata collector, to be handled
// by the metadata store.
type CollectorEvent struct {
	Type   EventType
	Source Source
	Entity Entity
}

// Event is an event generated by the metadata store, to be broadcasted to
// subscribers.
type Event struct {
	Type    EventType
	Sources []Source
	Entity  Entity
}

// EventBundle is a collection of events, and a channel that needs to be closed
// when the receiving subscriber wants to unblock the notifier.
type EventBundle struct {
	Events []Event
	Ch     chan struct{}
}
