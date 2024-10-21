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
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
)

// TODO(component): it might make more sense to move the store into its own
//                  component and provide a bundle for workloadmeta - we can
//                  refine that later.
//
// former `type Store interface` is now the component's interface; current references
// to workloadmeta.Store have been changed to reference the component itself.
//
// Store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
//

// Kind is the kind of an entity.
type Kind string

// Defined Kinds
const (
	KindContainer              Kind = "container"
	KindKubernetesPod          Kind = "kubernetes_pod"
	KindKubernetesMetadata     Kind = "kubernetes_metadata"
	KindKubernetesDeployment   Kind = "kubernetes_deployment"
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

	// SourceLanguageDetectionServer represents container languages
	// detected by node agents
	SourceLanguageDetectionServer Source = "language_detection_server"

	// SourceHost represents entities detected by the host such as host tags.
	SourceHost Source = "host"

	// SourceLocalProcessCollector reprents processes entities detected
	// by the LocalProcessCollector.
	SourceLocalProcessCollector Source = "local_process_collector"
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

// ContainerRuntimeFlavor is the container runtime with respect to the OCI spect
type ContainerRuntimeFlavor string

// Defined ContainerRuntimeFlavors
const (
	ContainerRuntimeFlavorDefault ContainerRuntimeFlavor = ""
	ContainerRuntimeFlavorKata    ContainerRuntimeFlavor = "kata"
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

// AgentType defines the workloadmeta agent type
type AgentType uint8

// Define types of agent for catalog
const (
	NodeAgent AgentType = 1 << iota
	ClusterAgent
	ProcessAgent
	Remote
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

// SBOMStatus is the status of a SBOM
type SBOMStatus string

const (
	// Pending is the status when the image was not scanned
	Pending SBOMStatus = "Pending"
	// Success is the status when the image was scanned
	Success SBOMStatus = "Success"
	// Failed is the status when the scan failed
	Failed SBOMStatus = "Failed"
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
		_, _ = fmt.Fprintln(&sb, "Annotations:", mapToScrubbedJSONString(e.Annotations))
		_, _ = fmt.Fprintln(&sb, "Labels:", mapToScrubbedJSONString(e.Labels))
	}

	return sb.String()
}

// ContainerImage is the an image used by a container.
// For historical reason, The imageId from containerd runtime and kubernetes refer to different fields.
// For containerd, it is the digest of the image config.
// For kubernetes, it referres to repo digest of the image (at least before CRI-O v1.28)
// See https://github.com/kubernetes/kubernetes/issues/46255
// To avoid confusion, an extra field of repo digest is added to the struct, if it is available, it
// will also be added to the container tags in tagger.
type ContainerImage struct {
	ID         string
	RawName    string
	Name       string
	Registry   string
	ShortName  string
	Tag        string
	RepoDigest string
}

// NewContainerImage builds a ContainerImage from an image name and its id
func NewContainerImage(imageID string, imageName string) (ContainerImage, error) {
	image := ContainerImage{
		ID:      imageID,
		RawName: imageName,
		Name:    imageName,
	}

	name, registry, shortName, tag, err := pkgcontainersimage.SplitImageName(imageName)
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
		_, _ = fmt.Fprintln(&sb, "Repo Digest:", c.RepoDigest)
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
	ExitCode   *int64
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
	HostPort uint16
}

// String returns a string representation of ContainerPort.
func (c ContainerPort) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Port:", c.Port)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Name:", c.Name)
		_, _ = fmt.Fprintln(&sb, "Protocol:", c.Protocol)
		_, _ = fmt.Fprintln(&sb, "Host Port:", c.HostPort)
	}

	return sb.String()
}

// ContainerNetwork is the network attached to the container.
type ContainerNetwork struct {
	NetworkMode   string
	IPv4Addresses []string
	IPv6Addresses []string
}

// String returns a string representation of ContainerPort.
func (c ContainerNetwork) String(_ bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Network Mode:", c.NetworkMode)
	_, _ = fmt.Fprintln(&sb, "IPv4 Addresses:", c.IPv4Addresses)
	_, _ = fmt.Fprintln(&sb, "IPv6 Addresses:", c.IPv6Addresses)

	return sb.String()
}

// ContainerVolume is a volume mounted in the container.
type ContainerVolume struct {
	Name        string
	Source      string
	Destination string
}

