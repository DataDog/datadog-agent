// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imdario/mergo"
	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
//
// Typically there is only one instance, accessed via GetGlobalStore.
type Store interface {
	// Start starts the store, asynchronously initializing collectors and
	// beginning to gather workload data.  This is typically called during
	// agent startup.
	Start(ctx context.Context)

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

	// Unsubscribe reverses the effect of Subscribe.
	Unsubscribe(ch chan EventBundle)

	// GetContainer returns metadata about a container.  It fetches the entity
	// with kind KindContainer and the given ID.
	GetContainer(id string) (*Container, error)

	// ListContainers returns metadata about all known containers, equivalent
	// to all entities with kind KindContainer.
	ListContainers() ([]*Container, error)

	// GetKubernetesPod returns metadata about a Kubernetes pod.  It fetches
	// the entity with kind KindKubernetesPod and the given ID.
	GetKubernetesPod(id string) (*KubernetesPod, error)

	// GetKubernetesPodForContainer searches all known KubernetesPod entities
	// for one containing the given container.
	GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error)

	// GetECSTask returns metadata about an ECS task.  It fetches the entity with
	// kind KindECSTask and the given ID.
	GetECSTask(id string) (*ECSTask, error)

	// Notify notifies the store with a slice of events.  It should only be
	// used by workloadmeta collectors.
	Notify(events []CollectorEvent)

	// Dump lists the content of the store, for debugging purposes.
	Dump(verbose bool) WorkloadDumpResponse
}

// Kind is the kind of an entity.
type Kind string

// Defined Kinds
const (
	KindContainer     Kind = "container"
	KindKubernetesPod Kind = "kubernetes_pod"
	KindECSTask       Kind = "ecs_task"
)

// Source is the source name of an entity.
type Source string

// Defined Sources
const (
	// SourceAll matches any source. Should not be returned by collectors,
	// as its only meant to be used in filters.
	SourceAll Source = ""

	// SourceRuntime represents entities detected by the container runtime
	// running on the node, collecting lower level information about
	// containers. `docker`, `containerd`, `podman` and `ecs_fargate` use
	// this source.
	SourceRuntime Source = "runtime"

	// SourceNodeOrchestrator represents entities detected by the node
	// agent from an orchestrator. `kubelet` and `ecs` use this.
	SourceNodeOrchestrator Source = "node_orchestrator"

	// SourceClusterOrchestrator represents entities detected by calling
	// the central component of an orchestrator, or the Datadog Cluster
	// Agent.  `kube_metadata` and `cloudfoundry` use this.
	SourceClusterOrchestrator Source = "cluster_orchestrator"
)

// ContainerRuntime is the container runtime used by a container.
type ContainerRuntime string

// Defined ContainerRuntimes
const (
	ContainerRuntimeDocker     ContainerRuntime = "docker"
	ContainerRuntimeContainerd ContainerRuntime = "containerd"
	ContainerRuntimePodman     ContainerRuntime = "podman"
	ContainerRuntimeCRIO       ContainerRuntime = "cri-o"
	ContainerRuntimeGarden     ContainerRuntime = "garden"
	// ECS Fargate can be considered as a runtime in the sense that we don't
	// know the actual runtime but we need to identify it's Fargate
	ContainerRuntimeECSFargate ContainerRuntime = "ecsfargate"
)

// ContainerStatus is the status of the container
type ContainerStatus string

// Defined ContainerStatus
const (
	ContainerStatusUnknown    ContainerStatus = "unknown"
	ContainerStatusCreated    ContainerStatus = "created"
	ContainerStatusRunning    ContainerStatus = "running"
	ContainerStatusRestarting ContainerStatus = "restarting"
	ContainerStatusPaused     ContainerStatus = "paused"
	ContainerStatusStopped    ContainerStatus = "stopped"
)

// ContainerHealth is the health of the container
type ContainerHealth string

// Defined ContainerHealth
const (
	ContainerHealthUnknown   ContainerHealth = "unknown"
	ContainerHealthHealthy   ContainerHealth = "healthy"
	ContainerHealthUnhealthy ContainerHealth = "unhealthy"
)

// ECSLaunchType is the launch type of an ECS task.
type ECSLaunchType string

// Defined ECSLaunchTypes
const (
	ECSLaunchTypeEC2     ECSLaunchType = "ec2"
	ECSLaunchTypeFargate ECSLaunchType = "fargate"
)

// EventType is the type of an event (set or unset).
type EventType int

