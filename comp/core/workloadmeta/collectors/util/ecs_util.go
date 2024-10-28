// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package util

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TaskParser is a function that parses ECS tasks
type TaskParser func(ctx context.Context) ([]workloadmeta.CollectorEvent, error)

// IsTaskCollectionEnabled returns true if the task metadata collection is enabled for core agent
// If agent launch type is EC2, collector will query the latest ECS metadata endpoint for each task returned by v1/tasks
// If agent launch type is Fargate, collector will query the latest ECS metadata endpoint
func IsTaskCollectionEnabled(cfg config.Component) bool {
	return cfg.GetBool("ecs_task_collection_enabled") && (flavor.GetFlavor() == flavor.DefaultAgent)
}

// ParseV4Task parses a metadata v4 task into a workloadmeta.ECSTask
func ParseV4Task(task v3or4.Task, seen map[workloadmeta.EntityID]struct{}) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindECSTask,
		ID:   task.TaskARN,
	}

	seen[entityID] = struct{}{}

	arnParts := strings.Split(task.TaskARN, "/")
	taskID := arnParts[len(arnParts)-1]

	taskContainers, containerEvents := ParseV4TaskContainers(task, seen)
	region, awsAccountID := ParseRegionAndAWSAccountID(task.TaskARN)

	entity := &workloadmeta.ECSTask{
		EntityID: entityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskID,
		},
		ClusterName:             parseClusterName(task.ClusterName),
		AWSAccountID:            awsAccountID,
		Region:                  region,
		Family:                  task.Family,
		Version:                 task.Version,
		DesiredStatus:           task.DesiredStatus,
		KnownStatus:             task.KnownStatus,
		VPCID:                   task.VPCID,
		ServiceName:             task.ServiceName,
		EphemeralStorageMetrics: task.EphemeralStorageMetrics,
		Limits:                  task.Limits,
		AvailabilityZone:        task.AvailabilityZone,
		Containers:              taskContainers,
		Tags:                    task.TaskTags,
		ContainerInstanceTags:   task.ContainerInstanceTags,
		PullStartedAt:           parseTime(taskID, "PullStartedAt", task.PullStartedAt),
		PullStoppedAt:           parseTime(taskID, "PullStoppedAt", task.PullStoppedAt),
		ExecutionStoppedAt:      parseTime(taskID, "ExecutionStoppedAt", task.ExecutionStoppedAt),
	}

	source := workloadmeta.SourceNodeOrchestrator
	entity.LaunchType = workloadmeta.ECSLaunchTypeEC2
	if strings.ToUpper(task.LaunchType) == "FARGATE" {
		entity.LaunchType = workloadmeta.ECSLaunchTypeFargate
		source = workloadmeta.SourceRuntime
	}

	events = append(events, containerEvents...)
	events = append(events, workloadmeta.CollectorEvent{
		Source: source,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	return events
}

// ParseV4TaskContainers extracts containers from a metadata v4 task and parse them
func ParseV4TaskContainers(
	task v3or4.Task,
	seen map[workloadmeta.EntityID]struct{},
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	taskContainers := make([]workloadmeta.OrchestratorContainer, 0, len(task.Containers))
	events := make([]workloadmeta.CollectorEvent, 0, len(task.Containers))

	for _, container := range task.Containers {
		containerID := container.DockerID
		taskContainers = append(taskContainers, workloadmeta.OrchestratorContainer{
			ID:   containerID,
			Name: container.Name,
		})
		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		}

		seen[entityID] = struct{}{}

		image, err := workloadmeta.NewContainerImage(container.ImageID, container.Image)
		if err != nil {
			log.Debugf("cannot split image name %q: %s", container.Image, err)
		}

		ips := make(map[string]string)
		for _, net := range container.Networks {
			if net.NetworkMode == "awsvpc" && len(net.IPv4Addresses) > 0 {
				ips["awsvpc"] = net.IPv4Addresses[0]
			}
		}

		containerEvent := &workloadmeta.Container{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:   container.DockerName,
				Labels: container.Labels,
			},
			State: workloadmeta.ContainerState{
				Running:  container.KnownStatus == "RUNNING",
				ExitCode: container.ExitCode,
			},
			Owner: &workloadmeta.EntityID{
				Kind: workloadmeta.KindECSTask,
				ID:   task.TaskARN,
			},
			ECSContainer: &workloadmeta.ECSContainer{
				DisplayName:   container.Name,
				Type:          container.Type,
				KnownStatus:   container.KnownStatus,
				DesiredStatus: container.DesiredStatus,
				LogOptions:    container.LogOptions,
				LogDriver:     container.LogDriver,
				ContainerARN:  container.ContainerARN,
				Snapshotter:   container.Snapshotter,
				Networks:      make([]workloadmeta.ContainerNetwork, 0, len(container.Networks)),
				Volumes:       make([]workloadmeta.ContainerVolume, 0, len(container.Volumes)),
			},
			Image:      image,
			NetworkIPs: ips,
			Ports:      make([]workloadmeta.ContainerPort, 0, len(container.Ports)),
		}

		containerEvent.Resources = workloadmeta.ContainerResources{}
		if _, ok := container.Limits["CPU"]; ok {
			cpuLimit := float64(container.Limits["CPU"])
			containerEvent.Resources.CPULimit = &cpuLimit
		}
		if _, ok := container.Limits["Memory"]; ok {
			memoryLimit := container.Limits["Memory"]
			containerEvent.Resources.MemoryLimit = &memoryLimit
		}

		if container.StartedAt != "" {
			containerEvent.State.StartedAt = *parseTime(containerID, "StartedAt", container.StartedAt)
		}
		if container.CreatedAt != "" {
			containerEvent.State.CreatedAt = *parseTime(containerID, "CreatedAt", container.CreatedAt)
		}

		for _, network := range container.Networks {
			containerEvent.Networks = append(containerEvent.Networks, workloadmeta.ContainerNetwork{
				NetworkMode:   network.NetworkMode,
				IPv4Addresses: network.IPv4Addresses,
				IPv6Addresses: network.IPv6Addresses,
			})
		}

		for _, port := range container.Ports {
			containerEvent.Ports = append(containerEvent.Ports, workloadmeta.ContainerPort{
				Port:     int(port.ContainerPort),
				Protocol: port.Protocol,
				HostPort: port.HostPort,
			})
		}

		for _, volume := range container.Volumes {
			containerEvent.Volumes = append(containerEvent.Volumes, workloadmeta.ContainerVolume{
				Name:        volume.DockerName,
				Source:      volume.Source,
				Destination: volume.Destination,
			})
		}

		if container.Health != nil {
			containerEvent.Health = &workloadmeta.ContainerHealthStatus{
				Status:   container.Health.Status,
				Since:    parseTime(containerID, "Health.Since", container.Health.Since),
				ExitCode: container.Health.ExitCode,
				Output:   container.Health.Output,
			}
		}

		source := workloadmeta.SourceNodeOrchestrator
		containerEvent.Runtime = workloadmeta.ContainerRuntimeDocker
		if task.LaunchType == "FARGATE" {
			source = workloadmeta.SourceRuntime
			containerEvent.Runtime = workloadmeta.ContainerRuntimeECSFargate
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: source,
			Type:   workloadmeta.EventTypeSet,
			Entity: containerEvent,
		})
	}

	return taskContainers, events
}

