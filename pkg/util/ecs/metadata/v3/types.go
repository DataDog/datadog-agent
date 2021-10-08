// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package v3

// Task represents a task as returned by the ECS metadata API v3.
type Task struct {
	ClusterName           string             `json:"Cluster"`
	Containers            []Container        `json:"Containers"`
	KnownStatus           string             `json:"KnownStatus"`
	TaskARN               string             `json:"TaskARN"`
	Family                string             `json:"Family"`
	Version               string             `json:"Revision"`
	Limits                map[string]float64 `json:"Limits"`
	DesiredStatus         string             `json:"DesiredStatus"`
	ContainerInstanceTags map[string]string  `json:"ContainerInstanceTags,omitempty"` // undocumented
	TaskTags              map[string]string  `json:"TaskTags,omitempty"`              // undocumented
}

// Container represents a container within a task.
type Container struct {
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