const (
	// EventTypeSet indicates that an entity has been added or updated.
	EventTypeSet EventType = iota

	// EventTypeUnset indicates that an entity has been removed.  If multiple
	// sources provide data for an entity, this message is only sent when the
	// last source stops providing that data.
	EventTypeUnset
)

// Entity represents a single unit of work being done that is of interest to
// the agent.
//
// This interface is implemented by several concrete types, and is typically
// cast to that concrete type to get detailed information.  For EntityTypeSet
// events, the concrete type corresponds to the entity's type (GetID().Kind),
// and it is safe to make an unchecked cast.  For EntityTypeUnset, the entity
// is an EntityID and such a cast will fail.
type Entity interface {
	// GetID gets the EntityID for this entity.
	GetID() EntityID

	// Merge merges this entity with another of the same kind.  This is used
	// to generate a composite entity representing data from several sources.
	Merge(Entity) error

	// DeepCopy copies an entity such that modifications of the copy will not
	// affect the original.
	DeepCopy() Entity

	// String provides a summary of the entity.  The string may span several lines,
	// especially if verbose.
	String(verbose bool) string
}

// EntityID represents the ID of an Entity.  Note that entities from different sources
// may have the same EntityID.
type EntityID struct {
	// Kind identifies the kind of entity.  This typically corresponds to the concrete
	// type of the Entity, but this is not always the case; see Entity for details.
	Kind Kind

	// ID is the ID for this entity, in a format specific to the entity Kind.
	ID string
}

// EntityID satisfies the Entity interface for EntityID to allow a standalone
// EntityID to be passed in events of type EventTypeUnset without the need to
// build a full, concrete entity.
var _ Entity = EntityID{}

// GetID implements Entity#GetID.
func (i EntityID) GetID() EntityID {
	return i
}

// Merge implements Entity#Merge.
func (i EntityID) Merge(e Entity) error {
	// Merge returns an error because EntityID is not expected to be merged
	// with another Entity, because it's used as an identifier.
	return errors.New("cannot merge EntityID with another entity")
}

// DeepCopy implements Entity#DeepCopy.
func (i EntityID) DeepCopy() Entity {
	return i
}

// String implements Entity#String.
func (i EntityID) String(_ bool) string {
	return fmt.Sprintln("Kind:", i.Kind, "ID:", i.ID)
}

// EntityMeta represents generic metadata about an Entity.
type EntityMeta struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Labels      map[string]string
}

// String returns a string representation of EntityMeta.
func (e EntityMeta) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Name:", e.Name)
	_, _ = fmt.Fprintln(&sb, "Namespace:", e.Namespace)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Annotations:", mapToString(e.Annotations))
		_, _ = fmt.Fprintln(&sb, "Labels:", mapToString(e.Labels))
	}

	return sb.String()
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

// String returns a string representation of ContainerImage.
func (c ContainerImage) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Name:", c.Name)
	_, _ = fmt.Fprintln(&sb, "Tag:", c.Tag)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "ID:", c.ID)
		_, _ = fmt.Fprintln(&sb, "Raw Name:", c.RawName)
		_, _ = fmt.Fprintln(&sb, "Short Name:", c.ShortName)
	}

	return sb.String()
}

// ContainerState is the state of a container.
type ContainerState struct {
	Running    bool
	Status     ContainerStatus
	Health     ContainerHealth
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   *uint32
}

// String returns a string representation of ContainerState.
func (c ContainerState) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Running:", c.Running)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Status:", c.Status)
		_, _ = fmt.Fprintln(&sb, "Health:", c.Health)
		_, _ = fmt.Fprintln(&sb, "Created At:", c.CreatedAt)
		_, _ = fmt.Fprintln(&sb, "Started At:", c.StartedAt)
		_, _ = fmt.Fprintln(&sb, "Finished At:", c.FinishedAt)
		if c.ExitCode != nil {
			_, _ = fmt.Fprintln(&sb, "Exit Code:", *c.ExitCode)
		}
	}

	return sb.String()
}

// ContainerPort is a port open in the container.
type ContainerPort struct {
	Name     string
	Port     int
	Protocol string
}

// String returns a string representation of ContainerPort.
func (c ContainerPort) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Port:", c.Port)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Name:", c.Name)
		_, _ = fmt.Fprintln(&sb, "Protocol:", c.Protocol)
	}

	return sb.String()
}

// OrchestratorContainer is a reference to a Container with
// orchestrator-specific data attached to it.
type OrchestratorContainer struct {
	ID    string
	Name  string
	Image ContainerImage
}