// String returns a string representation of ContainerVolume.
func (c ContainerVolume) String(_ bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Name:", c.Name)
	_, _ = fmt.Fprintln(&sb, "Source:", c.Source)
	_, _ = fmt.Fprintln(&sb, "Destination:", c.Destination)

	return sb.String()
}

// ContainerHealthStatus is the health status of a container
type ContainerHealthStatus struct {
	Status   string
	Since    *time.Time
	ExitCode *int64
	Output   string
}

// String returns a string representation of ContainerHealthStatus.
func (c ContainerHealthStatus) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Status:", c.Status)

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Since:", c.Since)
		_, _ = fmt.Fprintln(&sb, "ExitCode:", c.ExitCode)
		_, _ = fmt.Fprintln(&sb, "Output:", c.Output)
	}

	return sb.String()
}

// ContainerResources is resources requests or limitations for a container
type ContainerResources struct {
	GPURequest    *uint64 // Number of GPUs
	GPULimit      *uint64
	GPUVendorList []string // The type of GPU requested (eg. nvidia, amd, intel)
	CPURequest    *float64 // Percentage 0-100*numCPU (aligned with CPU Limit from metrics provider)
	CPULimit      *float64
	MemoryRequest *uint64 // Bytes
	MemoryLimit   *uint64
}

