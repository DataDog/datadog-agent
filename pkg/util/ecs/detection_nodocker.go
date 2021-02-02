// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// +build !docker

package ecs

// IsECSInstance returns whether the agent is running in ECS.
func IsECSInstance() bool {
	return false
}

// IsFargateInstance returns whether the agent is in an ECS fargate task.
// It detects it by getting and unmarshalling the metadata API response.
func IsFargateInstance() bool {
	return false
}

// IsRunningOn returns true if the agent is running on ECS/Fargate
func IsRunningOn() bool {
	return false
}

// GetNTPHosts returns the NTP hosts for ECS/Fargate if it is detected as the cloud provider, otherwise an empty array.
func GetNTPHosts() []string {
	return nil
}
