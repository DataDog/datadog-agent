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
	"k8s.io/client-go/dynamic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/ssi"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const (
	// AppliedTargetEnvVar is the environment variable that contains the JSON of the target that was applied to the pod.
	AppliedTargetEnvVar = "DD_INSTRUMENTATION_APPLIED_TARGET"
	// AppliedPolicyEnvVar is the environment variable that contains the compact JSON of the policy that was applied to the pod.
	AppliedPolicyEnvVar = "DD_INSTRUMENTATION_APPLIED_POLICY"
)

// injectionResolution is the complete resolution consumed by TargetMutator. The
// selected configuration and SSI mode can originate from different sources.
type injectionResolution struct {
	plan       *injectionPlan
	isSSI      bool
	selectedBy injectionSourceName
}

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	core                          *mutatorCore
	securityClientLibraryMutator  containerMutator
	profilingClientLibraryMutator containerMutator
	disabledNamespaces            map[string]struct{}
	sources                       []injectionSourceEntry
	remoteSource                  *remotePolicySource
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups. When on-demand instrumentation is enabled and rcClient is non-nil, the
// mutator also subscribes to remote-config SSI policies, which override matching static targets.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver, csiDriverWatcher libraryinjection.CSIDriverWatcher, rcClient RemoteConfigClient, ddiTargets DDITargetProvider) (*TargetMutator, error) {
	defaultLibraries := getAllLatestDefaultLibraries(config.containerRegistry)
	disabledNamespaces := make(map[string]struct{}, len(config.Instrumentation.DisabledNamespaces))
	for _, namespace := range config.Instrumentation.DisabledNamespaces {
		disabledNamespaces[namespace] = struct{}{}
	}

	var targets []Target
	if config.Instrumentation.Enabled {
		targets = config.Instrumentation.Targets
		if len(targets) == 0 && len(config.Instrumentation.EnabledNamespaces) > 0 {
			targets = append(targets, createDefaultTarget(config.Instrumentation.EnabledNamespaces, config.Instrumentation.LibVersions))
		}
	}
	staticSource, err := newStaticPolicySource(config, targets, defaultLibraries, wmeta)
	if err != nil {
		return nil, err
	}
	fallback, err := buildInjectionPlans(config, []Target{createDefaultTarget(nil, config.Instrumentation.LibVersions)}, defaultLibraries)
	if err != nil {
		return nil, err
	}
	var gpuTargets []Target
	if config.gpuTarget != nil {
		gpuTargets = append(gpuTargets, *config.gpuTarget)
	}
	gpuSource, err := newStaticPolicySource(config, gpuTargets, defaultLibraries, wmeta)
	if err != nil {
		return nil, err
	}

	annotationSource := &annotationSource{
		containerRegistry: config.containerRegistry,
		defaultLibraries:  defaultLibraries,
		mutateUnlabelled:  config.mutateUnlabelled,
	}
	ddiSource := &ddiSource{
		provider:          ddiTargets,
		containerRegistry: config.containerRegistry,
		defaultLibraries:  defaultLibraries,
	}
	remoteSource := &remotePolicySource{
		config:           config,
		wmeta:            wmeta,
		defaultLibraries: defaultLibraries,
	}
	injectAllSource := &injectAllSource{
		enabled: config.Instrumentation.Enabled,
		plan:    &fallback[0],
		static:  staticSource,
		remote:  &remoteSource.current,
	}

	m := &TargetMutator{
		core:                          newMutatorCore(config, wmeta, imageResolver, csiDriverWatcher),
		securityClientLibraryMutator:  config.securityClientLibraryMutator,
		profilingClientLibraryMutator: config.profilingClientLibraryMutator,
		disabledNamespaces:            disabledNamespaces,
		remoteSource:                  remoteSource,
		// This is the single declaration of source precedence. Sources remain
		// in the list and decide for themselves to pass or inject.
		sources: []injectionSourceEntry{
			{name: injectionSourceAnnotation, source: annotationSource},
			{name: injectionSourceDatadogInstrumentation, determinesSSIMode: true, source: ddiSource},
			{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: remoteSource},
			{name: injectionSourceGPU, determinesSSIMode: true, source: gpuSource},
			{name: injectionSourceStatic, determinesSSIMode: true, source: staticSource},
			{name: injectionSourceInjectAll, determinesSSIMode: true, source: injectAllSource},
		},
	}

	// On-demand instrumentation is the local gate for remote-config SSI
	// policies. subscribeRemoteConfig is a no-op when rcClient is nil (e.g. in
	// tests or when remote config is disabled).
	if config.Instrumentation.OnDemand {
		m.subscribeRemoteConfig(rcClient)
	}

	return m, nil
}

