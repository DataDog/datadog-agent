// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/mohae/deepcopy"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
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

	// GetKubernetesNode returns metadata about a Kubernetes node. It fetches
	// the entity with kind KindKubernetesNode and the given ID.
	GetKubernetesNode(id string) (*KubernetesNode, error)

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
}

// Kind is the kind of an entity.
type Kind string

// Defined Kinds
const (
	KindContainer              Kind = "container"
	KindKubernetesPod          Kind = "kubernetes_pod"
	KindKubernetesNode         Kind = "kubernetes_node"
	KindECSTask                Kind = "ecs_task"
	KindContainerImageMetadata Kind = "container_image_metadata"
	KindProcess                Kind = "process"
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

	// SourceRemoteWorkloadmeta represents entities detected by the remote
	// workloadmeta.
	SourceRemoteWorkloadmeta Source = "remote_workloadmeta"

	// SourceRemoteProcessCollector reprents processes entities detected
	// by the RemoteProcessCollector.
	SourceRemoteProcessCollector Source = "remote_process_collector"
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
	// EventTypeAll matches any event type. Should not be returned by
	// collectors, as it is only meant to be used in filters.
	EventTypeAll EventType = iota

	// EventTypeSet indicates that an entity has been added or updated.
	EventTypeSet

	// EventTypeUnset indicates that an entity has been removed.  If multiple
	// sources provide data for an entity, this message is only sent when the
	// last source stops providing that data.
	EventTypeUnset
)

// Entity represents a single unit of work being done that is of interest to
// the agent.
//
// This interface is implemented by several concrete types, and is typically
// cast to that concrete type to get detailed information.  The concrete type
// corresponds to the entity's type (GetID().Kind), and it is safe to make an
// unchecked cast.
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
	Registry  string
	ShortName string
	Tag       string
}

// NewContainerImage builds a ContainerImage from an image name and its id
func NewContainerImage(imageID string, imageName string) (ContainerImage, error) {
	image := ContainerImage{
		ID:      imageID,
		RawName: imageName,
		Name:    imageName,
	}

	name, registry, shortName, tag, err := containers.SplitImageName(imageName)
	if err != nil {
		return image, err
	}

	if tag == "" {
		tag = "latest"
	}

	image.Name = name
	image.Registry = registry
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
	// EnvVars are limited to variables included in pkg/util/containers/env_vars_filter.go
	EnvVars    map[string]string
	Hostname   string
	Image      ContainerImage
	NetworkIPs map[string]string
	PID        int
	Ports      []ContainerPort
	Runtime    ContainerRuntime
	State      ContainerState
	// CollectorTags represent tags coming from the collector itself
	// and that it would be impossible to compute later on
	CollectorTags   []string
	Owner           *EntityID
	SecurityContext *ContainerSecurityContext
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

	return merge(c, cc)
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
		_, _ = fmt.Fprintln(&sb, "Allowed env variables:", filterAndFormatEnvVars(c.EnvVars))
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

	if c.SecurityContext != nil {
		_, _ = fmt.Fprintln(&sb, "----------- Security Context -----------")
		if c.SecurityContext.Capabilities != nil {
			_, _ = fmt.Fprintln(&sb, "----------- Capabilities -----------")
			_, _ = fmt.Fprintln(&sb, "Add:", c.SecurityContext.Capabilities.Add)
			_, _ = fmt.Fprintln(&sb, "Drop:", c.SecurityContext.Capabilities.Drop)
		}

		_, _ = fmt.Fprintln(&sb, "Privileged:", c.SecurityContext.Privileged)
		if c.SecurityContext.SeccompProfile != nil {
			_, _ = fmt.Fprintln(&sb, "----------- Seccomp Profile -----------")
			_, _ = fmt.Fprintln(&sb, "Type:", c.SecurityContext.SeccompProfile.Type)
			if c.SecurityContext.SeccompProfile.Type == SeccompProfileTypeLocalhost {
				_, _ = fmt.Fprintln(&sb, "Localhost Profile:", c.SecurityContext.SeccompProfile.LocalhostProfile)
			}
		}
	}

	return sb.String()
}

// PodSecurityContext is the Security Context of a Kubernete pod
type PodSecurityContext struct {
	RunAsUser  int32
	RunAsGroup int32
	FsGroup    int32
}

// ContainerSecurityContext is the Security Context of a Container
type ContainerSecurityContext struct {
	*Capabilities
	Privileged     bool
	SeccompProfile *SeccompProfile
}

type Capabilities struct {
	Add  []string
	Drop []string
}

// SeccompProfileType is the type of seccomp profile used
type SeccompProfileType string

