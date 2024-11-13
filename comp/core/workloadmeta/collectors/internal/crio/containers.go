// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type containerPort struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostPort      uint16 `json:"hostPort"`
}

// convertToEvent converts a CRI-O container to a workloadmeta event.
func (c *collector) convertContainerToEvent(ctx context.Context, ctr *v1.Container) workloadmeta.CollectorEvent {
	name := getContainerName(ctr.GetMetadata())
	namespace := getPodNamespace(ctx, c.client, ctr.GetPodSandboxId())
	containerStatus, info := getContainerStatus(ctx, c.client, ctr.GetId())
	pid, hostname, cgroupsPath := parseContainerInfo(info)
	cpuLimit, memLimit := getResourceLimits(containerStatus, info)
	image := getContainerImage(ctx, c.client, ctr.GetImage())
	ports := extractPortsFromAnnotations(ctr.GetAnnotations())

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   ctr.GetId(),
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        name,
				Namespace:   namespace,
				Labels:      ctr.GetLabels(),
				Annotations: ctr.GetAnnotations(),
			},
			Hostname: hostname,
			Image:    image,
			PID:      pid,
			Ports:    ports,
			Runtime:  workloadmeta.ContainerRuntimeCRIO,
			State:    getContainerState(containerStatus),
			Resources: workloadmeta.ContainerResources{
				CPULimit:    cpuLimit,
				MemoryLimit: memLimit,
			},
			CgroupPath: cgroupsPath,
		},
	}
}

// getContainerName retrieves the container name.
func getContainerName(containerMetadata *v1.ContainerMetadata) string {
	if containerMetadata == nil {
		return ""
	}
	return containerMetadata.Name
}

// getPodNamespace retrieves the namespace for a given pod ID.
func getPodNamespace(ctx context.Context, client crio.Client, podID string) string {
	pod, err := client.GetPodStatus(ctx, podID)
	if err != nil || pod == nil || pod.Metadata == nil {
		log.Errorf("Failed to get pod namespace for pod ID %s: %v", podID, err)
		return ""
	}
	return pod.Metadata.Namespace
}

// getContainerStatus retrieves the status of a container.
func getContainerStatus(ctx context.Context, client crio.Client, containerID string) (*v1.ContainerStatus, map[string]string) {
	statusResponse, err := client.GetContainerStatus(ctx, containerID)
	status := statusResponse.GetStatus()
	info := statusResponse.GetInfo()
	if err != nil || status == nil {
		log.Errorf("Failed to get container status for container %s: %v", containerID, err)
		return &v1.ContainerStatus{State: v1.ContainerState_CONTAINER_UNKNOWN}, make(map[string]string)
	}
	return status, info
}

// getResourceLimits extracts CPU and memory limits from container status or info as a fallback.
func getResourceLimits(containerStatus *v1.ContainerStatus, info map[string]string) (*float64, *uint64) {
	// First, try to get resources from containerStatus
	if containerStatus != nil && containerStatus.GetResources() != nil && containerStatus.GetResources().GetLinux() != nil {
		var cpuLimit *float64
		var memLimit *uint64
		cpuPeriod := float64(containerStatus.GetResources().GetLinux().GetCpuPeriod())
		cpuQuota := float64(containerStatus.GetResources().GetLinux().GetCpuQuota())
		memLimitInBytes := uint64(containerStatus.GetResources().GetLinux().GetMemoryLimitInBytes())

		if cpuPeriod != 0 && cpuQuota != 0 {
			limit := cpuQuota / cpuPeriod
			cpuLimit = &limit
		}
		if memLimitInBytes != 0 {
			memLimit = &memLimitInBytes
		}
		return cpuLimit, memLimit
	}

	if info == nil || info["info"] == "" {
		log.Warn("Info map is nil or does not contain resource information")
		return nil, nil
	}

	// Fallback to parsing resources from info if status resources are nil
	var parsedInfo struct {
		RuntimeSpec struct {
			Linux struct {
				Resources struct {
					CPU struct {
						Quota  int64 `json:"quota"`
						Period int64 `json:"period"`
					} `json:"cpu"`
					Memory struct {
						LimitInBytes int64 `json:"memoryLimitInBytes"`
					} `json:"memory"`
				} `json:"resources"`
			} `json:"linux"`
		} `json:"runtimeSpec"`
	}

	if err := json.Unmarshal([]byte(info["info"]), &parsedInfo); err != nil {
		log.Warnf("Failed to parse resources from container info: %v", err)
		return nil, nil
	}

	cpuPeriod := float64(parsedInfo.RuntimeSpec.Linux.Resources.CPU.Period)
	cpuQuota := float64(parsedInfo.RuntimeSpec.Linux.Resources.CPU.Quota)
	memLimitInBytes := uint64(parsedInfo.RuntimeSpec.Linux.Resources.Memory.LimitInBytes)

	var cpuLimit *float64
	var memLimit *uint64
	if cpuPeriod != 0 && cpuQuota != 0 {
		limit := cpuQuota / cpuPeriod
		cpuLimit = &limit
	}
	if memLimitInBytes != 0 {
		memLimit = &memLimitInBytes
	}
	return cpuLimit, memLimit
}

