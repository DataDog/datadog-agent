// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// AppliedTargetEnvVar is the environment variable that contains the JSON of the target that was applied to the pod.
	AppliedTargetEnvVar = "DD_INSTRUMENTATION_APPLIED_TARGET"
)

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	enabled                       bool
	core                          *mutatorCore
	targets                       []targetInternal
	disabledNamespaces            map[string]bool
	securityClientLibraryMutator  containerMutator
	profilingClientLibraryMutator containerMutator
	containerRegistry             string
	mutateUnlabelled              bool
	defaultLibVersions            []libInfo
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver) (*TargetMutator, error) {
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

	// Fetch the default lib versions to use if there are no user defined versions.
	defaultLibVersions := getAllLatestDefaultLibraries(config.containerRegistry)

	// If there are no targets, we should fall back to enabledNamespace/libVersions. If those are also not defined, the
	// expected behavior is to inject all pods into all namespaces.
	var internalTargets []targetInternal
	if config.Instrumentation.Enabled {
		targets := config.Instrumentation.Targets
		if len(targets) == 0 {
			targets = append(targets, createDefaultTarget(config.Instrumentation.EnabledNamespaces, config.Instrumentation.LibVersions))
		}

		// Convert the targets to internal format.
		internalTargets = make([]targetInternal, len(targets))
		for i, t := range targets {
			// Convert the pod selector to a label selector.
			podSelector := labels.Everything()
			var err error
			if t.PodSelector != nil {
				podSelector, err = t.PodSelector.AsLabelSelector()
				if err != nil {
					return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
				}
			}

			// Determine if we should use the namespace selector or if we should use enabledNamespaces.
			useNamespaceSelector := t.NamespaceSelector != nil && len(t.NamespaceSelector.MatchLabels)+len(t.NamespaceSelector.MatchExpressions) > 0

			// Convert the namespace selector to a label selector.
			namespaceSelector := labels.Everything()
			if useNamespaceSelector && t.NamespaceSelector != nil {
				namespaceSelector, err = t.NamespaceSelector.AsLabelSelector()
				if err != nil {
					return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
				}
			}

			// Create a map of enabled namespaces for quick lookups.
			var enabledNamespaces map[string]bool
			if !useNamespaceSelector && t.NamespaceSelector != nil {
				enabledNamespaces = make(map[string]bool, len(t.NamespaceSelector.MatchNames))
				for _, ns := range t.NamespaceSelector.MatchNames {
					enabledNamespaces[ns] = true
				}
			}

			// We build the libVersions based on if they are specified in `tracerVersions` else ask the higher-level configuration from `libVersions`
			// and/or defer to language detection.
			var libVersions []libInfo
			usesDefaultLibs := false
			if len(t.TracerVersions) == 0 {
				libVersions = defaultLibVersions
				usesDefaultLibs = true
			} else {
				pinnedLibraries := getPinnedLibraries(t.TracerVersions, config.containerRegistry, true)
				usesDefaultLibs = pinnedLibraries.areSetToDefaults
				libVersions = pinnedLibraries.libs
			}

			// Convert the tracer configs to env vars. We check that the env var names start with the DD_ prefix to avoid
			// this from being used as a generic env var injector. If there is a product requirement to allow arbitrary env
			// vars in the future, we could relax this requirement.
			envVars := make([]corev1.EnvVar, len(t.TracerConfigs))
			for i, tc := range t.TracerConfigs {
				if !strings.HasPrefix(tc.Name, "DD_") {
					return nil, fmt.Errorf("tracer config %q does not start with DD_", tc.Name)
				}
				envVars[i] = tc.AsEnvVar()
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
				envVars:              envVars,
				json:                 createJSON(t),
				usesDefaultLibs:      usesDefaultLibs,
			}
		}
	}

	m := &TargetMutator{
		enabled:                       config.Instrumentation.Enabled,
		targets:                       internalTargets,
		disabledNamespaces:            disabledNamespacesMap,
		securityClientLibraryMutator:  config.securityClientLibraryMutator,
		profilingClientLibraryMutator: config.profilingClientLibraryMutator,
		containerRegistry:             config.containerRegistry,
		mutateUnlabelled:              config.mutateUnlabelled,
		defaultLibVersions:            defaultLibVersions,
	}

	// Create the core mutator. This is a bit gross.
	// The target mutator is also the filter which we are passing in.
	core := newMutatorCore(config, wmeta, m, imageResolver)
	m.core = core

	return m, nil
}

