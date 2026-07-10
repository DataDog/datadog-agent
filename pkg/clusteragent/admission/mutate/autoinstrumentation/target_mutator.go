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

	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// AppliedTargetEnvVar is the environment variable that contains the JSON of the target that was applied to the pod.
	AppliedTargetEnvVar = "DD_INSTRUMENTATION_APPLIED_TARGET"
	// AppliedPolicyEnvVar is the environment variable that contains the compact JSON of the policy that was applied to the pod.
	AppliedPolicyEnvVar = "DD_INSTRUMENTATION_APPLIED_POLICY"
)

// policySet is an immutable, atomically swappable view of the policies a
// TargetMutator matches against. matcher.policies and targets are aligned by
// index, so a match resolves directly to its injection config. A disabled
// mutator is simply represented by an empty set (no targets, no policies),
// which naturally matches nothing.
type policySet struct {
	targets []targetInternal
	matcher *policyMatcher
}

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	core                          *mutatorCore
	disabledNamespaces            map[string]bool
	securityClientLibraryMutator  containerMutator
	profilingClientLibraryMutator containerMutator
	containerRegistry             string
	mutateUnlabelled              bool
	defaultLibVersions            []libInfo

	// base is the policy set derived from the agent configuration file. It is
	// the baseline that remote-config policies are layered on top of.
	base policySet
	// active is the effective policy set: base when no remote policies are
	// present, or remote policies (taking precedence) layered on top of base
	// otherwise. It is swapped atomically on remote-config updates.
	active atomic.Pointer[policySet]
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups. When rcClient is non-nil, the mutator also subscribes to
// remote-config SSI policies, which are layered on top of the configuration baseline at runtime.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver, csiDriverWatcher libraryinjection.CSIDriverWatcher, rcClient *rcclient.Client) (*TargetMutator, error) {
	// If there are no targets, we should fall back to enabledNamespace/libVersions. If those are also not defined, the
	// expected behavior is to inject all pods into all namespaces.
	targets := config.Instrumentation.Targets
	if config.Instrumentation.Enabled && len(targets) == 0 {
		targets = append(targets, createDefaultTarget(config.Instrumentation.EnabledNamespaces, config.Instrumentation.LibVersions))
	}

	m, err := newTargetMutatorWithTargets(config, wmeta, imageResolver, csiDriverWatcher, targets, config.Instrumentation.Enabled)
	if err != nil {
		return nil, err
	}

	m.subscribeRemoteConfig(rcClient)
	return m, nil
}

func newTargetMutatorWithTargets(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver, csiDriverWatcher libraryinjection.CSIDriverWatcher, targets []Target, enabled bool) (*TargetMutator, error) {
	// Create a map of user-configured disabled namespaces for quick lookups.
	// Default namespaces (kube-system, datadog agent namespace) are excluded at
	// the webhook layer via namespace selectors and not duplicated here.
	disabledNamespacesMap := make(map[string]bool, len(config.Instrumentation.DisabledNamespaces))
	for _, ns := range config.Instrumentation.DisabledNamespaces {
		disabledNamespacesMap[ns] = true
	}

	// Fetch the default lib versions to use if there are no user defined versions.
	defaultLibVersions := getAllLatestDefaultLibraries(config.containerRegistry)

	var internalTargets []targetInternal
	if enabled {
		var err error
		internalTargets, err = buildInternalTargets(config, wmeta, targets, defaultLibVersions)
		if err != nil {
			return nil, err
		}
	} else {
		// Keep the matcher aligned with the (empty) internal targets when
		// instrumentation is disabled so the active set stays consistent.
		targets = nil
	}

	// Lower the configuration targets into policies once, at the config
	// boundary. Everything past this point matches on policies only, aligned
	// by index with the internal targets above.
	configPolicies := policiesFromTargets(targets)

	m := &TargetMutator{
		disabledNamespaces:            disabledNamespacesMap,
		securityClientLibraryMutator:  config.securityClientLibraryMutator,
		profilingClientLibraryMutator: config.profilingClientLibraryMutator,
		containerRegistry:             config.containerRegistry,
		mutateUnlabelled:              config.mutateUnlabelled,
		defaultLibVersions:            defaultLibVersions,
		base: policySet{
			targets: internalTargets,
			matcher: newPolicyMatcher(configPolicies, wmeta),
		},
	}
	m.active.Store(&m.base)

	// Create the core mutator. This is a bit gross.
	// The target mutator is also the filter which we are passing in.
	core := newMutatorCore(config, wmeta, m, imageResolver, csiDriverWatcher)
	m.core = core

	return m, nil
}

// activeSet returns the effective policy set. It is never nil after the
// mutator has been constructed.
func (m *TargetMutator) activeSet() *policySet {
	return m.active.Load()
}

