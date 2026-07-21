// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/config"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// completenessTracker determines whether entities are "complete", meaning all
// expected collectors have reported data for them. It owns the mapping of
// entity kinds to the sources that are expected to report data for them.
//
// TODO: For now, the expected sources map is static and not updated when a
// collector permanently fails. A permanent failure means entities waiting on
// that source will never be considered complete.
type completenessTracker struct {
	expectedSources map[wmdef.Kind][]wmdef.Source
}

// newCompletenessTracker creates a completenessTracker with expected sources
// initialized from the detected environment features. This determines which
// collectors are expected to report data for each entity kind.
//
// Note: Kubernetes Deployments are also reported by multiple collectors
// (kubeapiserver and language detection), but completeness tracking is not
// needed for them.
func newCompletenessTracker(agentType wmdef.AgentType, cfg config.Component) *completenessTracker {
	return &completenessTracker{expectedSources: initExpectedSources(agentType, cfg)}
}

// isComplete reports whether an entity of the given kind is complete, meaning
// all expected collectors have reported data for it. If no expected sources
// are defined for the entity kind, it returns true (considered complete by
// default).
func (c *completenessTracker) isComplete(kind wmdef.Kind, reportedSources []wmdef.Source) bool {
	expectedSources, ok := c.expectedSources[kind]
	if !ok || len(expectedSources) == 0 {
		return true
	}

	for _, expectedSource := range expectedSources {
		if !slices.Contains(reportedSources, expectedSource) {
			return false
		}
	}

	return true
}

// initExpectedSources returns the expected sources map for the detected
// environment. Returns nil when no multi-source completeness tracking is
// needed.
func initExpectedSources(agentType wmdef.AgentType, cfg config.Component) map[wmdef.Kind][]wmdef.Source {
	// Only the Node Agent runs multiple collectors that need to report
	// for an entity to be complete.
	if agentType != wmdef.NodeAgent {
		return nil
	}

	// In ECS EC2 and ECS Managed (no sidecar), containers are reported by two
	// collectors (ECS + container runtime). In sidecar mode (Fargate, or
	// Managed Instances configured as sidecar), there's a single collector, so
	// entities are always complete.
	switch {
	case env.IsFeaturePresent(env.Kubernetes):
		return expectedSourcesKubernetes()
	case env.IsFeaturePresent(env.ECSEC2) || (env.IsFeaturePresent(env.ECSManagedInstances) && !env.IsECSSidecarMode(cfg)):
		return expectedSourcesECS()
	}

	return nil
}

func expectedSourcesKubernetes() map[wmdef.Kind][]wmdef.Source {
	// In Kubernetes, containers are reported by:
	// - kubelet collector (SourceNodeOrchestrator)
	// - container runtime collector if accessible (SourceRuntime)
	containerSources := []wmdef.Source{wmdef.SourceNodeOrchestrator}
	if containerRuntimeIsAccessible() {
		containerSources = append(containerSources, wmdef.SourceRuntime)
	}

	return map[wmdef.Kind][]wmdef.Source{
		// In Kubernetes, pods are reported by:
		// - kubelet collector (SourceNodeOrchestrator)
		// - kubemetadata collector (SourceClusterOrchestrator)
		wmdef.KindKubernetesPod: {
			wmdef.SourceNodeOrchestrator,
			wmdef.SourceClusterOrchestrator,
		},
		wmdef.KindContainer: containerSources,
	}
}

func expectedSourcesECS() map[wmdef.Kind][]wmdef.Source {
	// In ECS EC2 and ECS Managed (no sidecar), containers are reported by:
	// - ECS collector (SourceNodeOrchestrator)
	// - container runtime collector (SourceRuntime)
	containerSources := []wmdef.Source{wmdef.SourceNodeOrchestrator}
	if containerRuntimeIsAccessible() {
		containerSources = append(containerSources, wmdef.SourceRuntime)
	}
	return map[wmdef.Kind][]wmdef.Source{
		wmdef.KindContainer: containerSources,
	}
}

func containerRuntimeIsAccessible() bool {
	runtimes := []env.Feature{env.Docker, env.Containerd, env.Crio, env.Podman}
	return slices.ContainsFunc(runtimes, env.IsFeaturePresent)
}