// getContainerImage retrieves and converts a container image to workloadmeta format.
func getContainerImage(ctx context.Context, client crio.Client, imageSpec *v1.ImageSpec) workloadmeta.ContainerImage {
	imageResp, err := client.GetContainerImage(ctx, imageSpec, false)
	if err != nil || imageResp == nil || imageResp.Image == nil {
		log.Warnf("Failed to fetch image: %v", err)
		return workloadmeta.ContainerImage{}
	}
	image := imageResp.Image
	imgID := image.Id
	imgName := ""
	if len(image.RepoTags) > 0 {
		imgName = image.RepoTags[0]
	}
	wmImg, err := workloadmeta.NewContainerImage(imgID, imgName)
	if err != nil {
		log.Warnf("Failed to create image: %v", err)
		return workloadmeta.ContainerImage{}
	}
	if len(image.RepoDigests) > 0 {
		wmImg.RepoDigest = image.RepoDigests[0]
	}
	return wmImg
}

// getContainerState returns the workloadmeta.ContainerState based on container status.
func getContainerState(containerStatus *v1.ContainerStatus) workloadmeta.ContainerState {
	if containerStatus == nil {
		return workloadmeta.ContainerState{Status: workloadmeta.ContainerStatusUnknown}
	}
	exitCode := int64(containerStatus.ExitCode)
	return workloadmeta.ContainerState{
		Running:    containerStatus.State == v1.ContainerState_CONTAINER_RUNNING,
		Status:     mapContainerStatus(containerStatus.State),
		CreatedAt:  time.Unix(0, containerStatus.CreatedAt),
		StartedAt:  time.Unix(0, containerStatus.StartedAt),
		FinishedAt: time.Unix(0, containerStatus.FinishedAt),
		ExitCode:   &exitCode,
	}
}

// mapContainerStatus maps CRI-O container state to workloadmeta.ContainerStatus.
func mapContainerStatus(state v1.ContainerState) workloadmeta.ContainerStatus {
	switch state {
	case v1.ContainerState_CONTAINER_CREATED:
		return workloadmeta.ContainerStatusCreated
	case v1.ContainerState_CONTAINER_RUNNING:
		return workloadmeta.ContainerStatusRunning
	case v1.ContainerState_CONTAINER_EXITED:
		return workloadmeta.ContainerStatusStopped
	case v1.ContainerState_CONTAINER_UNKNOWN:
		return workloadmeta.ContainerStatusUnknown
	}
	return workloadmeta.ContainerStatusUnknown
}

// generateUnsetEvent creates an unset event for a given container ID.
func generateUnsetContainerEvent(seenID workloadmeta.EntityID) workloadmeta.CollectorEvent {
	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.Container{
			EntityID: seenID,
		},
	}
}

// extractPortsFromAnnotations parses container ports from annotations.
func extractPortsFromAnnotations(annotations map[string]string) []workloadmeta.ContainerPort {
	var wmContainerPorts []workloadmeta.ContainerPort

	if len(annotations) == 0 {
		log.Warn("Annotations are nil or empty")
		return wmContainerPorts
	}

	for key, value := range annotations {
		if strings.Contains(key, "ports") {
			var ports []struct {
				Name          string `json:"name"`
				ContainerPort int    `json:"containerPort"`
				Protocol      string `json:"protocol"`
				HostPort      uint16 `json:"hostPort"`
			}

			if err := json.Unmarshal([]byte(value), &ports); err != nil {
				log.Warnf("Failed to parse ports from annotation %s: %v", key, err)
				continue //skip to next annotation
			}

			for _, port := range ports {
				wmContainerPorts = append(wmContainerPorts, workloadmeta.ContainerPort{
					Name:     port.Name,
					Port:     port.ContainerPort,
					Protocol: port.Protocol,
					HostPort: port.HostPort,
				})
			}
		}
	}
	return wmContainerPorts
}

// parseContainerInfo takes a map[string]string with JSON-encoded data and extracts PID, Hostname, and CgroupsPath.
func parseContainerInfo(info map[string]string) (int, string, string) {
	var pid int
	var hostname, cgroupsPath string

	if info == nil || info["info"] == "" {
		log.Warn("Container info is nil or empty")
		return pid, hostname, cgroupsPath
	}

	var parsedInfo struct {
		PID         int `json:"pid"`
		RuntimeSpec struct {
			Hostname string `json:"hostname"`
			Linux    struct {
				CgroupsPath string `json:"cgroupsPath"`
			} `json:"linux"`
		} `json:"runtimeSpec"`
	}

	// Unmarshal the JSON string into the struct
	if err := json.Unmarshal([]byte(info["info"]), &parsedInfo); err == nil {
		pid = parsedInfo.PID
		hostname = parsedInfo.RuntimeSpec.Hostname
		cgroupsPath = parsedInfo.RuntimeSpec.Linux.CgroupsPath
	} else {
		log.Warnf("Failed to parse container info: %v", err)
	}

	return pid, hostname, cgroupsPath
}
