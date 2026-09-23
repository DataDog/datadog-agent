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
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
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

// resolvedTarget is the complete resolution consumed by TargetMutator. The
// selected target and SSI mode can originate from different sources: an
// annotation selects libraries, while a matching SSI source still determines
// whether SSI side effects apply.
type resolvedTarget struct {
	target          *targetInternal
	isSSI           bool
	selectionSource targetSourceName
	ssiSource       targetSourceName
}

// TargetMutator is an autoinstrumentation mutator that filters pods based on the target based workload selection.
type TargetMutator struct {
	core                          *mutatorCore
	securityClientLibraryMutator  containerMutator
	profilingClientLibraryMutator containerMutator
	disabledNamespaces            map[string]struct{}
	sources                       []sourceEntry
	annotationSource              *annotationTargetSource
	ddiSource                     *ddiTargetSource
	remoteSource                  *remotePolicyTargetSource
	staticSource                  *policyTargetSource
	injectAllSource               *injectAllTargetSource
}

// NewTargetMutator creates a new mutator for target based workload selection. We convert the targets to a more
// efficient internal format for quick lookups. When on-demand instrumentation is enabled and rcClient is non-nil, the
// mutator also subscribes to remote-config SSI policies, which override matching static targets.
func NewTargetMutator(config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver, csiDriverWatcher libraryinjection.CSIDriverWatcher, rcClient *rcclient.Client, ddiTargets DDITargetProvider) (*TargetMutator, error) {
	defaultLibVersions := getAllLatestDefaultLibraries(config.containerRegistry)
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
	staticPolicies, err := newPolicySet(config, targets, defaultLibVersions, wmeta)
	if err != nil {
		return nil, err
	}
	fallback, err := buildInternalTargets(config, []Target{createDefaultTarget(nil, config.Instrumentation.LibVersions)}, defaultLibVersions)
	if err != nil {
		return nil, err
	}

	annotationSource := &annotationTargetSource{
		containerRegistry:  config.containerRegistry,
		defaultLibVersions: defaultLibVersions,
		mutateUnlabelled:   config.mutateUnlabelled,
	}
	ddiSource := &ddiTargetSource{
		provider:           ddiTargets,
		containerRegistry:  config.containerRegistry,
		defaultLibVersions: defaultLibVersions,
	}
	remoteSource := &remotePolicyTargetSource{
		config:             config,
		wmeta:              wmeta,
		defaultLibVersions: defaultLibVersions,
	}
	staticSource := &policyTargetSource{policies: staticPolicies}
	injectAllSource := &injectAllTargetSource{
		enabled: config.Instrumentation.Enabled,
		target:  &fallback[0],
		remote:  remoteSource,
		static:  staticSource,
	}

	m := &TargetMutator{
		core:                          newMutatorCore(config, wmeta, imageResolver, csiDriverWatcher),
		securityClientLibraryMutator:  config.securityClientLibraryMutator,
		profilingClientLibraryMutator: config.profilingClientLibraryMutator,
		disabledNamespaces:            disabledNamespaces,
		annotationSource:              annotationSource,
		ddiSource:                     ddiSource,
		remoteSource:                  remoteSource,
		staticSource:                  staticSource,
		injectAllSource:               injectAllSource,
		// This is the single declaration of source precedence. Sources remain
		// in the chain when unconfigured and decide for themselves to abstain.
		sources: []sourceEntry{
			{name: targetSourceAnnotation, source: annotationSource},
			{name: targetSourceDatadogInstrumentation, determineSSIMode: true, source: ddiSource},
			{name: targetSourceRemoteConfig, determineSSIMode: true, source: remoteSource},
			{name: targetSourceStatic, determineSSIMode: true, source: staticSource},
			{name: targetSourceInjectAll, determineSSIMode: true, source: injectAllSource},
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

	resolved := m.getTarget(pod)
	if resolved == nil {
		return false, nil
	}
	target := resolved.target
	extracted := m.core.initExtractedLibInfo(pod, resolved.isSSI).withLibs(target.libVersions)

	// Language detection is an SSI-only fallback when the selected target did
	// not pin library versions (annotation short-circuit sets usesDefaultLibs
	// false, so this path stays for true SSI matches with default libs).
	if resolved.isSSI && target.usesDefaultLibs {
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
	// A remote-config policy match carries its information on a dedicated env
	// var / annotation, distinct from configuration targets.
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
	return m.getTarget(pod) != nil
}

// getTarget resolves both the injection target and whether the pod is in SSI
// mode. The first decisive source selects the target. The first decisive SSI
// source independently selects the mode so annotations can override libraries
// without hiding an underlying SSI match.
func (m *TargetMutator) getTarget(pod *corev1.Pod) *resolvedTarget {
	if _, disabled := m.disabledNamespaces[pod.Namespace]; disabled {
		return nil
	}

	var selected sourceResult
	var selectionSource targetSourceName
	selectionDecided := false
	isSSI := false
	var ssiSource targetSourceName

	for _, entry := range m.sources {
		result := entry.source.resolve(pod)
		if result.action == sourceAbstain {
			continue
		}

		if !selectionDecided {
			selected = result
			selectionSource = entry.name
			selectionDecided = true
			if result.action == sourceDeny {
				return nil
			}
		}

		if entry.determineSSIMode {
			isSSI = result.action == sourceInject
			ssiSource = entry.name
			break
		}

		if selectionSource != targetSourceAnnotation {
			break
		}
	}

	if !selectionDecided || selected.action != sourceInject {
		return nil
	}
	return &resolvedTarget{
		target:          selected.target,
		isSSI:           isSSI,
		selectionSource: selectionSource,
		ssiSource:       ssiSource,
	}
}

// getSSITarget returns the target selected by SSI sources only. It is used by
// behavioral matching tests; production mutation uses getTarget.
func (m *TargetMutator) getSSITarget(pod *corev1.Pod) *targetInternal {
	if _, disabled := m.disabledNamespaces[pod.Namespace]; disabled {
		return nil
	}
	for _, entry := range m.sources {
		if !entry.determineSSIMode {
			continue
		}
		result := entry.source.resolve(pod)
		switch result.action {
		case sourceInject:
			return result.target
		case sourceDeny:
			return nil
		}
	}
	return nil
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