// MutatePod mutates the pod if it matches the target based workload selection or has the appropriate annotations.
func (m *TargetMutator) MutatePod(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	log.Debugf("Mutating pod in target mutator %q", mutatecommon.PodString(pod))

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

	log.Debugf("Mutating pod in target mutator %q", mutatecommon.PodString(pod))

	// The admission can be re-run for the same pod. Fast return if we injected the library already.
	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), mutatecommon.PodString(pod))
			return false, nil
		}
	}

	// Get the target to inject. If there is not target, we should not mutate the pod.
	target := m.getTarget(pod)
	if target == nil {
		return false, nil
	}
	extracted := m.core.initExtractedLibInfo(pod).withLibs(target.libVersions)

	// If the user did not specify versions, this target is eligible for language detection.
	if target.usesDefaultLibs {
		extractedLanguageDetection, usingLanguageDetection := extracted.useLanguageDetectionLibs()
		if usingLanguageDetection {
			extracted = extractedLanguageDetection
		}
	}

	// Add the configuration for the security client library.
	if err := m.core.mutatePodContainers(pod, m.securityClientLibraryMutator, true); err != nil {
		return false, fmt.Errorf("error mutating pod for security client: %w", err)
	}

	// Add the configuration for profiling.
	if err := m.core.mutatePodContainers(pod, m.profilingClientLibraryMutator, true); err != nil {
		return false, fmt.Errorf("error mutating pod for profiling client: %w", err)
	}

	// Inject the tracer configs. We do this before lib injection to ensure DD_SERVICE is set if the user configures it
	// in the target.
	for _, envVar := range target.envVars {
		_ = m.core.mutatePodContainers(pod, envVarMutator(envVar), true)
	}

	// Inject the libraries.
	err := m.core.injectTracers(pod, extracted)
	if err != nil {
		return false, fmt.Errorf("error injecting libraries: %w", err)
	}

	// Only add annotations/env vars if there is a target json to set. This would be blank for local lib injection.
	if target.json != "" {
		m.addTargetJSONInfo(pod, target)
	}

	return true, nil
}

func (m *TargetMutator) addTargetJSONInfo(pod *corev1.Pod, target *targetInternal) {
	// Inject the target json. The is added so that the injector can make use of the target information.
	_ = m.core.mutatePodContainers(pod, envVarMutator(corev1.EnvVar{
		Name:  AppliedTargetEnvVar,
		Value: target.json,
	}), true)

	// Add the annotations to the pod.
	annotation.Set(pod, annotation.AppliedTarget, target.json)
}

// ShouldMutatePod determines if a pod would be mutated by the target mutator. It is used by other webhook mutators as
// a filter.
func (m *TargetMutator) ShouldMutatePod(pod *corev1.Pod) bool {
	// If the namespace is disabled, we should not mutate the pod.
	if _, ok := m.disabledNamespaces[pod.Namespace]; ok {
		return false
	}

	// We need to explicitly check for the label being set to false, which opts out of mutation.
	enabledLabelVal, enabledLabelExists := getEnabledLabel(pod)
	if enabledLabelExists && !enabledLabelVal {
		return false
	}

	// At this point, we should only mutate if a target matches.
	return m.getTarget(pod) != nil
}

// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
func (m *TargetMutator) IsNamespaceEligible(namespace string) bool {
	// Return if the mutator is disabled.
	if !m.enabled {
		return false
	}

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

// targetInternal is the struct we use to convert the config based target into something more performant.
type targetInternal struct {
	name                 string
	podSelector          labels.Selector
	nameSpaceSelector    labels.Selector
	useNamespaceSelector bool
	enabledNamespaces    map[string]bool
	libVersions          []libInfo
	envVars              []corev1.EnvVar
	wmeta                workloadmeta.Component
	json                 string
	usesDefaultLibs      bool
}

// getTarget determines which target to use for a given a pod, which includes the set of tracing libraries to inject.
func (m *TargetMutator) getTarget(pod *corev1.Pod) *targetInternal {
	result := m.getTargetFromAnnotation(pod)
	if !result.shouldContinue {
		return result.target
	}

	return m.getMatchingTarget(pod)
}

type annotationResult struct {
	shouldContinue bool
	target         *targetInternal
}

// getTargetFromAnnotation determines which tracing libraries to use given
func (m *TargetMutator) getTargetFromAnnotation(pod *corev1.Pod) *annotationResult {
	// The enabled label existing takes precedence...
	enabledLabelVal, enabledLabelExists := getEnabledLabel(pod)
	if enabledLabelExists && !enabledLabelVal {
		return &annotationResult{
			shouldContinue: false,
			target:         nil,
		}
	}

	if !enabledLabelExists && !m.mutateUnlabelled {
		return &annotationResult{
			shouldContinue: true,
			target:         nil,
		}
	}

	// If local lib is enabled, then we should prefer the user defined libs.
	extractedLibraries := extractLibrariesFromAnnotations(pod, m.containerRegistry)
	if len(extractedLibraries) > 0 {
		return &annotationResult{
			shouldContinue: false,
			target: &targetInternal{
				libVersions: extractedLibraries,
			},
		}
	}

	injectAllAnnotation := strings.ToLower(annotation.LibraryVersion.Format("all"))
	if _, found := pod.Annotations[injectAllAnnotation]; found {
		return &annotationResult{
			shouldContinue: false,
			target: &targetInternal{
				libVersions: m.defaultLibVersions,
			},
		}
	}

	return &annotationResult{
		shouldContinue: true,
		target:         nil,
	}
}

// getMatchingTarget filters a pod based on the targets. It returns the target to inject.
func (m *TargetMutator) getMatchingTarget(pod *corev1.Pod) *targetInternal {
	// If instrumentation is disabled, we don't need to check the targets.
	if !m.enabled {
		return nil
	}

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
		return &target
	}

	// No target matched.
	return nil
}

