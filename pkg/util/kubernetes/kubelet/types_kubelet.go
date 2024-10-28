// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Pod contains fields for unmarshalling a Pod
type Pod struct {
	Spec     Spec        `json:"spec,omitempty"`
	Status   Status      `json:"status,omitempty"`
	Metadata PodMetadata `json:"metadata,omitempty"`
}

// PodList contains fields for unmarshalling a PodList
type PodList struct {
	Items        []*Pod `json:"items,omitempty"`
	ExpiredCount int
}

// PodMetadata contains fields for unmarshalling a pod's metadata
type PodMetadata struct {
	Name              string            `json:"name,omitempty"`
	UID               string            `json:"uid,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	ResVersion        string            `json:"resourceVersion,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Owners            []PodOwner        `json:"ownerReferences,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
}

// PodOwner contains fields for unmarshalling a Pod.Metadata.Owners
type PodOwner struct {
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	ID         string `json:"uid,omitempty"`
	Controller *bool  `json:"controller,omitempty"`
}

// Spec contains fields for unmarshalling a Pod.Spec
type Spec struct {
	HostNetwork       bool                    `json:"hostNetwork,omitempty"`
	NodeName          string                  `json:"nodeName,omitempty"`
	InitContainers    []ContainerSpec         `json:"initContainers,omitempty"`
	Containers        []ContainerSpec         `json:"containers,omitempty"`
	Volumes           []VolumeSpec            `json:"volumes,omitempty"`
	PriorityClassName string                  `json:"priorityClassName,omitempty"`
	SecurityContext   *PodSecurityContextSpec `json:"securityContext,omitempty"`
	RuntimeClassName  *string                 `json:"runtimeClassName,omitempty"`
	Tolerations       []Toleration            `json:"tolerations,omitempty"`
}

// PodSecurityContextSpec contains fields for unmarshalling a Pod.Spec.SecurityContext
type PodSecurityContextSpec struct {
	RunAsUser  int32 `json:"runAsUser,omitempty"`
	RunAsGroup int32 `json:"runAsGroup,omitempty"`
	FsGroup    int32 `json:"fsGroup,omitempty"`
}

// ContainerSpec contains fields for unmarshalling a Pod.Spec.Containers
type ContainerSpec struct {
	Name            string                        `json:"name"`
	Image           string                        `json:"image,omitempty"`
	Ports           []ContainerPortSpec           `json:"ports,omitempty"`
	ReadinessProbe  *ContainerProbe               `json:"readinessProbe,omitempty"`
	Env             []EnvVar                      `json:"env,omitempty"`
	SecurityContext *ContainerSecurityContextSpec `json:"securityContext,omitempty"`
	Resources       *ContainerResourcesSpec       `json:"resources,omitempty"`
}

// Toleration contains fields for unmarshalling a Pod.Spec.Tolerations
type Toleration struct {
	Key               string `json:"key,omitempty"`
	Operator          string `json:"operator,omitempty"`
	Value             string `json:"value,omitempty"`
	Effect            string `json:"effect,omitempty"`
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}

// ResourceName is the key to fields in in Pod.Spec.Containers.Resources
type ResourceName string

// Resources name
const (
	// Kubernetes GPU resource types by vendor as shown below
	// https://kubernetes.io/docs/tasks/manage-gpus/scheduling-gpus/
	ResourceGenericNvidiaGPU ResourceName = "nvidia.com/gpu"
	ResourcePrefixNvidiaMIG  ResourceName = "nvidia.com/mig"
	ResourcePrefixIntelGPU   ResourceName = "gpu.intel.com/"
	ResourcePrefixAMDGPU     ResourceName = "amd.com/"

	ResourceCPU              ResourceName = "cpu"
	ResourceMemory           ResourceName = "memory"
	ResourceStorage          ResourceName = "storage"
	ResourceEphemeralStorage ResourceName = "ephemeral-storage"
)

// GetGPUResourceNames returns the list of GPU resource names
func GetGPUResourceNames() []ResourceName {
	return []ResourceName{ResourcePrefixNvidiaMIG, ResourceGenericNvidiaGPU, ResourcePrefixIntelGPU, ResourcePrefixAMDGPU}
}

// ResourceList is the type of fields in Pod.Spec.Containers.Resources
type ResourceList map[ResourceName]resource.Quantity

// ContainerResourcesSpec contains fields for unmarshalling a Pod.Spec.Containers.Resources
type ContainerResourcesSpec struct {
	Requests ResourceList `json:"requests,omitempty"`
	Limits   ResourceList `json:"limits,omitempty"`
}

// ContainerPortSpec contains fields for unmarshalling a Pod.Spec.Containers.Ports
type ContainerPortSpec struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
}

// ContainerProbe contains fields for unmarshalling a Pod.Spec.Containers.ReadinessProbe
type ContainerProbe struct {
	InitialDelaySeconds int `json:"initialDelaySeconds"`
}

