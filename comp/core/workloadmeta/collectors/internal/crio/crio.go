// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

// Package crio implements the crio Workloadmeta collector.
package crio

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/fx"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID           = "crio"
	componentName         = "workloadmeta-crio"
	defaultCrioSocketPath = "/var/run/crio/crio.sock"
)

type collector struct {
	id      string
	client  crio.ClientItf
	store   workloadmeta.Component
	catalog workloadmeta.AgentType
	seen    map[workloadmeta.EntityID]struct{}
}

type containerPort struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostPort      uint16 `json:"hostPort"`
}

// NewCollector initializes a new CRI-O collector.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			seen:    make(map[workloadmeta.EntityID]struct{}),
			catalog: workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start initializes the collector for workloadmeta.
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Crio) {
		return dderrors.NewDisabled(componentName, "Crio not detected")
	}
	c.store = store

	criSocket := getCRIOSocketPath()
	client, err := crio.NewCRIOClient(criSocket)
	if err != nil {
		log.Errorf("CRI-O client creation failed for socket %s: %v", criSocket, err)
		client.Close()
		return err
	}
	c.client = client
	return nil
}

// Pull gathers container data.
func (c *collector) Pull(ctx context.Context) error {
	containers, err := c.client.GetAllContainers(ctx)
	if err != nil {
		log.Errorf("Failed to pull container list: %v", err)
		return err
	}

	seen := make(map[workloadmeta.EntityID]struct{})
	events := make([]workloadmeta.CollectorEvent, 0, len(containers))
	for _, container := range containers {
		event := c.convertToEvent(ctx, container)
		seen[event.Entity.GetID()] = struct{}{}
		events = append(events, event)
	}
	for seenID := range c.seen {
		if _, ok := seen[seenID]; !ok {
			events = append(events, generateUnsetEvent(seenID))
		}
	}
	c.seen = seen
	c.store.Notify(events)
	return nil
}

// GetID returns the collector ID.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog returns the workloadmeta agent type.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

// convertToEvent converts a CRI-O container to a workloadmeta event.
func (c *collector) convertToEvent(ctx context.Context, ctr *v1.Container) workloadmeta.CollectorEvent {
	name := getContainerName(ctr.Metadata)
	namespace := getPodNamespace(ctx, c.client, ctr.PodSandboxId)
	containerStatus := getContainerStatus(ctx, c.client, ctr.Id)
	cpuLimit, memLimit := getResourceLimits(containerStatus)
	image := getContainerImage(ctx, c.client, ctr.Image)
	ports := extractPortsFromAnnotations(ctr.Annotations)

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   ctr.Id,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        name,
				Namespace:   namespace,
				Labels:      ctr.Labels,
				Annotations: ctr.Annotations,
			},
			Image:   image,
			Ports:   ports,
			Runtime: workloadmeta.ContainerRuntimeCRIO,
			State:   getContainerState(containerStatus),
			Resources: workloadmeta.ContainerResources{
				CPULimit:    cpuLimit,
				MemoryLimit: memLimit,
			},
		},
	}
}

// getCRIOSocketPath returns the configured CRI-O socket path or the default path.
func getCRIOSocketPath() string {
	criSocket := pkgconfigsetup.Datadog().GetString("cri_socket_path")
	if criSocket == "" {
		return defaultCrioSocketPath
	}
	return criSocket
}

// getContainerName retrieves the container name.
func getContainerName(containerMetadata *v1.ContainerMetadata) string {
	if containerMetadata == nil {
		return ""
	}
	return containerMetadata.Name
}

// getPodNamespace retrieves the namespace for a given pod ID.
func getPodNamespace(ctx context.Context, client crio.ClientItf, podID string) string {
	pod, err := client.GetPodStatus(ctx, podID)
	if err != nil || pod == nil || pod.Metadata == nil {
		log.Errorf("Failed to get pod namespace for pod ID %s: %v", podID, err)
		return ""
	}
	return pod.Metadata.Namespace
}

// getContainerStatus retrieves the status of a container.
func getContainerStatus(ctx context.Context, client crio.ClientItf, containerID string) *v1.ContainerStatus {
	status, err := client.GetContainerStatus(ctx, containerID)
	if err != nil || status == nil {
		log.Errorf("Failed to get container status for container %s: %v", containerID, err)
		return &v1.ContainerStatus{State: v1.ContainerState_CONTAINER_UNKNOWN}
	}
	return status
}

// getResourceLimits extracts CPU and memory limits from container status.
func getResourceLimits(containerStatus *v1.ContainerStatus) (*float64, *uint64) {
	if containerStatus == nil || containerStatus.Resources == nil || containerStatus.Resources.Linux == nil {
		return nil, nil
	}

	var cpuLimit *float64
	var memLimit *uint64
	cpuPeriod := float64(containerStatus.Resources.Linux.CpuPeriod)
	cpuQuota := float64(containerStatus.Resources.Linux.CpuQuota)
	memLimitInBytes := uint64(containerStatus.Resources.Linux.MemoryLimitInBytes)

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
func getContainerImage(ctx context.Context, client crio.ClientItf, imageSpec *v1.ImageSpec) workloadmeta.ContainerImage {
	image, err := client.GetContainerImage(ctx, imageSpec)
	if err != nil || image == nil {
		log.Warnf("Failed to fetch image: %v", err)
		return workloadmeta.ContainerImage{}
	}

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
func generateUnsetEvent(seenID workloadmeta.EntityID) workloadmeta.CollectorEvent {
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
	for key, value := range annotations {
		if strings.Contains(key, "ports") {
			var ports []containerPort
			if err := json.Unmarshal([]byte(value), &ports); err != nil {
				log.Warnf("Failed to parse ports from annotation %s: %v", key, err)
				return nil
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
