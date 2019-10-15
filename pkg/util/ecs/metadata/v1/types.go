// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2019 Datadog, Inc.

package v1

// Commands represents the list of available commands exposed by the ECS introspection endpoint.
type Commands struct {
	AvailableCommands []string `json:"AvailableCommands"`
}

// Instance represents the instance metadata exposed by the ECS introspection endpoint.
// See http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-introspection.html
type Instance struct {
	Cluster string `json:"Cluster"`
}

// Tasks represents the list of task exposed by the ECS introspection endpoint.
// See http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-introspection.html
type Tasks struct {
	Tasks []Task `json:"Tasks"`
}

// Task represents a task in the ECS introspection endpoint response.
type Task struct {
	Arn           string      `json:"Arn"`
	DesiredStatus string      `json:"DesiredStatus"`
	KnownStatus   string      `json:"KnownStatus"`
	Family        string      `json:"Family"`
	Version       string      `json:"Version"`
	Containers    []Container `json:"containers"`
}

// Container represents a container in a task.
type Container struct {
	DockerID   string `json:"DockerId"`
	DockerName string `json:"DockerName"`
	Name       string `json:"Name"`
}