func parseTime(fieldOwner, fieldName, fieldValue string) *time.Time {
	if fieldValue == "" {
		return nil
	}
	result, err := time.Parse(time.RFC3339, fieldValue)
	if err != nil {
		log.Debugf("cannot parse %s %s for %s: %s", fieldName, fieldValue, fieldOwner, err)
	}
	return &result
}

// ParseRegionAndAWSAccountID parses the region and AWS account ID from a task ARN.
func ParseRegionAndAWSAccountID(taskARN string) (string, int) {
	arnParts := strings.Split(taskARN, ":")
	if len(arnParts) < 5 {
		return "", 0
	}
	if arnParts[0] != "arn" || arnParts[1] != "aws" {
		return "", 0
	}
	region := arnParts[3]
	if strings.Count(region, "-") < 2 {
		region = ""
	}

	id := arnParts[4]
	// aws account id is 12 digits
	// https://docs.aws.amazon.com/accounts/latest/reference/manage-acct-identifiers.html
	if len(id) != 12 {
		return region, 0
	}
	awsAccountID, err := strconv.Atoi(id)
	if err != nil {
		return region, 0
	}

	return region, awsAccountID
}

func parseClusterName(cluster string) string {
	parts := strings.Split(cluster, "/")
	if len(parts) != 2 {
		return cluster
	}
	return parts[1]
}

// ecsAgentRegexp is a regular expression to match ECS agent versions
// \d+(?:\.\d+){0,2} for versions like 1.32.0, 1.3 and 1
// (-\w+)? for optional pre-release tags like -beta
var ecsAgentVersionRegexp = regexp.MustCompile(`\bv(\d+(?:\.\d+){0,2}(?:-\w+)?)\b`)

// ParseECSAgentVersion parses the ECS agent version from the version string
// Instance metadata returns the version in the format `Amazon ECS Agent - v1.30.0 (02ff320c)`
func ParseECSAgentVersion(s string) string {
	match := ecsAgentVersionRegexp.FindStringSubmatch(s)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