// SetRemotePolicies layers remote-config policies on top of the configuration
// baseline and swaps in the result atomically. Remote policies are evaluated
// first (they take precedence), then the configuration policies, preserving
// first-match-wins semantics.
func (m *TargetMutator) SetRemotePolicies(ps []policies.Policy) error {
	remoteTargets, err := buildInternalTargetsFromPolicies(m.core.config, m.core.wmeta, ps, m.defaultLibVersions)
	if err != nil {
		return err
	}

	combinedPolicies := make([]policies.Policy, 0, len(ps)+len(m.base.matcher.policies))
	combinedPolicies = append(combinedPolicies, ps...)
	combinedPolicies = append(combinedPolicies, m.base.matcher.policies...)

	combinedTargets := make([]targetInternal, 0, len(remoteTargets)+len(m.base.targets))
	combinedTargets = append(combinedTargets, remoteTargets...)
	combinedTargets = append(combinedTargets, m.base.targets...)

	m.active.Store(&policySet{
		targets: combinedTargets,
		matcher: newPolicyMatcher(combinedPolicies, m.core.wmeta),
	})
	return nil
}

// ClearRemotePolicies reverts the mutator to the configuration baseline.
func (m *TargetMutator) ClearRemotePolicies() {
	m.active.Store(&m.base)
}

// buildInternalTargetsFromPolicies resolves each policy's outcome (tracer
// versions and configs) into the internal injection format, mirroring
// buildInternalTargets but sourced from policies rather than Targets.
func buildInternalTargetsFromPolicies(config *Config, wmeta workloadmeta.Component, ps []policies.Policy, defaultLibVersions []libInfo) ([]targetInternal, error) {
	internalTargets := make([]targetInternal, len(ps))
	for i, p := range ps {
		var libVersions []libInfo
		usesDefaultLibs := false
		if len(p.Outcome.TracerVersions) == 0 {
			libVersions = defaultLibVersions
			usesDefaultLibs = true
		} else {
			pinnedLibraries := getPinnedLibraries(p.Outcome.TracerVersions, config.containerRegistry, true)
			usesDefaultLibs = pinnedLibraries.areSetToDefaults
			libVersions = pinnedLibraries.libs
		}

		envVars := make([]corev1.EnvVar, len(p.Outcome.TracerConfigs))
		for j, tc := range p.Outcome.TracerConfigs {
			if !strings.HasPrefix(tc.Name, "DD_") {
				return nil, fmt.Errorf("tracer config %q does not start with DD_", tc.Name)
			}
			envVars[j] = corev1.EnvVar{Name: tc.Name, Value: tc.Value}
		}

		internalTargets[i] = targetInternal{
			name:            p.Name,
			wmeta:           wmeta,
			libVersions:     libVersions,
			envVars:         envVars,
			json:            createPolicyJSON(p),
			usesDefaultLibs: usesDefaultLibs,
			fromPolicy:      true,
		}
	}

	return internalTargets, nil
}

func buildInternalTargets(config *Config, wmeta workloadmeta.Component, targets []Target, defaultLibVersions []libInfo) ([]targetInternal, error) {
	internalTargets := make([]targetInternal, len(targets))
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

	return internalTargets, nil
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

	// The admission can be re-run for the same pod (e.g. webhook reinvocation triggered by another
	// mutating webhook, as happens on GKE Autopilot). Fast return if we injected the library
	// already, otherwise we would mutate the pod a second time and, for instance, append the
	// injector to LD_PRELOAD twice.
	//
	// The instrumentation volume is added by every injection mode (init_container, image_volume and
	// CSI), so checking for it guards all modes. The CSI mode in particular has no init container,
	// so the per-init-container checks below would miss it.
	if containsVolume(pod, libraryinjection.InstrumentationVolumeName) {
		log.Debugf("Instrumentation volume %q already exists in pod %q", libraryinjection.InstrumentationVolumeName, mutatecommon.PodString(pod))
		return false, nil
	}
	// Check for the init_container mode's per-language init containers.
	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), mutatecommon.PodString(pod))
			return false, nil
		}
	}
	// Check for the image_volume mode's init container.
	if containsInitContainer(pod, libraryinjection.InjectLDPreloadInitContainerName) {
		log.Debugf("Init container %q already exists in pod %q", libraryinjection.InjectLDPreloadInitContainerName, mutatecommon.PodString(pod))
		return false, nil
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
	// Policy-driven and target-driven injection are surfaced through distinct
	// annotations/env vars: applied-target keeps its original "configuration
	// target" contract, while applied-policy carries a compact policy identity.
	envVarName := AppliedTargetEnvVar
	annotationKey := annotation.AppliedTarget
	if target.fromPolicy {
		envVarName = AppliedPolicyEnvVar
		annotationKey = annotation.AppliedPolicy
	}

	// Inject the json so that the injector can make use of the applied information.
	_ = m.core.mutatePodContainers(pod, envVarMutator(corev1.EnvVar{
		Name:  envVarName,
		Value: target.json,
	}), true)

	// Add the annotation to the pod.
	annotation.Set(pod, annotationKey, target.json)
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
	set := m.activeSet()

	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := m.disabledNamespaces[namespace]; ok {
		return false
	}

	// Check if the namespace matches any of the targets. A disabled mutator has
	// no targets, so this naturally returns false.
	for _, target := range set.targets {
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
	// fromPolicy is true when this internal target was derived from a policy
	// (remote config) rather than a configuration target. It selects which
	// annotation/env var carries the applied information.
	fromPolicy bool
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
				envVars:     extractTracerConfigsFromAnnotations(pod),
			},
		}
	}

	injectAllAnnotation := strings.ToLower(annotation.LibraryVersion.Format("all"))
	if _, found := pod.Annotations[injectAllAnnotation]; found {
		return &annotationResult{
			shouldContinue: false,
			target: &targetInternal{
				libVersions: m.defaultLibVersions,
				envVars:     extractTracerConfigsFromAnnotations(pod),
			},
		}
	}

	return &annotationResult{
		shouldContinue: true,
		target:         nil,
	}
}

