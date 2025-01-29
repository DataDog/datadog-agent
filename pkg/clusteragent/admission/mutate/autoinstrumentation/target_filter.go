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
)

// TargetFilter filters pods based on a set of targeting rules.
type TargetFilter struct {
	targets            []targetInternal
	disabledNamespaces map[string]bool
}

// targetInternal is the struct we use to conver the config based target into
// something more performant to check against.
type targetInternal struct {
	name              string
	podSelector       labels.Selector
	enabledNamespaces map[string]bool
	libVersions       []libInfo
}

// NewTargetFilter creates a new TargetFilter from a list of targets and disabled namespaces. We convert the targets
// to a more efficient internal format for quick lookups.
func NewTargetFilter(targets []Target, disabledNamespaces []string, containerRegistry string) (*TargetFilter, error) {
	// Create a map of disabled namespaces for quick lookups.
	disabledNamespacesMap := make(map[string]bool, len(disabledNamespaces))
	for _, ns := range disabledNamespaces {
		disabledNamespacesMap[ns] = true
	}

	// Convert the targets to internal format.
	internalTargets := make([]targetInternal, len(targets))
	for i, t := range targets {
		// Convert the pod selector to a label selector.
		podSelector, err := t.Selector.AsLabelSelector()
		if err != nil {
			return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
		}

		// Create a map of enabled namespaces for quick lookups.
		enabledNamespaces := make(map[string]bool, len(t.NamespaceSelector.MatchNames))
		for _, ns := range t.NamespaceSelector.MatchNames {
			enabledNamespaces[ns] = true
		}

		// Get the library versions to inject. If no versions are specified, we inject all libraries.
		var libVersions []libInfo
		if len(t.TracerVersions) == 0 {
			libVersions = getAllLatestDefaultLibraries(containerRegistry)
		} else {
			libVersions = getPinnedLibraries(t.TracerVersions, containerRegistry)
		}

		// Store the target in the internal format.
		internalTargets[i] = targetInternal{
			name:              t.Name,
			podSelector:       podSelector,
			enabledNamespaces: enabledNamespaces,
			libVersions:       libVersions,
		}
	}

	return &TargetFilter{
		targets:            internalTargets,
		disabledNamespaces: disabledNamespacesMap,
	}, nil
}

// filter filters a pod based on the targets. It returns the list of libraries to inject.
func (f *TargetFilter) filter(pod *corev1.Pod) []libInfo {
	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := f.disabledNamespaces[pod.Namespace]; ok {
		return nil
	}

	// Check if the pod matches any of the targets. The first match wins.
	for _, target := range f.targets {
		// Check the pod namespace against the namespace selector.
		if !matchesNamespaceSelector(pod, target.enabledNamespaces) {
			continue
		}

		// Check the pod labels against the pod selector.
		if !target.podSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		// If the namespace and pod selector match, return the libraries to inject.
		return target.libVersions
	}

	// No target matched.
	return nil
}

func matchesNamespaceSelector(pod *corev1.Pod, enabledNamespaces map[string]bool) bool {
	// If there are no match names, the selector matches all namespaces.
	if len(enabledNamespaces) == 0 {
		return true
	}

	// Check if the pod namespace is in the match names.
	_, ok := enabledNamespaces[pod.Namespace]
	return ok
}
