// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package v3or4

// Task represents a task as returned by the ECS metadata API v3 or v4.
type Task struct {
	ClusterName             string             `json:"Cluster"`
	Containers              []Container        `json:"Containers"`
	KnownStatus             string             `json:"KnownStatus"`
	TaskARN                 string             `json:"TaskARN"`
	Family                  string             `json:"Family"`
	Version                 string             `json:"Revision"`
	Limits                  map[string]float64 `json:"Limits,omitempty"`
	DesiredStatus           string             `json:"DesiredStatus"`
	LaunchType              string             `json:"LaunchType,omitempty"` // present only in v4
	ContainerInstanceTags   map[string]string  `json:"ContainerInstanceTags,omitempty"`
	TaskTags                map[string]string  `json:"TaskTags,omitempty"`
	EphemeralStorageMetrics map[string]int64   `json:"EphemeralStorageMetrics,omitempty"`
	ServiceName             string             `json:"ServiceName,omitempty"`
	VPCID                   string             `json:"VPCID,omitempty"`
	PullStartedAt           string             `json:"PullStartedAt,omitempty"`
	PullStoppedAt           string             `json:"PullStoppedAt,omitempty"`
	ExecutionStoppedAt      string             `json:"ExecutionStoppedAt,omitempty"`
	AvailabilityZone        string             `json:"AvailabilityZone,omitempty"`
	Errors                  []AwsError         `Json:"Errors,Omitempty"`
}

// AwsError represents errors returned in the payload
type AwsError struct {
	ErrorField   string `json:"ErrorField,omitempty"`
	ErrorCode    string `json:"ErrorCode,omitempty"`
	ErrorMessage string `json:"ErrorMessage,omitempty"`
	StatusCode   int    `json:"StatusCode,omitempty"`
	RequestID    string `json:"RequestId,omitempty"`
	RequestARN   string `json:"RequestARN,omitempty"`
}

// Container represents a container within a task.
type Container struct {
	Name          string            `json:"Name"`
	Limits        map[string]uint64 `json:"Limits,omitempty"`
	ImageID       string            `json:"ImageID,omitempty"`
	StartedAt     string            `json:"StartedAt,omitempty"` // 2017-11-17T17:14:07.781711848Z
	DockerName    string            `json:"DockerName"`
	Type          string            `json:"Type"`
	Image         string            `json:"Image"`
	Labels        map[string]string `json:"Labels,omitempty"`
	KnownStatus   string            `json:"KnownStatus"` // See https://github.com/aws/amazon-ecs-agent/blob/master/agent/api/container/status/containerstatus.go
	DesiredStatus string            `json:"DesiredStatus"`
	DockerID      string            `json:"DockerID"`
	CreatedAt     string            `json:"CreatedAt,omitempty"`
	Networks      []Network         `json:"Networks,omitempty"`
	Ports         []Port            `json:"Ports,omitempty"`
	LogDriver     string            `json:"LogDriver,omitempty"`    // present only in v4
	LogOptions    map[string]string `json:"LogOptions,omitempty"`   // present only in v4
	ContainerARN  string            `json:"ContainerARN,omitempty"` // present only in v4
	Health        *HealthStatus     `json:"Health,omitempty"`
	Volumes       []Volume          `json:"Volumes,omitempty"`
	ExitCode      *int64            `json:"ExitCode,omitempty"`
	Snapshotter   string            `json:"Snapshotter,omitempty"`
	RestartCount  *int              `json:"RestartCount,omitempty"` // present only in v4
}

// HealthStatus represents the health status of a container
type HealthStatus struct {
	Status   string `json:"status,omitempty"`
	Since    string `json:"statusSince,omitempty"`
	ExitCode *int64 `json:"exitCode,omitempty"`
	Output   string `json:"output,omitempty"`
}

// Network represents the network of a container
type Network struct {
	NetworkMode   string   `json:"NetworkMode"`   // supports awsvpc and bridge
	IPv4Addresses []string `json:"IPv4Addresses"` // one-element list
	IPv6Addresses []string `json:"IPv6Addresses,omitempty"`
}

// Port represents the ports of a container
type Port struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
	HostIP        string `json:"HostIP,omitempty"`
}

// Volume represents the volumes of a container
type Volume struct {
	DockerName  string `json:"DockerName,omitempty"`
	Source      string `json:"Source,omitempty"`
	Destination string `json:"Destination,omitempty"`
}
