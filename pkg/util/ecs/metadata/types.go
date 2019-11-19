// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package metadata

// TaskMetadata is the info returned by the ECS task metadata API
type TaskMetadata struct {
	ClusterName   string              `json:"Cluster"`
	Containers    []ContainerMetadata `json:"Containers"`
	KnownStatus   string              `json:"KnownStatus"`
	TaskARN       string              `json:"TaskARN"`
	Family        string              `json:"Family"`
	Version       string              `json:"Revision"`
	Limits        map[string]float64  `json:"Limits"`
	DesiredStatus string              `json:"DesiredStatus"`
}

// ContainerMetadata is the representation of a container as exposed by the ECS metadata API
type ContainerMetadata struct {
	Name          string            `json:"Name"`
	Limits        map[string]uint64 `json:"Limits"`
	ImageID       string            `json:"ImageID,omitempty"`
	StartedAt     string            `json:"StartedAt"` // 2017-11-17T17:14:07.781711848Z
	DockerName    string            `json:"DockerName"`
	Type          string            `json:"Type"`
	Image         string            `json:"Image"`
	Labels        map[string]string `json:"Labels"`
	KnownStatus   string            `json:"KnownStatus"`
	DesiredStatus string            `json:"DesiredStatus"`
	DockerID      string            `json:"DockerID"`
	CreatedAt     string            `json:"CreatedAt"`
	Networks      []Network         `json:"Networks"`
	Ports         []Port            `json:"Ports"`
}

// Network represents the network of a container
type Network struct {
	NetworkMode   string   `json:"NetworkMode"`   // as of today the only supported mode is awsvpc
	IPv4Addresses []string `json:"IPv4Addresses"` // one-element list
}

// Port represents the ports of a container
type Port struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
}