const (
	SeccompProfileTypeUnconfined     SeccompProfileType = "Unconfined"
	SeccompProfileTypeRuntimeDefault SeccompProfileType = "RuntimeDefault"
	SeccompProfileTypeLocalhost      SeccompProfileType = "Localhost"
)

// SeccompProfileSpec contains fields for unmarshalling a Pod.Spec.Containers.SecurityContext.SeccompProfile
type SeccompProfile struct {
	Type             SeccompProfileType
	LocalhostProfile string
}

var _ Entity = &Container{}

// ContainerFilterFunc is a function used to filter containers.
type ContainerFilterFunc func(container *Container) bool

// ProcessFilterFunc is a function used to filter processes.
type ProcessFilterFunc func(process *Process) bool

// GetRunningContainers is a function that evaluates to true for running containers.
var GetRunningContainers ContainerFilterFunc = func(container *Container) bool { return container.State.Running }

// KubernetesPod is an Entity representing a Kubernetes Pod.
type KubernetesPod struct {
	EntityID
	EntityMeta
	Owners                     []KubernetesPodOwner
	PersistentVolumeClaimNames []string
	InitContainers             []OrchestratorContainer
	Containers                 []OrchestratorContainer
	Ready                      bool
	Phase                      string
	IP                         string
	PriorityClass              string
	QOSClass                   string
	KubeServices               []string
	NamespaceLabels            map[string]string
	FinishedAt                 time.Time
	SecurityContext            *PodSecurityContext
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

	return merge(p, pp)
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

	if len(p.InitContainers) > 0 {
		_, _ = fmt.Fprintln(&sb, "----------- Init Containers -----------")
		for _, c := range p.InitContainers {
			_, _ = fmt.Fprint(&sb, c.String(verbose))
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
		_, _ = fmt.Fprintln(&sb, "QOS Class:", p.QOSClass)
		_, _ = fmt.Fprintln(&sb, "PVCs:", sliceToString(p.PersistentVolumeClaimNames))
		_, _ = fmt.Fprintln(&sb, "Kube Services:", sliceToString(p.KubeServices))
		_, _ = fmt.Fprintln(&sb, "Namespace Labels:", mapToString(p.NamespaceLabels))
		if !p.FinishedAt.IsZero() {
			_, _ = fmt.Fprintln(&sb, "Finished At:", p.FinishedAt)
		}
	}

	if p.SecurityContext != nil {
		_, _ = fmt.Fprintln(&sb, "----------- Pod Security Context -----------")
		_, _ = fmt.Fprintln(&sb, "RunAsUser:", p.SecurityContext.RunAsUser)
		_, _ = fmt.Fprintln(&sb, "RunAsGroup:", p.SecurityContext.RunAsGroup)
		_, _ = fmt.Fprintln(&sb, "FsGroup:", p.SecurityContext.FsGroup)
	}

	return sb.String()
}

// GetAllContainers returns init containers and containers.
func (p KubernetesPod) GetAllContainers() []OrchestratorContainer {
	return append(p.InitContainers, p.Containers...)
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

// KubernetesNode is an Entity representing a Kubernetes Node.
type KubernetesNode struct {
	EntityID
	EntityMeta
}

// GetID implements Entity#GetID.
func (n *KubernetesNode) GetID() EntityID {
	return n.EntityID
}

// Merge implements Entity#Merge.
func (n *KubernetesNode) Merge(e Entity) error {
	nn, ok := e.(*KubernetesNode)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesNode with different kind %T", e)
	}

	return merge(n, nn)
}

// DeepCopy implements Entity#DeepCopy.
func (n KubernetesNode) DeepCopy() Entity {
	cn := deepcopy.Copy(n).(KubernetesNode)
	return &cn
}

// String implements Entity#String
func (n KubernetesNode) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, n.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, n.EntityMeta.String(verbose))

	return sb.String()
}

var _ Entity = &KubernetesNode{}

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

	return merge(t, tt)
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

// ContainerImageMetadata is an Entity that represents container image metadata
type ContainerImageMetadata struct {
	EntityID
	EntityMeta
	RepoTags     []string
	RepoDigests  []string
	MediaType    string
	SizeBytes    int64
	OS           string
	OSVersion    string
	Architecture string
	Variant      string
	Layers       []ContainerImageLayer
	SBOM         *SBOM
}

// ContainerImageLayer represents a layer of a container image
type ContainerImageLayer struct {
	MediaType string
	Digest    string
	SizeBytes int64
	URLs      []string
	History   v1.History
}

