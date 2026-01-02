// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides CEL-based service name calculation.
//
// These types define the schema exposed to CEL expressions. They are used
// at runtime to map workloadmeta entities to CEL input variables, NOT for
// compile-time type checking (we use DynType for flexibility).
//
// Example CEL expressions:
//   - container.labels["tags.datadoghq.com/my-app.service"]
//   - process.binary.name.startsWith("java")
//   - pod.ownerref.name
package servicenaming

// CELInput is the input structure for CEL evaluation.
// It contains the context for service name calculation.
type CELInput struct {
	Process   *ProcessCEL   `cel:"process"`
	Container *ContainerCEL `cel:"container"`
	Pod       *PodCEL       `cel:"pod"`
}

// ProcessCEL represents a process in the CEL environment.
// Maps from workloadmeta.Process.
type ProcessCEL struct {
	// Pid is the process ID
	Pid int32 `cel:"pid"`

	// Cmd is the full command line as a single string (joined from cmdline args)
	Cmd string `cel:"cmd"`

	// Cmdline is the command line as a list of arguments
	Cmdline []string `cel:"cmdline"`

	// Binary contains information about the executable
	Binary BinaryCEL `cel:"binary"`

	// User is the username running the process (resolved from UID if possible)
	User string `cel:"user"`

	// Cwd is the current working directory
	Cwd string `cel:"cwd"`

	// Container is the container this process runs in (nil if not containerized)
	Container *ContainerCEL `cel:"container"`
}

// BinaryCEL represents binary/executable information in the CEL environment.
type BinaryCEL struct {
	// Name is the binary name (basename of exe path)
	Name string `cel:"name"`

	// Path is the full path to the executable
	Path string `cel:"path"`
}

// ContainerCEL represents a container in the CEL environment.
// Maps from workloadmeta.Container.
type ContainerCEL struct {
	// ID is the container ID
	ID string `cel:"id"`

	// Name is the container name
	Name string `cel:"name"`

	// Image contains image information
	Image ImageCEL `cel:"image"`

	// Labels are container labels (includes k8s annotations propagated as labels)
	// Access UST tags via: container.labels["tags.datadoghq.com/my-app.service"]
	Labels map[string]string `cel:"labels"`

	// Envs are environment variables (filtered subset available in workloadmeta)
	// Access UST env vars via: container.envs["DD_SERVICE"]
	Envs map[string]string `cel:"envs"`

	// Ports are the exposed container ports
	Ports []int `cel:"ports"`

	// Pod is the Kubernetes pod this container belongs to (nil if not in k8s)
	Pod *PodCEL `cel:"pod"`
}

// ImageCEL represents container image information in the CEL environment.
// Maps from workloadmeta.ContainerImage.
type ImageCEL struct {
	// Name is the full image name (e.g., "docker.io/library/redis:latest")
	Name string `cel:"name"`

	// ShortName is the image short name without registry (e.g., "redis")
	ShortName string `cel:"shortname"`

	// Tag is the image tag (e.g., "latest", "v1.2.3")
	Tag string `cel:"tag"`

	// Registry is the image registry (e.g., "docker.io")
	Registry string `cel:"registry"`
}

// PodCEL represents a Kubernetes pod in the CEL environment.
// Maps from workloadmeta.KubernetesPod.
type PodCEL struct {
	// Name is the pod name
	Name string `cel:"name"`

	// Namespace is the pod namespace
	Namespace string `cel:"namespace"`

	// OwnerRef contains the primary owner reference (first controller owner)
	// Access via: pod.ownerref.name, pod.ownerref.kind
	OwnerRef OwnerRefCEL `cel:"ownerref"`

	// Metadata contains pod labels and annotations
	Metadata MetadataCEL `cel:"metadata"`
}

// OwnerRefCEL represents the owner reference of a Kubernetes resource.
// Maps from workloadmeta.KubernetesPodOwner.
type OwnerRefCEL struct {
	// Name is the owner name (e.g., deployment name, replicaset name)
	Name string `cel:"name"`

	// Kind is the owner kind (e.g., "Deployment", "ReplicaSet", "DaemonSet")
	Kind string `cel:"kind"`
}

// MetadataCEL represents Kubernetes metadata in the CEL environment.
type MetadataCEL struct {
	// Labels are the pod labels
	// Access UST labels via: pod.metadata.labels["tags.datadoghq.com/my-app.service"]
	Labels map[string]string `cel:"labels"`

	// Annotations are the pod annotations
	Annotations map[string]string `cel:"annotations"`
}

// ServiceDiscoveryResult contains the evaluated service discovery values.
type ServiceDiscoveryResult struct {
	// ServiceName is the computed service name
	ServiceName string

	// SourceName indicates where the service name came from (e.g., "cel", "java", "container")
	SourceName string

	// Version is the computed service version
	Version string

	// MatchedRule is the index or description of the rule that matched (for debugging)
	MatchedRule string
}
