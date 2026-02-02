// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmetaimpl

import (
	"time"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// FilterEntitiesForVerbose filters entities to only include fields shown in non-verbose text output
func FilterEntitiesForVerbose(entities []wmdef.Entity, verbose bool) []wmdef.Entity {
	if verbose {
		// Verbose mode: return all entities as-is
		return entities
	}

	// Non-verbose mode: filter out verbose-only fields
	filtered := make([]wmdef.Entity, len(entities))
	for i, entity := range entities {
		filtered[i] = filterEntityForNonVerbose(entity)
	}
	return filtered
}

// filterEntityForNonVerbose returns a copy of the entity with verbose-only fields zeroed out
func filterEntityForNonVerbose(entity wmdef.Entity) wmdef.Entity {
	switch e := entity.(type) {
	case *wmdef.Container:
		return filterContainer(e)
	case *wmdef.KubernetesPod:
		return filterKubernetesPod(e)
	case *wmdef.ECSTask:
		return filterECSTask(e)
	case *wmdef.Process:
		return filterProcess(e)
	case *wmdef.ContainerImageMetadata:
		return filterContainerImageMetadata(e)
	case *wmdef.GPU:
		return filterGPU(e)
	case *wmdef.KubeCapabilities:
		return filterKubeCapabilities(e)
	case *wmdef.Kubelet:
		return filterKubelet(e)
	case *wmdef.KubernetesDeployment:
		return filterKubernetesDeployment(e)
	case *wmdef.KubernetesMetadata:
		return filterKubernetesMetadata(e)
	case *wmdef.CRD:
		return filterCRD(e)
	default:
		// For other types, return as-is
		return entity
	}
}

// filterContainer removes verbose-only fields from Container
func filterContainer(c *wmdef.Container) *wmdef.Container {
	filtered := *c // Copy the struct

	// Clear verbose-only fields
	// Based on Container.String(verbose=false) behavior:
	// Verbose-only: Hostname, NetworkIPs, PID, CgroupPath, Ports, ResizePolicy
	filtered.Hostname = ""
	filtered.NetworkIPs = nil
	filtered.PID = 0
	filtered.CgroupPath = ""
	filtered.Ports = nil
	filtered.ResizePolicy = wmdef.ContainerResizePolicy{}
	filtered.EnvVars = nil // Not shown in non-verbose

	// Filter nested State to only show Running
	filtered.State = filterContainerState(c.State)

	// Filter nested Image to only show Name and Tag
	filtered.Image = filterContainerImage(c.Image)

	// Filter nested EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(c.EntityMeta)

	return &filtered
}

// filterContainerState keeps only non-verbose fields
func filterContainerState(s wmdef.ContainerState) wmdef.ContainerState {
	// Non-verbose only shows Running field
	return wmdef.ContainerState{
		Running: s.Running,
		// Zero out verbose-only fields:
		// Status, Health, CreatedAt, StartedAt, FinishedAt, ExitCode
	}
}

// filterContainerImage keeps only non-verbose fields
func filterContainerImage(img wmdef.ContainerImage) wmdef.ContainerImage {
	// Non-verbose shows Name and Tag
	return wmdef.ContainerImage{
		Name: img.Name,
		Tag:  img.Tag,
		// Zero out verbose-only fields:
		// ID, RawName, ShortName, Registry, RepoDigest
	}
}

// filterEntityMeta keeps only non-verbose fields
func filterEntityMeta(meta wmdef.EntityMeta) wmdef.EntityMeta {
	// Non-verbose shows Name and Namespace
	return wmdef.EntityMeta{
		Name:      meta.Name,
		Namespace: meta.Namespace,
		// Zero out verbose-only fields:
		// Annotations, Labels, UID
	}
}

// filterKubernetesPod removes verbose-only fields from KubernetesPod
func filterKubernetesPod(p *wmdef.KubernetesPod) *wmdef.KubernetesPod {
	filtered := *p // Copy the struct

	// Filter EntityMeta
	filtered.EntityMeta = filterEntityMeta(p.EntityMeta)

	// Based on KubernetesPod.String() - keep Phase, Ready, IP, Containers
	// Clear verbose-only fields
	filtered.CreationTimestamp = time.Time{}
	filtered.DeletionTimestamp = nil
	filtered.StartTime = nil
	filtered.HostIP = ""
	filtered.HostNetwork = false
	filtered.InitContainerStatuses = nil
	filtered.ContainerStatuses = nil
	filtered.EphemeralContainerStatuses = nil
	filtered.Conditions = nil
	filtered.Volumes = nil
	filtered.Tolerations = nil
	filtered.PersistentVolumeClaimNames = nil
	filtered.NamespaceLabels = nil
	filtered.NamespaceAnnotations = nil

	// Filter containers to show only basic info
	filtered.InitContainers = filterOrchestratorContainers(p.InitContainers)
	filtered.Containers = filterOrchestratorContainers(p.Containers)
	filtered.EphemeralContainers = filterOrchestratorContainers(p.EphemeralContainers)

	return &filtered
}

// filterOrchestratorContainers filters container list
func filterOrchestratorContainers(containers []wmdef.OrchestratorContainer) []wmdef.OrchestratorContainer {
	if len(containers) == 0 {
		return nil
	}

	filtered := make([]wmdef.OrchestratorContainer, len(containers))
	for i, c := range containers {
		filtered[i] = wmdef.OrchestratorContainer{
			ID:    c.ID,
			Name:  c.Name,
			Image: filterContainerImage(c.Image),
			// Resources omitted in non-verbose
		}
	}
	return filtered
}

// filterECSTask removes verbose-only fields from ECSTask
func filterECSTask(t *wmdef.ECSTask) *wmdef.ECSTask {
	filtered := *t // Copy the struct

	// Filter EntityMeta
	filtered.EntityMeta = filterEntityMeta(t.EntityMeta)

	// Keep basic task info, clear verbose details
	filtered.Tags = nil
	filtered.ContainerInstanceTags = nil
	filtered.EphemeralStorageMetrics = nil

	return &filtered
}

// filterProcess removes verbose-only fields from Process
func filterProcess(p *wmdef.Process) *wmdef.Process {
	filtered := *p // Copy the struct

	// Based on Process.String() - non-verbose shows PID, Command, Language
	// For now, return as-is since Process doesn't have many verbose-only fields

	return &filtered
}

// filterContainerImageMetadata removes verbose-only fields from ContainerImageMetadata
func filterContainerImageMetadata(img *wmdef.ContainerImageMetadata) *wmdef.ContainerImageMetadata {
	filtered := *img // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(img.EntityMeta)

	// Based on ContainerImageMetadata.String(verbose=false):
	// Non-verbose shows: EntityID, EntityMeta (filtered), RepoTags, RepoDigests
	// Verbose-only: MediaType, SizeBytes, OS, OSVersion, Architecture, Variant, Layers, SBOM
	filtered.MediaType = ""
	filtered.SizeBytes = 0
	filtered.OS = ""
	filtered.OSVersion = ""
	filtered.Architecture = ""
	filtered.Variant = ""
	filtered.Layers = nil
	filtered.SBOM = nil

	return &filtered
}

// filterGPU removes verbose-only fields from GPU
func filterGPU(g *wmdef.GPU) *wmdef.GPU {
	filtered := *g // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(g.EntityMeta)

	// GPU.String() doesn't have verbose-specific logic, but we filter EntityMeta for consistency
	return &filtered
}

// filterKubeCapabilities removes verbose-only fields from KubeCapabilities
func filterKubeCapabilities(kc *wmdef.KubeCapabilities) *wmdef.KubeCapabilities {
	filtered := *kc // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(kc.EntityMeta)

	// Based on KubeCapabilities.String(verbose=false):
	// Non-verbose shows: EntityID, EntityMeta (filtered), Version
	// Verbose-only: FeatureGates
	filtered.FeatureGates = nil

	return &filtered
}

// filterKubelet removes verbose-only fields from Kubelet
func filterKubelet(ku *wmdef.Kubelet) *wmdef.Kubelet {
	filtered := *ku // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(ku.EntityMeta)

	// Based on Kubelet.String(verbose=false):
	// Non-verbose shows: EntityID, EntityMeta (filtered), NodeName
	// Verbose-only: ConfigDocument, RawConfig
	filtered.ConfigDocument = wmdef.KubeletConfigDocument{}
	filtered.RawConfig = nil

	return &filtered
}

// filterKubernetesDeployment removes verbose-only fields from KubernetesDeployment
func filterKubernetesDeployment(d *wmdef.KubernetesDeployment) *wmdef.KubernetesDeployment {
	filtered := *d // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(d.EntityMeta)

	// KubernetesDeployment.String() doesn't have verbose-specific logic,
	// but we filter EntityMeta for consistency
	return &filtered
}

// filterKubernetesMetadata removes verbose-only fields from KubernetesMetadata
func filterKubernetesMetadata(m *wmdef.KubernetesMetadata) *wmdef.KubernetesMetadata {
	filtered := *m // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(m.EntityMeta)

	// Based on KubernetesMetadata.String(verbose=false):
	// Non-verbose shows: EntityID, EntityMeta (filtered)
	// Verbose-only: GVR (GroupVersionResource)
	filtered.GVR = nil

	return &filtered
}

// filterCRD removes verbose-only fields from CRD
func filterCRD(crd *wmdef.CRD) *wmdef.CRD {
	filtered := *crd // Copy the struct

	// Filter EntityMeta to only show Name and Namespace
	filtered.EntityMeta = filterEntityMeta(crd.EntityMeta)

	// CRD.String() doesn't have verbose-specific logic,
	// but we filter EntityMeta for consistency
	return &filtered
}
