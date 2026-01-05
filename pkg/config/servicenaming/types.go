// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

package servicenaming

// ProcessCEL represents a process in the CEL environment
type ProcessCEL struct {
	Cmd    string    `cel:"cmd"`
	Binary BinaryCEL `cel:"binary"`
	Ports  []int     `cel:"ports"`
	User   string    `cel:"user"`

	// Spec examples reference process.container.pod...
	Container *ContainerCEL `cel:"container"`
}

// BinaryCEL represents binary information in the CEL environment
type BinaryCEL struct {
	Name  string `cel:"name"`
	User  string `cel:"user"`
	Group string `cel:"group"`
}

// ContainerCEL represents a container in the CEL environment
type ContainerCEL struct {
	Name  string   `cel:"name"`
	Image ImageCEL `cel:"image"`
	Pod   PodCEL   `cel:"pod"`
}

// ImageCEL represents container image information in the CEL environment
type ImageCEL struct {
	Name      string `cel:"name"`
	ShortName string `cel:"shortname"`
	Tag       string `cel:"tag"`
}

// PodCEL represents a Kubernetes pod in the CEL environment
type PodCEL struct {
	Name         string      `cel:"name"`
	Namespace    string      `cel:"namespace"`
	OwnerRefName string      `cel:"ownerrefname"`
	OwnerRefKind string      `cel:"ownerrefkind"`
	Metadata     MetadataCEL `cel:"metadata"`

	// Spec examples reference pod.ownerref.name / pod.ownerref.kind
	OwnerRef OwnerRefCEL `cel:"ownerref"`
}

// OwnerRefCEL represents the structured owner reference (name/kind)
type OwnerRefCEL struct {
	Name string `cel:"name"`
	Kind string `cel:"kind"`
}

// MetadataCEL represents Kubernetes metadata in the CEL environment
type MetadataCEL struct {
	Labels map[string]string `cel:"labels"`
}

// ServiceDiscoveryResult contains the evaluated service discovery values
type ServiceDiscoveryResult struct {
	ServiceName string
	SourceName  string
	Version     string
	MatchedRule string
}