// getMatchingTarget filters a pod based on the targets. It returns the target to inject.
//
// Matching is delegated to the native policy engine: each target is compiled
// into an equivalent policy (namespace and pod selectors ANDed together) and
// the first policy that evaluates to true wins, preserving the previous
// first-match semantics without relying on CGO or k8s label selectors.
func (m *TargetMutator) getMatchingTarget(pod *corev1.Pod) *targetInternal {
	set := m.activeSet()

	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := m.disabledNamespaces[pod.Namespace]; ok {
		return nil
	}

	// A disabled mutator has an empty set, so matchIndex returns -1 below.
	idx, err := set.matcher.matchIndex(pod)
	if err != nil {
		log.Errorf("error encountered matching targets, aborting all together to avoid inaccurate match: %v", err)
		return nil
	}
	if idx < 0 || idx >= len(set.targets) {
		return nil
	}

	// A matched policy may explicitly deny injection (first match wins).
	if !set.matcher.policies[idx].Outcome.Inject {
		log.Debugf("Pod %q matched policy %q which denies injection", mutatecommon.PodString(pod), set.targets[idx].name)
		return nil
	}

	log.Debugf("Pod %q matched target %q", mutatecommon.PodString(pod), set.targets[idx].name)
	return &set.targets[idx]
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

// createPolicyJSON creates the compact annotation payload for a policy-driven
// match. It intentionally omits the rule tree and keeps only the policy
// identity (name, version) and the tracer versions that were injected.
func createPolicyJSON(p policies.Policy) string {
	payload := struct {
		Name           string            `json:"name,omitempty"`
		ID             string            `json:"id,omitempty"`
		Version        int64             `json:"version,omitempty"`
		TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
	}{
		Name:           p.Name,
		ID:             p.ID,
		Version:        p.Version,
		TracerVersions: p.Outcome.TracerVersions,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("error marshalling policy %q: %v", p.Name, err)
		return ""
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

// getAllLatestDefaultLibraries returns the tracing libraries included in the default/all bundle.
func getAllLatestDefaultLibraries(containerRegistry string) []libInfo {
	var libsToInject []libInfo
	for _, lang := range defaultInjectedLanguages {
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

func containsVolume(pod *corev1.Pod, volumeName string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == volumeName {
			return true
		}
	}

	return false
}

// extractTracerConfigsFromAnnotations parses the tracer-configs annotation into env vars to inject
// alongside the locally injected libraries. It is the annotation-based equivalent of a target's
// ddTraceConfigs. Invalid input (malformed JSON or a non DD_ prefixed name) is logged and skipped
// rather than failing the mutation, mirroring the lenient handling of the other local SDK
// injection annotations.
func extractTracerConfigsFromAnnotations(pod *corev1.Pod) []corev1.EnvVar {
	value, found := annotation.Get(pod, annotation.TracerConfigs)
	if !found {
		return nil
	}

	var tracerConfigs []TracerConfig
	if err := json.Unmarshal([]byte(value), &tracerConfigs); err != nil {
		log.Errorf("could not parse %q annotation for Single Step Instrumentation: %v", annotation.TracerConfigs, err)
		return nil
	}

	envVars := make([]corev1.EnvVar, 0, len(tracerConfigs))
	for _, tc := range tracerConfigs {
		// Match the validation applied to config-based ddTraceConfigs: only allow DD_ prefixed names
		// so this cannot be used as a generic env var injector.
		if !strings.HasPrefix(tc.Name, "DD_") {
			log.Errorf("tracer config %q from %q annotation does not start with DD_, skipping", tc.Name, annotation.TracerConfigs)
			continue
		}
		envVars = append(envVars, tc.AsEnvVar())
	}

	return envVars
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