// SBOM represents the Software Bill Of Materials (SBOM) of a container
type SBOM struct {
	CycloneDXBOM       *cyclonedx.BOM
	GenerationTime     time.Time
	GenerationDuration time.Duration
}

// GetID implements Entity#GetID.
func (i ContainerImageMetadata) GetID() EntityID {
	return i.EntityID
}

// Merge implements Entity#Merge.
func (i *ContainerImageMetadata) Merge(e Entity) error {
	otherImage, ok := e.(*ContainerImageMetadata)
	if !ok {
		return fmt.Errorf("cannot merge ContainerImageMetadata with different kind %T", e)
	}

	return merge(i, otherImage)
}

// DeepCopy implements Entity#DeepCopy.
func (i ContainerImageMetadata) DeepCopy() Entity {
	cp := deepcopy.Copy(i).(ContainerImageMetadata)
	return &cp
}

// String implements Entity#String.
func (i ContainerImageMetadata) String(verbose bool) string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprint(&sb, i.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, i.EntityMeta.String(verbose))

	_, _ = fmt.Fprintln(&sb, "Repo tags:", i.RepoTags)
	_, _ = fmt.Fprintln(&sb, "Repo digests:", i.RepoDigests)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Media Type:", i.MediaType)
		_, _ = fmt.Fprintln(&sb, "Size in bytes:", i.SizeBytes)
		_, _ = fmt.Fprintln(&sb, "OS:", i.OS)
		_, _ = fmt.Fprintln(&sb, "OS Version:", i.OSVersion)
		_, _ = fmt.Fprintln(&sb, "Architecture:", i.Architecture)
		_, _ = fmt.Fprintln(&sb, "Variant:", i.Variant)

		if i.SBOM != nil {
			_, _ = fmt.Fprintf(&sb, "SBOM: stored. Generated in: %.2f seconds\n", i.SBOM.GenerationDuration.Seconds())
		} else {
			_, _ = fmt.Fprintln(&sb, "SBOM: not stored")
		}

		_, _ = fmt.Fprintln(&sb, "----------- Layers -----------")
		for _, layer := range i.Layers {
			_, _ = fmt.Fprintln(&sb, layer)
		}
	}

	return sb.String()
}

// String returns a string representation of ContainerImageLayer
func (layer ContainerImageLayer) String() string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "Media Type:", layer.MediaType)
	_, _ = fmt.Fprintln(&sb, "Digest:", layer.Digest)
	_, _ = fmt.Fprintln(&sb, "Size in bytes:", layer.SizeBytes)
	_, _ = fmt.Fprintln(&sb, "URLs:", layer.URLs)

	printHistory(&sb, layer.History)

	return sb.String()
}

func printHistory(out io.Writer, history v1.History) {
	_, _ = fmt.Fprintln(out, "History:")
	_, _ = fmt.Fprintln(out, "- createdAt:", history.Created)
	_, _ = fmt.Fprintln(out, "- createdBy:", history.CreatedBy)
	_, _ = fmt.Fprintln(out, "- comment:", history.Comment)
	_, _ = fmt.Fprintln(out, "- emptyLayer:", history.EmptyLayer)
}

var _ Entity = &ContainerImageMetadata{}

type Process struct {
	EntityID // EntityID is the PID for now
	EntityMeta

	NsPid        int32
	ContainerId  string
	CreationTime time.Time
	Language     *languagemodels.Language
}

var _ Entity = &Process{}

// GetID implements Entity#GetID.
func (p Process) GetID() EntityID {
	return p.EntityID
}

// DeepCopy implements Entity#DeepCopy.
func (p Process) DeepCopy() Entity {
	cp := deepcopy.Copy(p).(Process)
	return &cp
}

// Merge implements Entity#Merge.
func (p *Process) Merge(e Entity) error {
	otherProcess, ok := e.(*Process)
	if !ok {
		return fmt.Errorf("cannot merge ProcessMetadata with different kind %T", e)
	}

	return merge(p, otherProcess)
}

// String implements Entity#String.
func (p Process) String(verbose bool) string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, "PID:", p.EntityID.ID)
	_, _ = fmt.Fprintln(&sb, "Namespace PID:", p.NsPid)
	_, _ = fmt.Fprintln(&sb, "Container ID:", p.ContainerId)
	_, _ = fmt.Fprintln(&sb, "Creation time:", p.CreationTime)
	_, _ = fmt.Fprintln(&sb, "Language:", p.Language.Name)

	return sb.String()
}

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

	// ConfigProviderPriority is the priority for the AD Config Provider.
	// This should come before other subscribers so that config provided by
	// entities is available to those other subscribers.
	ConfigProviderPriority SubscriberPriority = iota

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
