// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmetaimpl

import (
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

// filterContainer removes verbose-only fields from Container.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterContainer(c *wmdef.Container) *wmdef.Container {
	return c.FilterForNonVerbose()
}

// filterKubernetesPod removes verbose-only fields from KubernetesPod.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterKubernetesPod(p *wmdef.KubernetesPod) *wmdef.KubernetesPod {
	return p.FilterForNonVerbose()
}

// filterECSTask removes verbose-only fields from ECSTask.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterECSTask(t *wmdef.ECSTask) *wmdef.ECSTask {
	return t.FilterForNonVerbose()
}

// filterProcess removes verbose-only fields from Process.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterProcess(p *wmdef.Process) *wmdef.Process {
	return p.FilterForNonVerbose()
}

// filterContainerImageMetadata removes verbose-only fields from ContainerImageMetadata.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterContainerImageMetadata(img *wmdef.ContainerImageMetadata) *wmdef.ContainerImageMetadata {
	return img.FilterForNonVerbose()
}

// filterGPU removes verbose-only fields from GPU.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterGPU(g *wmdef.GPU) *wmdef.GPU {
	return g.FilterForNonVerbose()
}

// filterKubeCapabilities removes verbose-only fields from KubeCapabilities.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterKubeCapabilities(kc *wmdef.KubeCapabilities) *wmdef.KubeCapabilities {
	return kc.FilterForNonVerbose()
}

// filterKubelet removes verbose-only fields from Kubelet.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterKubelet(ku *wmdef.Kubelet) *wmdef.Kubelet {
	return ku.FilterForNonVerbose()
}

// filterKubernetesDeployment removes verbose-only fields from KubernetesDeployment.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterKubernetesDeployment(d *wmdef.KubernetesDeployment) *wmdef.KubernetesDeployment {
	return d.FilterForNonVerbose()
}

// filterKubernetesMetadata removes verbose-only fields from KubernetesMetadata.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterKubernetesMetadata(m *wmdef.KubernetesMetadata) *wmdef.KubernetesMetadata {
	return m.FilterForNonVerbose()
}

// filterCRD removes verbose-only fields from CRD.
// Uses the centralized FilterForNonVerbose method in types.go to ensure
// consistency between text output (String method) and JSON output.
func filterCRD(crd *wmdef.CRD) *wmdef.CRD {
	return crd.FilterForNonVerbose()
}