// ContainerSecurityContextSpec contains fields for unmarshalling a Pod.Spec.Containers.SecurityContext
type ContainerSecurityContextSpec struct {
	Capabilities   *CapabilitiesSpec   `json:"capabilities,omitempty"`
	Privileged     *bool               `json:"privileged,omitempty"`
	SeccompProfile *SeccompProfileSpec `json:"seccompProfile,omitempty"`
}

// CapabilitiesSpec contains fields for unmarshalling a Pod.Spec.Containers.SecurityContext.Capabilities
type CapabilitiesSpec struct {
	Add  []string `json:"add,omitempty"`
	Drop []string `json:"drop,omitempty"`
}

// SeccompProfileType is used for unmarshalling Pod.Spec.Containers.SecurityContext.SeccompProfile.Type
type SeccompProfileType string

// Seccomp profiles
const (
	SeccompProfileTypeUnconfined     SeccompProfileType = "Unconfined"
	SeccompProfileTypeRuntimeDefault SeccompProfileType = "RuntimeDefault"
	SeccompProfileTypeLocalhost      SeccompProfileType = "Localhost"
)

// SeccompProfileSpec contains fields for unmarshalling a Pod.Spec.Containers.SecurityContext.SeccompProfile
type SeccompProfileSpec struct {
	Type             SeccompProfileType `json:"type"`
	LocalhostProfile *string            `json:"localhostProfile,omitempty"`
}

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	// Name of the environment variable. Must be a C_IDENTIFIER.
	Name string `json:"name"`
	// Value of the environment variable.
	Value string `json:"value,omitempty"`
}

// VolumeSpec contains fields for unmarshalling a Pod.Spec.Volumes
type VolumeSpec struct {
	Name string `json:"name"`
	// Only try to retrieve persistent volume claim to tag statefulsets
	PersistentVolumeClaim *PersistentVolumeClaimSpec `json:"persistentVolumeClaim,omitempty"`
	Ephemeral             *EphemeralSpec             `json:"ephemeral,omitempty"`
}

// PersistentVolumeClaimSpec contains fields for unmarshalling a Pod.Spec.Volumes.PersistentVolumeClaim
type PersistentVolumeClaimSpec struct {
	ClaimName string `json:"claimName"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// EphemeralSpec contains fields for unmarshalling a Pod.Spec.Volumes.Ephemeral
type EphemeralSpec struct {
	VolumeClaimTemplate *VolumeClaimTemplateSpec `json:"volumeClaimTemplate,omitempty"`
}

// VolumeClaimTemplateSpec contains fields for unmarshalling a Pod.Spec.Volumes.Ephemeral.VolumeClaimTemplate
type VolumeClaimTemplateSpec struct {
	Metadata PodMetadata `json:"metadata,omitempty"`
}

// Status contains fields for unmarshalling a Pod.Status
type Status struct {
	Phase          string            `json:"phase,omitempty"`
	HostIP         string            `json:"hostIP,omitempty"`
	PodIP          string            `json:"podIP,omitempty"`
	Containers     []ContainerStatus `json:"containerStatuses,omitempty"`
	InitContainers []ContainerStatus `json:"initContainerStatuses,omitempty"`
	AllContainers  []ContainerStatus
	Conditions     []Conditions `json:"conditions,omitempty"`
	QOSClass       string       `json:"qosClass,omitempty"`
	StartTime      time.Time    `json:"startTime,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

// GetAllContainers returns the list of init and regular containers
// the list is created lazily assuming container statuses are not modified
func (s *Status) GetAllContainers() []ContainerStatus {
	return s.AllContainers
}

// Conditions contains fields for unmarshalling a Pod.Status.Conditions
type Conditions struct {
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
}

// ContainerStatus contains fields for unmarshalling a Pod.Status.Containers
type ContainerStatus struct {
	Name         string         `json:"name"`
	Image        string         `json:"image"`
	ImageID      string         `json:"imageID"`
	ID           string         `json:"containerID"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	State        ContainerState `json:"state"`
	LastState    ContainerState `json:"lastState"`
}

// IsPending returns if the container doesn't have an ID
func (c *ContainerStatus) IsPending() bool {
	return c.ID == ""
}

// IsTerminated returns if the container is in a terminated state
func (c *ContainerStatus) IsTerminated() bool {
	return c.State.Terminated != nil
}

// ContainerState holds a possible state of container.
// Only one of its members may be specified.
// If none of them is specified, the default one is ContainerStateWaiting.
type ContainerState struct {
	Waiting    *ContainerStateWaiting    `json:"waiting,omitempty"`
	Running    *ContainerStateRunning    `json:"running,omitempty"`
	Terminated *ContainerStateTerminated `json:"terminated,omitempty"`
}

// ContainerStateWaiting is a waiting state of a container.
type ContainerStateWaiting struct {
	Reason string `json:"reason"`
}

// ContainerStateRunning is a running state of a container.
type ContainerStateRunning struct {
	StartedAt time.Time `json:"startedAt"`
}

// ContainerStateTerminated is a terminated state of a container.
type ContainerStateTerminated struct {
	ExitCode   int32     `json:"exitCode"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Reason     string    `json:"reason"`
}