// String returns a string representation of ContainerPort.
func (cr ContainerResources) String(bool) string {
	var sb strings.Builder
	if cr.CPURequest != nil {
		_, _ = fmt.Fprintln(&sb, "TargetCPUUsage:", *cr.CPURequest)
	}
	if cr.CPULimit != nil {
		_, _ = fmt.Fprintln(&sb, "TargetCPULimit:", *cr.CPULimit)
	}
	if cr.MemoryRequest != nil {
		_, _ = fmt.Fprintln(&sb, "TargetMemoryUsage:", *cr.MemoryRequest)
	}
	if cr.MemoryLimit != nil {
		_, _ = fmt.Fprintln(&sb, "TargetMemoryLimit:", *cr.MemoryLimit)
	}
	if cr.GPUVendorList != nil {
		_, _ = fmt.Fprintln(&sb, "GPUVendor:", cr.GPUVendorList)
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

// ECSContainer is a reference to a container running in ECS
type ECSContainer struct {
	DisplayName   string
	Networks      []ContainerNetwork
	Volumes       []ContainerVolume
	Health        *ContainerHealthStatus
	DesiredStatus string
	KnownStatus   string
	Type          string
	LogDriver     string
	LogOptions    map[string]string
	ContainerARN  string
	Snapshotter   string
}

// String returns a string representation of ECSContainer.
func (e ECSContainer) String(verbose bool) string {
	var sb strings.Builder

	if len(e.Volumes) > 0 {
		_, _ = fmt.Fprintln(&sb, "----------- Volumes -----------")
		for _, v := range e.Volumes {
			_, _ = fmt.Fprint(&sb, v.String(verbose))
		}
	}

	if len(e.Networks) > 0 {
		_, _ = fmt.Fprintln(&sb, "----------- Networks -----------")
		for _, n := range e.Networks {
			_, _ = fmt.Fprint(&sb, n.String(verbose))
		}
	}

	if e.Health != nil {
		_, _ = fmt.Fprintln(&sb, "----------- ECS Container Health -----------")
		_, _ = fmt.Fprint(&sb, e.Health.String(verbose))
	}
	_, _ = fmt.Fprintln(&sb, "----------- ECS Container Priorities -----------")
	_, _ = fmt.Fprintln(&sb, "DesiredStatus:", e.DesiredStatus)
	_, _ = fmt.Fprintln(&sb, "KnownStatus:", e.KnownStatus)
	_, _ = fmt.Fprintln(&sb, "Type:", e.Type)
	_, _ = fmt.Fprintln(&sb, "LogDriver:", e.LogDriver)
	_, _ = fmt.Fprintln(&sb, "LogOptions:", e.LogOptions)
	_, _ = fmt.Fprintln(&sb, "Snapshotter:", e.Snapshotter)

	return sb.String()
}

// Container is an Entity representing a containerized workload.
type Container struct {
	EntityID
	EntityMeta
	// ECSContainer contains properties specific to container running in ECS
	*ECSContainer
	// EnvVars are limited to variables included in pkg/util/containers/env_vars_filter.go
	EnvVars       map[string]string
	Hostname      string
	Image         ContainerImage
	NetworkIPs    map[string]string
	PID           int
	Ports         []ContainerPort
	Runtime       ContainerRuntime
	RuntimeFlavor ContainerRuntimeFlavor
	State         ContainerState
	// CollectorTags represent tags coming from the collector itself
	// and that it would be impossible to compute later on
	CollectorTags   []string
	Owner           *EntityID
	SecurityContext *ContainerSecurityContext
	Resources       ContainerResources
	// CgroupPath is a path to the cgroup of the container.
	// It can be relative to the cgroup parent.
	// Linux only.
	CgroupPath string
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
	err := merge(c, cc)
	if cc.ECSContainer != nil {
		ec := deepcopy.Copy(*cc.ECSContainer).(ECSContainer)
		cc.ECSContainer = &ec
	}
	return err
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
	_, _ = fmt.Fprintln(&sb, "RuntimeFlavor:", c.RuntimeFlavor)
	_, _ = fmt.Fprint(&sb, c.State.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Resources -----------")
	_, _ = fmt.Fprint(&sb, c.Resources.String(verbose))

	if verbose {
		_, _ = fmt.Fprintln(&sb, "Hostname:", c.Hostname)
		_, _ = fmt.Fprintln(&sb, "Network IPs:", mapToString(c.NetworkIPs))
		_, _ = fmt.Fprintln(&sb, "PID:", c.PID)
		_, _ = fmt.Fprintln(&sb, "Cgroup path:", c.CgroupPath)
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

	if c.ECSContainer != nil {
		_, _ = fmt.Fprint(&sb, c.ECSContainer.String(verbose))
	}

	return sb.String()
}

// PodSecurityContext is the Security Context of a Kubernetes pod
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

// Capabilities is the capabilities a certain Container security context is capable of
type Capabilities struct {
	Add  []string
	Drop []string
}

// SeccompProfileType is the type of seccomp profile used
type SeccompProfileType string

// Seccomp profile types
const (
	SeccompProfileTypeUnconfined     SeccompProfileType = "Unconfined"
	SeccompProfileTypeRuntimeDefault SeccompProfileType = "RuntimeDefault"
	SeccompProfileTypeLocalhost      SeccompProfileType = "Localhost"
)

// SeccompProfile contains fields for unmarshalling a Pod.Spec.Containers.SecurityContext.SeccompProfile
type SeccompProfile struct {
	Type             SeccompProfileType
	LocalhostProfile string
}

var _ Entity = &Container{}

// GetRunningContainers is a function that evaluates to true for running containers.
var GetRunningContainers EntityFilterFunc[*Container] = func(container *Container) bool { return container.State.Running }

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
	GPUVendorList              []string
	RuntimeClass               string
	KubeServices               []string
	NamespaceLabels            map[string]string
	NamespaceAnnotations       map[string]string
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
		_, _ = fmt.Fprintln(&sb, "GPU Vendor:", p.GPUVendorList)
		_, _ = fmt.Fprintln(&sb, "Runtime Class:", p.RuntimeClass)
		_, _ = fmt.Fprintln(&sb, "PVCs:", sliceToString(p.PersistentVolumeClaimNames))
		_, _ = fmt.Fprintln(&sb, "Kube Services:", sliceToString(p.KubeServices))
		_, _ = fmt.Fprintln(&sb, "Namespace Labels:", mapToString(p.NamespaceLabels))
		_, _ = fmt.Fprintln(&sb, "Namespace Annotations:", mapToString(p.NamespaceAnnotations))
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

// KubeMetadataEntityID is a unique ID for Kube Metadata Entity
type KubeMetadataEntityID string

// KubernetesMetadata is an Entity representing kubernetes resource metadata
type KubernetesMetadata struct {
	EntityID
	EntityMeta
	GVR *schema.GroupVersionResource
}

// GetID implements Entity#GetID.
func (m *KubernetesMetadata) GetID() EntityID {
	return m.EntityID
}

// Merge implements Entity#Merge.
func (m *KubernetesMetadata) Merge(e Entity) error {
	mm, ok := e.(*KubernetesMetadata)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesMetadata with different kind %T", e)
	}

	return merge(m, mm)
}

// DeepCopy implements Entity#DeepCopy.
func (m KubernetesMetadata) DeepCopy() Entity {
	cm := deepcopy.Copy(m).(KubernetesMetadata)
	return &cm
}

// String implements Entity#String
func (m *KubernetesMetadata) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, m.EntityID.String(verbose))

	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, m.EntityMeta.String(verbose))

	if verbose {
		_, _ = fmt.Fprintln(&sb, "----------- Resource -----------")
		_, _ = fmt.Fprintln(&sb, m.GVR.String())
	}

	return sb.String()
}

