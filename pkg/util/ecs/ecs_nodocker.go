// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !docker

package ecs

import "github.com/DataDog/datadog-agent/pkg/util/docker"

const (
	// CloudProviderName contains the inventory name of for ECS
	CloudProviderName = "AWS"
)

// IsRunningOn returns true if the agent is running on ECS/Fargate
func IsRunningOn() bool {
	return false
}

// GetUtil returns an ECS util
func GetUtil() (*Util, error) {
	return nil, docker.ErrDockerNotCompiled
}

// IsECSInstance returns whether the agent is running in ECS.
func IsECSInstance() bool {
	return false
}

// IsFargateInstance returns whether the agent is in an ECS fargate task.
// It detects it by getting and unmarshalling the metadata API response.
func IsFargateInstance() bool {
	return false
}

// GetTaskMetadata extracts the metadata payload for the task the agent is in.
func GetTaskMetadata() (TaskMetadata, error) {
	var meta TaskMetadata
	return meta, nil
}
