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
	"slices"
	"strings"

	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const (
	// AppliedTargetEnvVar is the environment variable that contains the JSON of the target that was applied to the pod.
	AppliedTargetEnvVar = "DD_INSTRUMENTATION_APPLIED_TARGET"
	// AppliedPolicyEnvVar is the environment variable that contains the compact JSON of the policy that was applied to the pod.
	AppliedPolicyEnvVar = "DD_INSTRUMENTATION_APPLIED_POLICY"
)

// allowedTracerConfigPrefixes are the env var name prefixes accepted for tracer configs supplied
// via Targets, remote-config policies, or the tracer-configs annotation. This keeps the mechanism
// from being used as a generic env var injector while still allowing DD_* and OTel-native OTEL_*
// configuration (e.g. activating a tracer's OTel mode with OTEL_TRACES_EXPORTER).
var allowedTracerConfigPrefixes = []string{"DD_", "OTEL_"}

func hasAllowedTracerConfigPrefix(name string) bool {
	for _, prefix := range allowedTracerConfigPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// policySet is matcher.policies aligned with injection targets by index, so a
// match resolves directly to its injection config. An empty set (no targets, no
// policies) matches nothing.
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
	ssiEnabled                    bool

	// staticPolicies is local targeting: explicit targets, or enabledNamespaces
	// as a namespace target (Helm, Operator, or datadog.yaml). Empty when SSI
	// is off or when SSI is on with no targeting.
	staticPolicies policySet
	// injectAll is the SSI-on fallback when there is no static targeting and no RC.
	injectAll *targetInternal
	// remotePolicies is the current RC policy set. Nil when none are installed.
	remotePolicies atomic.Pointer[policySet]
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups. When on-demand instrumentation is enabled and rcClient is non-nil, the
// mutator also subscribes to remote-config SSI policies, which are evaluated after static targets.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver, csiDriverWatcher libraryinjection.CSIDriverWatcher, rcClient *rcclient.Client) (*TargetMutator, error) {
	// Create a map of user-configured disabled namespaces for quick lookups.
	// Default namespaces (kube-system, datadog agent namespace) are excluded at
	// the webhook layer via namespace selectors and not duplicated here.
	disabledNamespacesMap := make(map[string]bool, len(config.Instrumentation.DisabledNamespaces))
	for _, ns := range config.Instrumentation.DisabledNamespaces {
		disabledNamespacesMap[ns] = true
	}

	// Fetch the default lib versions to use if there are no user defined versions.
	defaultLibVersions := getAllLatestDefaultLibraries(config.containerRegistry)

	ssiEnabled := config.Instrumentation.Enabled
	var targets []Target
	if ssiEnabled {
		targets = config.Instrumentation.Targets
		if len(targets) == 0 && len(config.Instrumentation.EnabledNamespaces) > 0 {
			targets = append(targets, createDefaultTarget(config.Instrumentation.EnabledNamespaces, config.Instrumentation.LibVersions))
		}
	}

	staticPolicies, err := newPolicySet(config, targets, defaultLibVersions, wmeta)
	if err != nil {
		return nil, err
	}

	m := &TargetMutator{
		disabledNamespaces:            disabledNamespacesMap,
		securityClientLibraryMutator:  config.securityClientLibraryMutator,
		profilingClientLibraryMutator: config.profilingClientLibraryMutator,
		containerRegistry:             config.containerRegistry,
		mutateUnlabelled:              config.mutateUnlabelled,
		defaultLibVersions:            defaultLibVersions,
		ssiEnabled:                    ssiEnabled,
		staticPolicies:                staticPolicies,
	}
	// SSI on and no static targeting: prepare inject-all. Applied only when RC is also absent.
	if ssiEnabled && len(targets) == 0 {
		fallback, err := buildInternalTargets(config, []Target{createDefaultTarget(nil, config.Instrumentation.LibVersions)}, defaultLibVersions)
		if err != nil {
			return nil, err
		}
		m.injectAll = &fallback[0]
	}

	core := newMutatorCore(config, wmeta, imageResolver, csiDriverWatcher)
	m.core = core

	// On-demand instrumentation is the local gate for remote-config SSI
	// policies. subscribeRemoteConfig is a no-op when rcClient is nil (e.g. in
	// tests or when remote config is disabled).
	if config.Instrumentation.OnDemand {
		m.subscribeRemoteConfig(rcClient)
	}

	return m, nil
}