var _ Entity = &KubernetesMetadata{}

// KubernetesDeployment is an Entity representing a Kubernetes Deployment.
type KubernetesDeployment struct {
	EntityID
	EntityMeta
	Env     string
	Service string
	Version string

	// InjectableLanguages indicate containers languages that can be injected by the admission controller
	// These languages are determined by parsing the deployment annotations
	InjectableLanguages langUtil.ContainersLanguages

	// DetectedLanguages languages indicate containers languages detected and reported by the language
	// detection server.
	DetectedLanguages langUtil.ContainersLanguages
}

// GetID implements Entity#GetID.
func (d *KubernetesDeployment) GetID() EntityID {
	return d.EntityID
}

// Merge implements Entity#Merge.
func (d *KubernetesDeployment) Merge(e Entity) error {
	dd, ok := e.(*KubernetesDeployment)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesDeployment with different kind %T", e)
	}

	return merge(d, dd)
}

// DeepCopy implements Entity#DeepCopy.
func (d KubernetesDeployment) DeepCopy() Entity {
	cd := deepcopy.Copy(d).(KubernetesDeployment)
	return &cd
}

// String implements Entity#String
func (d KubernetesDeployment) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, d.EntityID.String(verbose))
	_, _ = fmt.Fprintln(&sb, "----------- Entity Meta -----------")
	_, _ = fmt.Fprint(&sb, d.EntityMeta.String(verbose))
	_, _ = fmt.Fprintln(&sb, "----------- Unified Service Tagging -----------")
	_, _ = fmt.Fprintln(&sb, "Env :", d.Env)
	_, _ = fmt.Fprintln(&sb, "Service :", d.Service)
	_, _ = fmt.Fprintln(&sb, "Version :", d.Version)

	langPrinter := func(containersLanguages langUtil.ContainersLanguages) {
		initContainersInfo := make([]string, 0, len(containersLanguages))
		containersInfo := make([]string, 0, len(containersLanguages))

		for container, languages := range containersLanguages {
			var langSb strings.Builder

			for lang := range languages {

				if langSb.Len() != 0 {
					_, _ = langSb.WriteString(",")
				}
				_, _ = langSb.WriteString(string(lang))
			}

			if container.Init {
				initContainersInfo = append(initContainersInfo, fmt.Sprintf("InitContainer %s=>[%s]\n", container.Name, langSb.String()))
			} else {
				containersInfo = append(initContainersInfo, fmt.Sprintf("Container %s=>[%s]\n", container.Name, langSb.String()))
			}

			for _, info := range initContainersInfo {
				_, _ = fmt.Fprint(&sb, info)
			}

			for _, info := range containersInfo {
				_, _ = fmt.Fprint(&sb, info)
			}
		}
	}
	_, _ = fmt.Fprintln(&sb, "----------- Injectable Languages -----------")
	langPrinter(d.InjectableLanguages)

	_, _ = fmt.Fprintln(&sb, "----------- Detected Languages -----------")
	langPrinter(d.DetectedLanguages)
	return sb.String()
}

var _ Entity = &KubernetesDeployment{}

// ECSTaskKnownStatusStopped is the known status of an ECS task that has stopped.
const ECSTaskKnownStatusStopped = "STOPPED"

// MapTags is a map of tags
type MapTags map[string]string

// ECSTask is an Entity representing an ECS Task.
type ECSTask struct {
	EntityID
	EntityMeta
	Tags                    MapTags
	ContainerInstanceTags   MapTags
	ClusterName             string
	AWSAccountID            int
	Region                  string
	AvailabilityZone        string
	Family                  string
	Version                 string
	DesiredStatus           string
	KnownStatus             string
	PullStartedAt           *time.Time
	PullStoppedAt           *time.Time
	ExecutionStoppedAt      *time.Time
	VPCID                   string
	ServiceName             string
	EphemeralStorageMetrics map[string]int64
	Limits                  map[string]float64
	LaunchType              ECSLaunchType
	Containers              []OrchestratorContainer
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
		_, _ = fmt.Fprintln(&sb, "AWS Account ID:", t.AWSAccountID)
		_, _ = fmt.Fprintln(&sb, "Desired Status:", t.DesiredStatus)
		_, _ = fmt.Fprintln(&sb, "Known Status:", t.KnownStatus)
		_, _ = fmt.Fprintln(&sb, "VPC ID:", t.VPCID)
		_, _ = fmt.Fprintln(&sb, "Ephemeral Storage Metrics:", t.EphemeralStorageMetrics)
		_, _ = fmt.Fprintln(&sb, "Limits:", t.Limits)
		if t.PullStartedAt != nil {
			_, _ = fmt.Fprintln(&sb, "Pull Started At:", *t.PullStartedAt)
		}
		if t.PullStoppedAt != nil {
			_, _ = fmt.Fprintln(&sb, "Pull Stopped At:", *t.PullStoppedAt)
		}
		if t.ExecutionStoppedAt != nil {
			_, _ = fmt.Fprintln(&sb, "Execution Stopped At:", *t.ExecutionStoppedAt)
		}
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
	History   *v1.History
}