// SetRemotePolicies updates the policies owned by the permanent RC source.
func (m *TargetMutator) SetRemotePolicies(ps []policies.Policy) error {
	return m.remoteSource.setPolicies(ps)
}

// ClearRemotePolicies drops remote-config policies from the RC source.
func (m *TargetMutator) ClearRemotePolicies() {
	m.remoteSource.clearPolicies()
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
	for _, supportedLang := range ssi.SupportedLanguages {
		lang := language(supportedLang)
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

	resolved := m.resolveTarget(pod)
	if resolved == nil {
		return false, nil
	}
	injection := resolved.plan
	if injection.blocked {
		annotation.Set(pod, annotation.AppliedPolicy, injection.appliedMetadataJSON)
		annotation.Set(pod, annotation.InjectionStatus, annotation.InjectionStatusBlocked)
		return false, nil
	}
	extracted := m.core.initExtractedLibInfo(pod, resolved.isSSI).withLibs(injection.libraries)

	// Language detection is an SSI-only fallback when the selected configuration
	// uses the default libraries.
	if resolved.isSSI && injection.languageDetectionEligible {
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
	for _, envVar := range injection.tracerEnvVars {
		_ = m.core.mutatePodContainers(pod, envVarMutator(envVar), true)
	}

	// Inject the libraries.
	err := m.core.injectTracers(pod, extracted)
	if err != nil {
		return false, fmt.Errorf("error injecting libraries: %w", err)
	}

	// Annotation-based injection has no applied target/policy metadata.
	if injection.appliedMetadataJSON != "" {
		m.addAppliedMetadata(pod, resolved)
	}

	return true, nil
}

func (m *TargetMutator) addAppliedMetadata(pod *corev1.Pod, resolved *injectionResolution) {
	injection := resolved.plan
	envVarName := AppliedTargetEnvVar
	annotationKey := annotation.AppliedTarget
	if resolved.selectedBy == injectionSourceRemoteConfig {
		envVarName = AppliedPolicyEnvVar
		annotationKey = annotation.AppliedPolicy
	}

	_ = m.core.mutatePodContainers(pod, envVarMutator(corev1.EnvVar{
		Name:  envVarName,
		Value: injection.appliedMetadataJSON,
	}), true)
	annotation.Set(pod, annotationKey, injection.appliedMetadataJSON)
}

// ShouldMutatePod determines if a pod would be mutated by the target mutator. It is used by other webhook mutators as
// a filter.
func (m *TargetMutator) ShouldMutatePod(pod *corev1.Pod) bool {
	return m.getTarget(pod) != nil
}

// getTarget returns an injectable resolution. A remote-config denial is not
// injectable, but resolveTarget still exposes it so MutatePod can record the
// blocking policy on the pod.
func (m *TargetMutator) getTarget(pod *corev1.Pod) *injectionResolution {
	resolved := m.resolveTarget(pod)
	if resolved != nil && resolved.plan.blocked {
		return nil
	}
	return resolved
}

// resolveTarget resolves both the injection plan and whether the pod is in SSI
// mode. The first decisive source selects the plan. The first decisive SSI
// source independently selects the mode so annotations can override libraries
// without hiding an underlying SSI match.
func (m *TargetMutator) resolveTarget(pod *corev1.Pod) *injectionResolution {
	if _, disabled := m.disabledNamespaces[pod.Namespace]; disabled {
		return nil
	}

	if m.remoteSource != nil {
		m.remoteSource.mu.RLock()
		defer m.remoteSource.mu.RUnlock()
	}

	var selected sourceResult
	var selectedBy injectionSourceName
	selectionDecided := false
	isSSI := false

	for _, entry := range m.sources {
		result := entry.source.resolve(pod)
		// An abstaining source has no matching plan, so move on to the next.
		if result.action == sourcePass {
			continue
		}

		// The first decisive source selects the plan; a deny blocks fallback.
		if !selectionDecided {
			selected = result
			selectedBy = entry.name
			selectionDecided = true
			if result.action == sourceDeny {
				break
			}
		}

		// An annotation can select libraries without deciding SSI mode, so the
		// first decisive SSI source may supply only the mode.
		if entry.determinesSSIMode {
			isSSI = result.action == sourceInject
			break
		}

		// Only annotation selection needs evaluation to continue for SSI mode.
		if selectedBy != injectionSourceAnnotation {
			break
		}
	}

	if !selectionDecided || selected.plan == nil {
		return nil
	}
	return &injectionResolution{
		plan:       selected.plan,
		isSSI:      isSSI,
		selectedBy: selectedBy,
	}
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