func newPolicySet(config *Config, targets []Target, defaultLibVersions []libInfo, wmeta workloadmeta.Component) (policySet, error) {
	// Configuration targets are first-wins. Reverse so the last-TRUE-wins matcher
	// preserves that order. RC is already last-wins on the wire and is not reversed.
	targets = slices.Clone(targets)
	slices.Reverse(targets)

	internalTargets, err := buildInternalTargets(config, targets, defaultLibVersions)
	if err != nil {
		return policySet{}, err
	}
	return policySet{
		targets: internalTargets,
		matcher: newPolicyMatcher(policiesFromTargets(targets), wmeta),
	}, nil
}

// buildInternalTargets converts configuration targets into the internal format used for injection. Matching is not
// part of it: the selectors are lowered into policies by policiesFromTargets and evaluated by the policy engine.
func buildInternalTargets(config *Config, targets []Target, defaultLibVersions []libInfo) ([]targetInternal, error) {
	internalTargets := make([]targetInternal, len(targets))
	for i, t := range targets {
		// The selectors are converted to k8s label selectors for validation only, so that an unsupported selector
		// is still rejected at startup rather than silently abstaining at evaluation time.
		if t.PodSelector != nil {
			if _, err := t.PodSelector.AsLabelSelector(); err != nil {
				return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
			}
		}
		if t.NamespaceSelector != nil {
			if _, err := t.NamespaceSelector.AsLabelSelector(); err != nil {
				return nil, fmt.Errorf("could not convert selector to label selector: %w", err)
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

		// Convert the tracer configs to env vars. We only allow DD_ and OTEL_ prefixed names to avoid
		// this from being used as a generic env var injector.
		envVars := make([]corev1.EnvVar, len(t.TracerConfigs))
		for j, tc := range t.TracerConfigs {
			if !hasAllowedTracerConfigPrefix(tc.Name) {
				return nil, fmt.Errorf("tracer config %q does not start with DD_ or OTEL_", tc.Name)
			}
			envVars[j] = tc.AsEnvVar()
		}

		internalTargets[i] = targetInternal{
			name:            t.Name,
			libVersions:     libVersions,
			envVars:         envVars,
			json:            createJSON(t),
			usesDefaultLibs: usesDefaultLibs,
		}
	}

	return internalTargets, nil
}

// SetRemotePolicies installs remote-config policies as a second last-TRUE-wins
// phase after static targets. The wire order is already last-TRUE-wins (default
// first, exceptions after) and is stored as-is.
func (m *TargetMutator) SetRemotePolicies(ps []policies.Policy) error {
	if len(ps) == 0 {
		m.ClearRemotePolicies()
		return nil
	}

	remoteTargets, err := buildInternalTargetsFromPolicies(m.core.config, ps, m.defaultLibVersions)
	if err != nil {
		return err
	}

	m.remotePolicies.Store(&policySet{
		targets: remoteTargets,
		matcher: newPolicyMatcher(ps, m.core.wmeta),
	})
	return nil
}

// ClearRemotePolicies drops remote-config policies. Matching falls back to
// static targets, then the SSI inject-all default if there is no static targeting.
func (m *TargetMutator) ClearRemotePolicies() {
	m.remotePolicies.Store(nil)
}

// buildInternalTargetsFromPolicies resolves each policy's outcome (tracer
// versions and configs) into the internal injection format, mirroring
// buildInternalTargets but sourced from policies rather than Targets.
func buildInternalTargetsFromPolicies(config *Config, ps []policies.Policy, defaultLibVersions []libInfo) ([]targetInternal, error) {
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
			if !hasAllowedTracerConfigPrefix(tc.Name) {
				return nil, fmt.Errorf("tracer config %q does not start with DD_ or OTEL_", tc.Name)
			}
			envVars[j] = corev1.EnvVar{Name: tc.Name, Value: tc.Value}
		}

		internalTargets[i] = targetInternal{
			name:            p.Name,
			libVersions:     libVersions,
			envVars:         envVars,
			json:            createPolicyJSON(p),
			usesDefaultLibs: usesDefaultLibs,
			fromPolicy:      true,
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

	// Library selection still short-circuits on annotations (unchanged GA
	// precedence). SSI mode is decided separately from whether a target/policy
	// matched the pod — not from a namespace-level eligibility approximation.
	target, ssi := m.resolveTargetAndSSI(pod)
	if target == nil {
		return false, nil
	}
	extracted := m.core.initExtractedLibInfo(pod, ssi).withLibs(target.libVersions)

	// Language detection is an SSI-only fallback when the selected target did
	// not pin library versions (annotation short-circuit sets usesDefaultLibs
	// false, so this path stays for true SSI matches with default libs).
	if ssi && target.usesDefaultLibs {
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
	// A policy-driven match (remote config) carries its information on a
	// dedicated env var / annotation, distinct from configuration targets.
	envVarName := AppliedTargetEnvVar
	annotationKey := annotation.AppliedTarget
	if target.fromPolicy {
		envVarName = AppliedPolicyEnvVar
		annotationKey = annotation.AppliedPolicy
	}

	// Inject the target json. The is added so that the injector can make use of the target information.
	_ = m.core.mutatePodContainers(pod, envVarMutator(corev1.EnvVar{
		Name:  envVarName,
		Value: target.json,
	}), true)

	// Add the annotations to the pod.
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

	// At this point, we should only mutate if a target matches or annotations apply.
	return m.getTarget(pod) != nil
}

// targetInternal is the injection configuration a matched policy resolves to.
// It carries no selector: matching is delegated to the policy engine, which is
// fed by policiesFromTargets for configuration targets and by remote config for
// policies.
type targetInternal struct {
	name            string
	libVersions     []libInfo
	envVars         []corev1.EnvVar
	json            string
	usesDefaultLibs bool
	// fromPolicy is true when this internal target was derived from a policy
	// (remote config) rather than a configuration target. It selects which
	// annotation/env var carries the applied information.
	fromPolicy bool
}

// getTarget determines which target to use for a given a pod, which includes the set of tracing libraries to inject.
// Library annotations still short-circuit matching (GA precedence unchanged in this change).
func (m *TargetMutator) getTarget(pod *corev1.Pod) *targetInternal {
	target, _ := m.resolveTargetAndSSI(pod)
	return target
}

// resolveTargetAndSSI selects what to inject and whether the pod is in SSI mode.
func (m *TargetMutator) resolveTargetAndSSI(pod *corev1.Pod) (*targetInternal, bool) {
	matched := m.getMatchingTarget(pod)
	result := m.getTargetFromAnnotation(pod)
	if !result.shouldContinue {
		return result.target, matched != nil
	}
	return matched, matched != nil
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

// getMatchingTarget: static targets first, then RC, then SSI inject-all if both
// are absent. A matched deny returns nil and does not fall through.
func (m *TargetMutator) getMatchingTarget(pod *corev1.Pod) *targetInternal {
	if _, ok := m.disabledNamespaces[pod.Namespace]; ok {
		return nil
	}

	if t, matched := applyMatch(&m.staticPolicies, pod); matched {
		return t
	}
	remotePolicies := m.remotePolicies.Load()
	if t, matched := applyMatch(remotePolicies, pod); matched {
		return t
	}
	if m.ssiEnabled && !hasTargets(&m.staticPolicies) && remotePolicies == nil {
		return m.injectAll
	}
	return nil
}

func hasTargets(set *policySet) bool {
	return set != nil && len(set.targets) > 0
}

// applyMatch returns the injection target for a policy set. matched is true
// when a policy evaluated to TRUE (even if that policy denies injection).
func applyMatch(set *policySet, pod *corev1.Pod) (*targetInternal, bool) {
	if set == nil || set.matcher == nil {
		return nil, false
	}

	idx := set.matcher.matchIndex(pod)
	if idx < 0 || idx >= len(set.targets) {
		return nil, false
	}

	if !set.matcher.policies[idx].Outcome.Inject {
		log.Debugf("Pod %q matched policy %q which denies injection", mutatecommon.PodString(pod), set.targets[idx].name)
		return nil, true
	}

	log.Debugf("Pod %q matched target %q", mutatecommon.PodString(pod), set.targets[idx].name)
	return &set.targets[idx], true
}

// createDefaultTarget translates enabledNamespaces/libVersions into a target.
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
// ddTraceConfigs. Invalid input (malformed JSON or a name without an allowed prefix) is logged and skipped
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
		// Match the validation applied to config-based ddTraceConfigs: only allow DD_ or OTEL_
		// prefixed names so this cannot be used as a generic env var injector.
		if !hasAllowedTracerConfigPrefix(tc.Name) {
			log.Errorf("tracer config %q from %q annotation does not start with DD_ or OTEL_, skipping", tc.Name, annotation.TracerConfigs)
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