// SBOM represents the Software Bill Of Materials (SBOM) of a container
type SBOM struct {
	CycloneDXBOM       *cyclonedx.BOM
	GenerationTime     time.Time
	GenerationDuration time.Duration
	Status             SBOMStatus
	Error              string // needs to be stored as a string otherwise the merge() will favor the nil value
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

		_, _ = fmt.Fprintln(&sb, "----------- SBOM -----------")
		_, _ = fmt.Fprintln(&sb, "Status:", i.SBOM.Status)
		switch i.SBOM.Status {
		case Success:
			_, _ = fmt.Fprintf(&sb, "Generated in: %.2f seconds\n", i.SBOM.GenerationDuration.Seconds())
		case Failed:
			_, _ = fmt.Fprintf(&sb, "Error: %s\n", i.SBOM.Error)
		default:
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

func printHistory(out io.Writer, history *v1.History) {
	if history == nil {
		_, _ = fmt.Fprintln(out, "History is nil")
		return
	}

	_, _ = fmt.Fprintln(out, "History:")
	_, _ = fmt.Fprintln(out, "- createdAt:", history.Created)
	_, _ = fmt.Fprintln(out, "- createdBy:", history.CreatedBy)
	_, _ = fmt.Fprintln(out, "- comment:", history.Comment)
	_, _ = fmt.Fprintln(out, "- emptyLayer:", history.EmptyLayer)
}

var _ Entity = &ContainerImageMetadata{}

// Process is an Entity that represents a process
type Process struct {
	EntityID // EntityID.ID is the PID

	NsPid        int32
	ContainerID  string
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
func (p Process) String(_ bool) string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprintln(&sb, "PID:", p.EntityID.ID)
	_, _ = fmt.Fprintln(&sb, "Namespace PID:", p.NsPid)
	_, _ = fmt.Fprintln(&sb, "Container ID:", p.ContainerID)
	_, _ = fmt.Fprintln(&sb, "Creation time:", p.CreationTime)
	_, _ = fmt.Fprintln(&sb, "Language:", p.Language.Name)

	return sb.String()
}

// HostTags is an Entity that represents host tags
type HostTags struct {
	EntityID

	HostTags []string
}

var _ Entity = &HostTags{}

// GetID implements Entity#GetID.
func (p HostTags) GetID() EntityID {
	return p.EntityID
}

// DeepCopy implements Entity#DeepCopy.
func (p HostTags) DeepCopy() Entity {
	cp := deepcopy.Copy(p).(HostTags)
	return &cp
}

// Merge implements Entity#Merge.
func (p *HostTags) Merge(e Entity) error {
	otherHost, ok := e.(*HostTags)
	if !ok {
		return fmt.Errorf("cannot merge Host metadata with different kind %T", e)
	}

	return merge(p, otherHost)
}

// String implements Entity#String.
func (p HostTags) String(verbose bool) string {
	var sb strings.Builder

	_, _ = fmt.Fprintln(&sb, "----------- Entity ID -----------")
	_, _ = fmt.Fprint(&sb, p.EntityID.String(verbose))
	_, _ = fmt.Fprintln(&sb, "Host Tags:", sliceToString(p.HostTags))

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

// Acknowledge acknowledges that the subscriber has handled the event.
func (e EventBundle) Acknowledge() {
	if e.Ch != nil {
		close(e.Ch)
	}
}

// InitHelper this should be provided as a helper to allow passing the component into
// the inithook for additional start-time configutation.
type InitHelper func(context.Context, Component, config.Component) error
