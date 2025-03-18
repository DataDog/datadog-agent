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

// convertContainerToEvent converts a CRI-O container to a workloadmeta event.
func (c *collector) convertContainerToEvent(ctx context.Context, ctr *v1.Container) workloadmeta.CollectorEvent {
	name := getContainerName(ctr.GetMetadata())
	namespace := getPodNamespace(ctx, c.client, ctr.GetPodSandboxId())
	containerStatus, info := getContainerStatus(ctx, c.client, ctr.GetId())
	pid, hostname, cgroupsPath := parseContainerInfo(info)
	cpuLimit, memLimit := getResourceLimits(containerStatus, info)
	image := getContainerImage(containerStatus)
	ports := parsePortsFromAnnotations(ctr.GetAnnotations())

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
	return containerMetadata.GetName()
}

// getPodNamespace retrieves the namespace for a given pod ID.
func getPodNamespace(ctx context.Context, client crio.Client, podID string) string {
	pod, err := client.GetPodStatus(ctx, podID)
	if err != nil || pod == nil || pod.GetMetadata() == nil {
		log.Errorf("Failed to get pod namespace for pod ID %s: %v", podID, err)
		return ""
	}
	return pod.GetMetadata().GetNamespace()
}

// getContainerStatus retrieves the status of a container.
func getContainerStatus(ctx context.Context, client crio.Client, containerID string) (*v1.ContainerStatus, map[string]string) {
	statusResponse, err := client.GetContainerStatus(ctx, containerID)
	if err != nil || statusResponse.GetStatus() == nil {
		log.Errorf("Failed to get container status for container %s: %v", containerID, err)
		return nil, nil
	}
	status := statusResponse.GetStatus()
	info := statusResponse.GetInfo()
	return status, info
}

// getContainerImage retrieves and converts a container image to workloadmeta format.
func getContainerImage(ctrStatus *v1.ContainerStatus) workloadmeta.ContainerImage {
	if ctrStatus == nil {
		log.Warn("container status is nil, cannot fetch image")
		return workloadmeta.ContainerImage{}
	}

	imageSpec := ctrStatus.GetImage()
	if imageSpec == nil {
		log.Warn("Image spec is nil, cannot fetch image")
		return workloadmeta.ContainerImage{}
	}

	imgID, digestErr := parseDigests([]string{ctrStatus.ImageRef})
	if digestErr != nil {
		imgID = ctrStatus.ImageRef
	}

	wmImg, err := workloadmeta.NewContainerImage(imgID, imageSpec.Image)
	if err != nil {
		log.Debugf("Failed to create image: %v", err)
		return workloadmeta.ContainerImage{}
	}
	if digestErr != nil && sbomCollectionIsEnabled() {
		// Don't log if the container image could not be created
		log.Warnf("Failed to parse digest for image with ID %s: %v. As a result, SBOM vulnerabilities may not be properly linked to this image.", imgID, digestErr)
	}
	wmImg.RepoDigest = ctrStatus.ImageRef

	return wmImg
}

// getContainerState returns the workloadmeta.ContainerState based on container status.
func getContainerState(containerStatus *v1.ContainerStatus) workloadmeta.ContainerState {
	if containerStatus == nil {
		return workloadmeta.ContainerState{Status: workloadmeta.ContainerStatusUnknown}
	}
	exitCode := int64(containerStatus.GetExitCode())
	return workloadmeta.ContainerState{
		Running:    containerStatus.GetState() == v1.ContainerState_CONTAINER_RUNNING,
		Status:     mapContainerStatus(containerStatus.GetState()),
		CreatedAt:  time.Unix(0, containerStatus.GetCreatedAt()).UTC(),
		StartedAt:  time.Unix(0, containerStatus.GetStartedAt()).UTC(),
		FinishedAt: time.Unix(0, containerStatus.GetFinishedAt()).UTC(),
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

// generateUnsetContainerEvent creates an unset event for a given container ID.
func generateUnsetContainerEvent(seenID workloadmeta.EntityID) workloadmeta.CollectorEvent {
	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.Container{
			EntityID: seenID,
		},
	}
}

// getResourceLimits extracts CPU and memory limits from container status or info as a fallback.
func getResourceLimits(containerStatus *v1.ContainerStatus, info map[string]string) (*float64, *uint64) {
	// First, try to get resources from containerStatus
	if containerStatus != nil && containerStatus.GetResources() != nil && containerStatus.GetResources().GetLinux() != nil {
		cpuPeriod := float64(containerStatus.GetResources().GetLinux().GetCpuPeriod())
		cpuQuota := float64(containerStatus.GetResources().GetLinux().GetCpuQuota())
		memLimitInBytes := uint64(containerStatus.GetResources().GetLinux().GetMemoryLimitInBytes())

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

	// If containerStatus is nil or does not contain resource information, try to get resources from container info
	return parseResourceLimitsFromInfo(info)
}

// parseResourceLimitsFromInfo extracts CPU and memory limits from JSON-encoded container info.
func parseResourceLimitsFromInfo(info map[string]string) (*float64, *uint64) {
	if info == nil || info["info"] == "" {
		log.Debug("Info map is nil or does not contain resource information")
		return nil, nil
	}

	var parsed resourceInfo
	if err := json.Unmarshal([]byte(info["info"]), &parsed); err != nil {
		log.Debugf("Failed to parse resources from container info: %v", err)
		return nil, nil
	}

	cpuPeriod := float64(parsed.RuntimeSpec.Linux.Resources.CPU.Period)
	cpuQuota := float64(parsed.RuntimeSpec.Linux.Resources.CPU.Quota)
	memLimitInBytes := uint64(parsed.RuntimeSpec.Linux.Resources.Memory.LimitInBytes)

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

// parsePortsFromAnnotations parses container ports from annotations.
func parsePortsFromAnnotations(annotations map[string]string) []workloadmeta.ContainerPort {
	var wmContainerPorts []workloadmeta.ContainerPort

	if len(annotations) == 0 {
		log.Debug("Annotations are nil or empty")
		return wmContainerPorts
	}

	for key, value := range annotations {
		if strings.Contains(key, "ports") {
			var ports []portAnnotation

			if err := json.Unmarshal([]byte(value), &ports); err != nil {
				log.Debugf("Failed to parse ports from annotation %s: %v", key, err)
				continue // skip to next annotation
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
	if info == nil || info["info"] == "" {
		log.Debug("Container info is nil or empty")
		return 0, "", ""
	}

	var parsed containerInfo
	if err := json.Unmarshal([]byte(info["info"]), &parsed); err != nil {
		log.Debugf("Failed to parse container info: %v", err)
		return 0, "", ""
	}

	return parsed.PID, parsed.RuntimeSpec.Hostname, parsed.RuntimeSpec.Linux.CgroupsPath
}

// resourceInfo contains CPU and memory resource information.
type resourceInfo struct {
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

// portAnnotation contains container port information.
type portAnnotation struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostPort      uint16 `json:"hostPort"`
}

// containerInfo contains additional container information.
type containerInfo struct {
	PID         int `json:"pid"`
	RuntimeSpec struct {
		Hostname string `json:"hostname"`
		Linux    struct {
			CgroupsPath string `json:"cgroupsPath"`
		} `json:"linux"`
	} `json:"runtimeSpec"`
}
