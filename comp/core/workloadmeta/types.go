// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"io"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
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
// Typically there is only one instance, accessed via GetGlobalStore.

// Kind is the kind of an entity.
type Kind string

// Defined Kinds
const (
	KindContainer              Kind = "container"
	KindKubernetesPod          Kind = "kubernetes_pod"
	KindKubernetesNode         Kind = "kubernetes_node"
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
	panic("not called")
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
	panic("not called")
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
	panic("not called")
}

// String returns a string representation of ContainerImage.
func (c ContainerImage) String(verbose bool) string {
	panic("not called")
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
	panic("not called")
}

// ContainerPort is a port open in the container.
type ContainerPort struct {
	Name     string
	Port     int
	Protocol string
}

// String returns a string representation of ContainerPort.
func (c ContainerPort) String(verbose bool) string {
	panic("not called")
}

// ContainerResources is resources requests or limitations for a container
type ContainerResources struct {
	CPURequest    *float64 // Percentage 0-100*numCPU (aligned with CPU Limit from metrics provider)
	MemoryRequest *uint64  // Bytes
}

// String returns a string representation of ContainerPort.
func (cr ContainerResources) String(bool) string {
	panic("not called")
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
	panic("not called")
}

// Container is an Entity representing a containerized workload.
type Container struct {
	EntityID
	EntityMeta
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
}

// GetID implements Entity#GetID.
func (c Container) GetID() EntityID {
	panic("not called")
}

// Merge implements Entity#Merge.
func (c *Container) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (c Container) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String.
func (c Container) String(verbose bool) string {
	panic("not called")
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
	panic("not called")
}

// Merge implements Entity#Merge.
func (p *KubernetesPod) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (p KubernetesPod) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String.
func (p KubernetesPod) String(verbose bool) string {
	panic("not called")
}

// GetAllContainers returns init containers and containers.
func (p KubernetesPod) GetAllContainers() []OrchestratorContainer {
	panic("not called")
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
	panic("not called")
}

// KubernetesNode is an Entity representing a Kubernetes Node.
type KubernetesNode struct {
	EntityID
	EntityMeta
}

// GetID implements Entity#GetID.
func (n *KubernetesNode) GetID() EntityID {
	panic("not called")
}

// Merge implements Entity#Merge.
func (n *KubernetesNode) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (n KubernetesNode) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String
func (n KubernetesNode) String(verbose bool) string {
	panic("not called")
}

var _ Entity = &KubernetesNode{}

// Languages represents languages detected for containers of a pod
type Languages struct {
	ContainerLanguages     map[string][]languagemodels.Language
	InitContainerLanguages map[string][]languagemodels.Language
}

// KubernetesDeployment is an Entity representing a Kubernetes Deployment.
type KubernetesDeployment struct {
	EntityID
	Env     string
	Service string
	Version string

	// InjectableLanguages indicate containers languages that can be injected by the admission controller
	// These languages are determined by parsing the deployment annotations
	InjectableLanguages Languages

	// DetectedLanguages languages indicate containers languages detected and reported by the language
	// detection server.
	DetectedLanguages Languages
}

// GetID implements Entity#GetID.
func (d *KubernetesDeployment) GetID() EntityID {
	panic("not called")
}

// Merge implements Entity#Merge.
func (d *KubernetesDeployment) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (d KubernetesDeployment) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String
func (d KubernetesDeployment) String(verbose bool) string {
	panic("not called")
}

var _ Entity = &KubernetesDeployment{}

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
	panic("not called")
}

// Merge implements Entity#Merge.
func (t *ECSTask) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (t ECSTask) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String.
func (t ECSTask) String(verbose bool) string {
	panic("not called")
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
	panic("not called")
}

// Merge implements Entity#Merge.
func (i *ContainerImageMetadata) Merge(e Entity) error {
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (i ContainerImageMetadata) DeepCopy() Entity {
	panic("not called")
}

// String implements Entity#String.
func (i ContainerImageMetadata) String(verbose bool) string {
	panic("not called")
}

// String returns a string representation of ContainerImageLayer
func (layer ContainerImageLayer) String() string {
	panic("not called")
}

func printHistory(out io.Writer, history *v1.History) {
	panic("not called")
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
	panic("not called")
}

// DeepCopy implements Entity#DeepCopy.
func (p Process) DeepCopy() Entity {
	panic("not called")
}

// Merge implements Entity#Merge.
func (p *Process) Merge(e Entity) error {
	panic("not called")
}

// String implements Entity#String.
func (p Process) String(verbose bool) string {
	panic("not called")
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
	panic("not called")
}
