// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	core                              *mutatorCore
	targets                           []targetInternal
	disabledNamespaces                map[string]bool
	securityClientLibraryPodMutators  []podMutator
	profilingClientLibraryPodMutators []podMutator
	containerRegistry                 string
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component) (*TargetMutator, error) {
	// Determine default disabled namespaces.
	defaultDisabled := mutatecommon.DefaultDisabledNamespaces()

	// Create a map of disabled namespaces for quick lookups.
	disabledNamespacesMap := make(map[string]bool, len(config.Instrumentation.DisabledNamespaces)+len(defaultDisabled))
	for _, ns := range config.Instrumentation.DisabledNamespaces {
		disabledNamespacesMap[ns] = true
	}
	for _, ns := range defaultDisabled {
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

	m := &TargetMutator{
		targets:                           internalTargets,
		disabledNamespaces:                disabledNamespacesMap,
		securityClientLibraryPodMutators:  config.securityClientLibraryPodMutators,
		profilingClientLibraryPodMutators: config.profilingClientLibraryPodMutators,
		containerRegistry:                 config.containerRegistry,
	}

	// Create the core mutator. This is a bit gross. The target mutator is also the filter which we are passing in.
	core := newMutatorCore(config, wmeta, m)
	m.core = core

	return m, nil
}

// MutatePod mutates the pod if it matches the target based workload selection or has the appropriate annotations.
func (m *TargetMutator) MutatePod(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	// Sanitize input.
	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}
	if pod.Namespace == "" {
		pod.Namespace = ns
	}

	// If the namespace is disabled, we should not mutate the pod.
	if _, ok := m.disabledNamespaces[pod.Namespace]; ok {
		return false, nil
	}

	// The admission can be re-run for the same pod. Fast return if we injected the library already.
	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), mutatecommon.PodString(pod))
			return false, nil
		}
	}

	// Get the libraries to inject. If there are no libraries to inject, we should not mutate the pod.
	libraries := m.getLibraries(pod)
	if len(libraries) == 0 {
		return false, nil
	}
	extracted := m.core.initExtractedLibInfo(pod).withLibs(libraries)

	// Add the configuration for the security client library.
	for _, mutator := range m.securityClientLibraryPodMutators {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for security client: %w", err)
		}
	}

	// Add the configuration for profiling.
	for _, mutator := range m.profilingClientLibraryPodMutators {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for profiling client: %w", err)
		}
	}

	// Inject the libraries.
	err := m.core.injectTracers(pod, extracted)
	if err != nil {
		return false, fmt.Errorf("error injecting libraries: %w", err)
	}

	return true, nil
}

// ShouldMutatePod checks if a pod is mutable. This is used by other mutators to determine if they should mutate a pod.
func (m *TargetMutator) ShouldMutatePod(pod *corev1.Pod) bool {
	return len(m.getLibraries(pod)) > 0
}

// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
func (m *TargetMutator) IsNamespaceEligible(namespace string) bool {
	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := m.disabledNamespaces[namespace]; ok {
		return false
	}

	// Check if the namespace matches any of the targets.
	for _, target := range m.targets {
		matches, err := target.matchesNamespaceSelector(namespace)
		if err != nil {
			log.Errorf("error encountered matching targets, aborting all together to avoid inaccurate match: %v", err)
			return false

		}
		if matches {
			log.Debugf("Namespace %q matched target %q", namespace, target.name)
			return true
		}
	}

	// No target matched.
	return false
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

// getLibraries determines which tracing libraries to use given a pod. It returns the list of tracing libraries to
// inject.
func (m *TargetMutator) getLibraries(pod *corev1.Pod) []libInfo {
	// If the pod has explicit tracer libraries defined as annotations, they take precedence.
	libraries := m.getAnnotationLibraries(pod)
	if len(libraries) > 0 {
		return libraries
	}

	// If there are no annotations, check if the pod matches any of the targets.
	return m.getTargetLibraries(pod)
}

// getAnnotationLibraries determines which tracing libraries to use given a pod's annotations. It returns the list of
// tracing libraries to inject.
func (m *TargetMutator) getAnnotationLibraries(pod *corev1.Pod) []libInfo {
	return extractLibrariesFromAnnotations(pod, m.containerRegistry)
}

// getTargetLibraries filters a pod based on the targets. It returns the list of libraries to inject.
func (m *TargetMutator) getTargetLibraries(pod *corev1.Pod) []libInfo {
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

		log.Debugf("Pod %q matched target %q", mutatecommon.PodString(pod), target.name)

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
