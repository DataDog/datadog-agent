// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

// Pod contains fields for unmarshalling a Pod
type Pod struct {
	Spec     Spec        `json:"spec,omitempty"`
	Status   Status      `json:"status,omitempty"`
	Metadata PodMetadata `json:"metadata,omitempty"`
}

// PodList contains fields for unmarshalling a PodList
type PodList struct {
	Items []*Pod `json:"items,omitempty"`
}

// PodMetadata contains fields for unmarshalling a pod's metadata
type PodMetadata struct {
	Name        string            `json:"name,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	ResVersion  string            `json:"resourceVersion,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Owners      []PodOwner        `json:"ownerReferences,omitempty"`
}

// PodOwner contains fields for unmarshalling a Pod.Metadata.Owners
type PodOwner struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
	ID   string `json:"uid,omitempty"`
}

// Spec contains fields for unmarshalling a Pod.Spec
type Spec struct {
	HostNetwork bool            `json:"hostNetwork,omitempty"`
	NodeName    string          `json:"nodeName,omitempty"`
	Containers  []ContainerSpec `json:"containers,omitempty"`
}

// ContainerSpec contains fields for unmarshalling a Pod.Spec.Containers
type ContainerSpec struct {
	Name  string              `json:"name"`
	Image string              `json:"image,omitempty"`
	Ports []ContainerPortSpec `json:"ports,omitempty"`
}

// ContainerSpec contains fields for unmarshalling a Pod.Spec.Containers.Ports
type ContainerPortSpec struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
}

// Status contains fields for unmarshalling a Pod.Status
type Status struct {
	Phase      string            `json:"phase,omitempty"`
	HostIP     string            `json:"hostIP,omitempty"`
	PodIP      string            `json:"podIP,omitempty"`
	Containers []ContainerStatus `json:"containerStatuses,omitempty"`
	Conditions []Conditions      `json:"conditions,omitempty"`
}

// Conditions contains fields for unmarshalling a Pod.Status.Conditions
type Conditions struct {
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
}

// ContainerStatus contains fields for unmarshalling a Pod.Status.Containers
type ContainerStatus struct {
	Name  string `json:"name,omitempty"`
	Image string `json:"image,omitempty"`
	ID    string `json:"containerID,omitempty"`
}
