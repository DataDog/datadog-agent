// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	targets            []targetInternal
	disabledNamespaces map[string]bool
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component) (*TargetMutator, error) {
	// Create a map of disabled namespaces for quick lookups.
	disabledNamespacesMap := make(map[string]bool, len(config.Instrumentation.DisabledNamespaces))
	for _, ns := range config.Instrumentation.DisabledNamespaces {
		disabledNamespacesMap[ns] = true
	}

	// Convert the targets to internal format.
	internalTargets := make([]targetInternal, len(config.Instrumentation.Targets))
	for i, t := range config.Instrumentation.Targets {
		// Convert the pod selector to a label selector.
		podSelector, err := t.PodSelector.AsLabelSelector()
		if err != nil {
			return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
		}

		// Determine if we should use the namespace selector or if we should use enabledNamespaces.
		useNamespaceSelector := len(t.NamespaceSelector.MatchLabels)+len(t.NamespaceSelector.MatchExpressions) > 0

		// Convert the namespace selector to a label selector.
		var namespaceSelector labels.Selector
		if useNamespaceSelector {
			namespaceSelector, err = t.NamespaceSelector.AsLabelSelector()
			if err != nil {
				return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
			}
		}

		// Create a map of enabled namespaces for quick lookups.
		var enabledNamespaces map[string]bool
		if !useNamespaceSelector {
			enabledNamespaces = make(map[string]bool, len(t.NamespaceSelector.MatchNames))
			for _, ns := range t.NamespaceSelector.MatchNames {
				enabledNamespaces[ns] = true
			}
		}

		// Get the library versions to inject. If no versions are specified, we inject all libraries.
		var libVersions []libInfo
		if len(t.TracerVersions) == 0 {
			libVersions = getAllLatestDefaultLibraries(config.containerRegistry)
		} else {
			libVersions = getPinnedLibraries(t.TracerVersions, config.containerRegistry)
		}

		// Store the target in the internal format.
		internalTargets[i] = targetInternal{
			name:                 t.Name,
			podSelector:          podSelector,
			useNamespaceSelector: useNamespaceSelector,
			nameSpaceSelector:    namespaceSelector,
			wmeta:                wmeta,
			enabledNamespaces:    enabledNamespaces,
			libVersions:          libVersions,
		}
	}

	return &TargetMutator{
		targets:            internalTargets,
		disabledNamespaces: disabledNamespacesMap,
	}, nil
}

// targetInternal is the struct we use to convert the config based target into
// something more performant to check against.
type targetInternal struct {
	name                 string
	podSelector          labels.Selector
	nameSpaceSelector    labels.Selector
	useNamespaceSelector bool
	enabledNamespaces    map[string]bool
	libVersions          []libInfo
	wmeta                workloadmeta.Component
}

// filter filters a pod based on the targets. It returns the list of libraries to inject.
func (m *TargetMutator) filter(pod *corev1.Pod) []libInfo {
	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := m.disabledNamespaces[pod.Namespace]; ok {
		return nil
	}

	// Check if the pod matches any of the targets. The first match wins.
	for _, target := range m.targets {
		// Check the pod namespace against the namespace selector.
		matches, err := target.matchesNamespaceSelector(pod.Namespace)
		if err != nil {
			log.Errorf("error encountered matching targets, aborting all together to avoid inaccurate match: %v", err)
			return nil

		}
		if !matches {
			continue
		}

		// Check the pod labels against the pod selector.
		if !target.matchesPodSelector(pod.Labels) {
			continue
		}

		// If the namespace and pod selector match, return the libraries to inject.
		return target.libVersions
	}

	// No target matched.
	return nil
}

func (t targetInternal) matchesNamespaceSelector(namespace string) (bool, error) {
	// If we are using the namespace selector, check if the namespace matches the selector.
	if t.useNamespaceSelector {
		// Get the namespace metadata.
		id := util.GenerateKubeMetadataEntityID("", "namespaces", "", namespace)
		ns, err := t.wmeta.GetKubernetesMetadata(id)
		if err != nil {
			return false, fmt.Errorf("could not get kubernetes namespace to match against for %s: %w", namespace, err)
		}

		// Check if the namespace labels match the selector.
		return t.nameSpaceSelector.Matches(labels.Set(ns.EntityMeta.Labels)), nil
	}

	// If there are no match names, we match all namespaces.
	if len(t.enabledNamespaces) == 0 {
		return true, nil
	}

	// Check if the pod namespace is in the match names.
	_, ok := t.enabledNamespaces[namespace]
	return ok, nil
}

func (t targetInternal) matchesPodSelector(podLabels map[string]string) bool {
	return t.podSelector.Matches(labels.Set(podLabels))
}