func (t targetInternal) matchesNamespaceSelector(namespace string) (bool, error) {
	// If we are using the namespace selector, check if the namespace matches the selector.
	if t.useNamespaceSelector {
		nsLabels, err := getNamespaceLabels(t.wmeta, namespace)
		if err != nil {
			return false, fmt.Errorf("could not get labels to match: %w", err)
		}

		// Check if the namespace labels match the selector.
		return t.nameSpaceSelector.Matches(labels.Set(nsLabels)), nil
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

// createDefaultTarget is used when there are no targets. If a user configures enabledNamespaces and libVersions, which
// are mutually exclusive with a list of targets, then we need to translate those configuration options into a target.
// Additionally, if there are no targets and enabledNamespaces/libVersions are not set, the expected behavior is that
// we would inject all SDKs to all pods. This target encompasses both of those cases.
func createDefaultTarget(namespaces []string, pinnedLibVersions map[string]string) Target {
	// Create a default target.
	target := Target{
		Name: "default",
	}

	// If there are pinned versions, set them.
	if len(pinnedLibVersions) > 0 {
		target.TracerVersions = pinnedLibVersions
	}

	// Add a namespace selector if a list of namespaces is configured.
	if len(namespaces) > 0 {
		target.NamespaceSelector = &NamespaceSelector{
			MatchNames: namespaces,
		}
	}

	return target
}

// createJSON creates a json string of the target used to apply as an annotation.
func createJSON(t Target) string {
	data, err := json.Marshal(t)
	if err != nil {
		log.Errorf("error marshalling target %q: %v", t.Name, err)
		return fmt.Sprintf("error marshalling target %q: %v", t.Name, err)
	}
	return string(data)
}

// getEnabledLable is a helper function to convert the found value from a string
// to a boolean.
func getEnabledLabel(pod *corev1.Pod) (bool, bool) {
	val, found := pod.GetLabels()[common.EnabledLabelKey]
	if !found {
		return false, found
	}

	if val == "true" {
		return true, found
	}

	return false, found
}

// getAllLatestDefaultLibraries returns all supported by APM Instrumentation tracing libraries
// that should be enabled by default
func getAllLatestDefaultLibraries(containerRegistry string) []libInfo {
	var libsToInject []libInfo
	for _, lang := range supportedLanguages {
		libsToInject = append(libsToInject, lang.defaultLibInfo(containerRegistry, ""))
	}

	return libsToInject
}

func getNamespaceLabels(wmeta workloadmeta.Component, name string) (map[string]string, error) {
	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", name)
	ns, err := wmeta.GetKubernetesMetadata(id)
	if err != nil {
		return nil, fmt.Errorf("error getting namespace metadata for ns=%s: %w", name, err)
	}

	return ns.EntityMeta.Labels, nil
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}

func extractLibrariesFromAnnotations(pod *corev1.Pod, registry string) []libInfo {
	libs := []libInfo{}

	// Check all supported languages for potential Local SDK Injection.
	for _, l := range supportedLanguages {
		// Check for a custom library image.
		customImage, found := annotation.Get(pod, annotation.LibraryImage.Format(string(l)))
		if found {
			libs = append(libs, l.libInfo("", customImage))
		}

		// Check for a custom library version.
		libVersion, found := annotation.Get(pod, annotation.LibraryVersion.Format(string(l)))
		if found {
			libs = append(libs, l.libInfoWithResolver("", registry, libVersion))
		}

		// Check all containers in the pod for container specific Local SDK Injection.
		for _, container := range pod.Spec.Containers {
			// Check for custom library image.
			customImage, found := annotation.Get(pod, annotation.LibraryContainerImage.Format(container.Name, string(l)))
			if found {
				libs = append(libs, l.libInfo(container.Name, customImage))
			}

			// Check for custom library version.
			libVersion, found := annotation.Get(pod, annotation.LibraryContainerVersion.Format(container.Name, string(l)))
			if found {
				libs = append(libs, l.libInfoWithResolver(container.Name, registry, libVersion))
			}
		}
	}

	return libs
}