// String returns a string representation of OrchestratorContainer.
func (o OrchestratorContainer) String(_ bool) string {
	return fmt.Sprintln("Name:", o.Name, "ID:", o.ID)
}

// Container is an Entity representing a containerized workload.
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
	// CollectorTags represent tags coming from the collector itself
	// and that it would impossible to compute later on
	CollectorTags []string
}

// GetID implements Entity#GetID.
func (c Container) GetID() EntityID {
	return c.EntityID
}

// Merge implements Entity#Merge.
func (c *Container) Merge(e Entity) error {
	cc, ok := e.(*Container)
	if !ok {
		return fmt.Errorf("cannot merge Container with different kind %T", e)
	}

	return mergo.Merge(c, cc)
}

// DeepCopy implements Entity#DeepCopy.
func (c Container) DeepCopy() Entity {
	cp := deepcopy.Copy(c).(Container)
	return &cp
}

// String implements Entity#String.
func (c Container) String(verbose bool) string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprint(&sb, c.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, c.EntityMeta.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Image -----------")
	_, _ = fmt.Fprint(&sb, c.Image.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Container Info -----------")
	_, _ = fmt.Fprintln(&sb, "Runtime:", c.Runtime)
	_, _ = fmt.Fprint(&sb, c.State.String(verbose))

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Env Variables:", mapToString(c.EnvVars))
		_, _ = fmt.Fprintln(&sb, "Hostname:", c.Hostname)
		_, _ = fmt.Fprintln(&sb, "Network IPs:", mapToString(c.NetworkIPs))
		_, _ = fmt.Fprintln(&sb, "PID:", c.PID)
	}

	if len(c.Ports) > 0 && verbose {
		_, _ = fmt.Fprintln(&sb, "----------- Ports -----------")
		for _, p := range c.Ports {
			_, _ = fmt.Fprint(&sb, p.String(verbose))
		}
	}

	return sb.String()
}

var _ Entity = &Container{}

// KubernetesPod is an Entity representing a Kubernetes Pod.
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

// GetID implements Entity#GetID.
func (p KubernetesPod) GetID() EntityID {
	return p.EntityID
}

// Merge implements Entity#Merge.
func (p *KubernetesPod) Merge(e Entity) error {
	pp, ok := e.(*KubernetesPod)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesPod with different kind %T", e)
	}

	return mergo.Merge(p, pp)
}

// DeepCopy implements Entity#DeepCopy.
func (p KubernetesPod) DeepCopy() Entity {
	cp := deepcopy.Copy(p).(KubernetesPod)
	return &cp
}

// String implements Entity#String.
func (p KubernetesPod) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, p.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, p.EntityMeta.String(verbose))

	if len(p.Owners) > 0 {
		_, _ = fmt.Fprintln(&sb, "----------- Owners -----------")
		for _, o := range p.Owners {
			_, _ = fmt.Fprint(&sb, o.String(verbose))
		}
	}

	if len(p.Containers) > 0 {
		_, _ = fmt.Fprintln(&sb, "----------- Containers -----------")
		for _, c := range p.Containers {
			_, _ = fmt.Fprint(&sb, c.String(verbose))
		}
	}

	_, _ = fmt.Fprintln(&sb, "----------- Pod Info -----------")
	_, _ = fmt.Fprintln(&sb, "Ready:", p.Ready)
	_, _ = fmt.Fprintln(&sb, "Phase:", p.Phase)
	_, _ = fmt.Fprintln(&sb, "IP:", p.IP)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Priority Class:", p.PriorityClass)
		_, _ = fmt.Fprintln(&sb, "PVCs:", sliceToString(p.PersistentVolumeClaimNames))
		_, _ = fmt.Fprintln(&sb, "Kube Services:", sliceToString(p.KubeServices))
		_, _ = fmt.Fprintln(&sb, "Namespace Labels:", mapToString(p.NamespaceLabels))
	}

	return sb.String()
}

var _ Entity = &KubernetesPod{}

// KubernetesPodOwner is extracted from a pod's owner references.
type KubernetesPodOwner struct {
	Kind string
	Name string
	ID   string
}

// String returns a string representation of KubernetesPodOwner.
func (o KubernetesPodOwner) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Kind:", o.Kind, "Name:", o.Name)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "ID:", o.ID)
	}

	return sb.String()
}

// ECSTask is an Entity representing an ECS Task.
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

// GetID implements Entity#GetID.
func (t ECSTask) GetID() EntityID {
	return t.EntityID
}

// Merge implements Entity#Merge.
func (t *ECSTask) Merge(e Entity) error {
	tt, ok := e.(*ECSTask)
	if !ok {
		return fmt.Errorf("cannot merge ECSTask with different kind %T", e)
	}

	return mergo.Merge(t, tt)
}

// DeepCopy implements Entity#DeepCopy.
func (t ECSTask) DeepCopy() Entity {
	cp := deepcopy.Copy(t).(ECSTask)
	return &cp
}

// String implements Entity#String.
func (t ECSTask) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprint(&sb, t.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, t.EntityMeta.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Containers -----------")
	for _, c := range t.Containers {
		_, _ = fmt.Fprint(&sb, c.String(verbose))
	}

	if verbose {
		_, _ = fmt.Fprintln(&sb, "----------- Task Info -----------")
		_, _ = fmt.Fprintln(&sb, "Tags:", mapToString(t.Tags))
		_, _ = fmt.Fprintln(&sb, "Container Instance Tags:", mapToString(t.ContainerInstanceTags))
		_, _ = fmt.Fprintln(&sb, "Cluster Name:", t.ClusterName)
		_, _ = fmt.Fprintln(&sb, "Region:", t.Region)
		_, _ = fmt.Fprintln(&sb, "Availability Zone:", t.AvailabilityZone)
		_, _ = fmt.Fprintln(&sb, "Family:", t.Family)
		_, _ = fmt.Fprintln(&sb, "Version:", t.Version)
		_, _ = fmt.Fprintln(&sb, "Launch Type:", t.LaunchType)
	}

	return sb.String()
}

var _ Entity = &ECSTask{}

// GardenContainer is an Entity representing a CloudFoundry Garden Container
type GardenContainer struct {
	EntityID
	EntityMeta
	Tags []string
}

// GetID returns a GardenContainer's EntityID.
func (c GardenContainer) GetID() EntityID {
	return c.EntityID
}

// Merge merges a GardenContainer with another. Returns an error if trying to
// merge with another kind.
func (c *GardenContainer) Merge(e Entity) error {
	cc, ok := e.(*GardenContainer)
	if !ok {
		return fmt.Errorf("cannot merge GardenContainer with different kind %T", e)
	}

	return mergo.Merge(c, cc)
}

// DeepCopy returns a deep copy of the container.
func (c GardenContainer) DeepCopy() Entity {
	cp := deepcopy.Copy(c).(GardenContainer)
	return &cp
}

// String returns a string representation of a GardenContainer.
func (c GardenContainer) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprint(&sb, c.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, c.EntityMeta.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Container Info -----------")
	_, _ = fmt.Fprintln(&sb, "Tags:", sliceToString(c.Tags))

	return sb.String()
}

var _ Entity = &GardenContainer{}

// CollectorEvent is an event generated by a metadata collector, to be handled
// by the metadata store.
type CollectorEvent struct {
	Type   EventType
	Source Source
	Entity Entity
}

// Event represents a change to an entity.
type Event struct {
	// Type gives the type of this event.
	//
	// When Type is EventTypeSet, this represents an added or updated entity.
	// Multiple set events may be sent for a single entity.
	//
	// When Type is EventTypeUnset, this represents a removed entity.
	Type EventType

	// Entity is the entity involved in this event.  For an EventTypeSet event,
	// this may contain information "merged" from multiple sources.  For an
	// unset event it contains only an EntityID.
	//
	// For Type == EventTypeSet, this field can be cast unconditionally to the
	// concrete type corresponding to its kind (Entity.GetID().Kind).  For Type
	// == EventTypeUnset, only the Entity ID is available and such a cast will
	// fail.
	Entity Entity
}

// SubscriberPriority is a priority for subscribers to the store.  Subscribers
// are notified in order by their priority, with each notification blocking the
// next, so this allows control of which compoents are informed of changes in
// the store first.
type SubscriberPriority int

const (
	// TaggerPriority is the priority for the Tagger.  The Tagger must always
	// come first.
	TaggerPriority SubscriberPriority = iota

	// NormalPriority should be used by subscribers on which other components
	// do not depend.
	NormalPriority SubscriberPriority = iota
)

// EventBundle is a collection of events sent to Store subscribers.
//
// Subscribers are expected to respond to EventBundles quickly.  The Store will
// not move on to notify the next subscriber until the included channel Ch is
// closed.  Subscribers which need to update their state before other
// subscribers are notified should close this channel once those updates are
// complete.  Other subscribers should close the channel immediately.
// See the example for Store#Subscribe for details.
type EventBundle struct {
	// Events gives the events in this bundle.
	Events []Event

	// Ch should be closed once the subscriber has handled the event.
	Ch chan struct{}
}
