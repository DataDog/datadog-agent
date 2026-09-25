// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/ssi"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

// targetSourceName identifies a source in the target resolution chain.
type targetSourceName string

const (
	targetSourceAnnotation             targetSourceName = "annotation"
	targetSourceDatadogInstrumentation targetSourceName = "datadog-instrumentation"
	targetSourceRemoteConfig           targetSourceName = "remote-config"
	targetSourceStatic                 targetSourceName = "static"
	targetSourceInjectAll              targetSourceName = "inject-all"
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

// sourceAction distinguishes a source that has no opinion from an explicit
// injection or denial. A denial is decisive and blocks lower-priority sources.
type sourceAction uint8

const (
	sourceAbstain sourceAction = iota
	sourceInject
	sourceDeny
)

type sourceResult struct {
	action sourceAction
	target *targetInternal
}

type targetSource interface {
	resolve(*corev1.Pod) sourceResult
}

type sourceEntry struct {
	name             targetSourceName
	determineSSIMode bool
	source           targetSource
}

// targetInternal is the injection configuration a matched target resolves to.
// It carries no selector: matching is delegated to the policy engine for
// static configuration targets and remote-config policies. DDI configuration
// is resolved directly from its workload target.
type targetInternal struct {
	name            string
	libVersions     []libInfo
	envVars         []corev1.EnvVar
	json            string
	usesDefaultLibs bool
	// fromPolicy is true when this internal target was derived from a
	// remote-config policy rather than a configuration target. It selects which
	// annotation/env var carries the applied information.
	fromPolicy bool
}

// policySet keeps matcher policies aligned with injection targets by index.
type policySet struct {
	targets []targetInternal
	matcher *policyMatcher
}

type policyTargetSource struct {
	policies policySet
}

func (s *policyTargetSource) configured() bool {
	return len(s.policies.targets) > 0 && s.policies.matcher != nil
}

func (s *policyTargetSource) resolve(pod *corev1.Pod) sourceResult {
	if !s.configured() {
		return sourceResult{action: sourceAbstain}
	}

	idx := s.policies.matcher.matchIndex(pod)
	if idx < 0 || idx >= len(s.policies.targets) {
		return sourceResult{action: sourceAbstain}
	}

	if !s.policies.matcher.policies[idx].Outcome.Inject {
		log.Debugf("Pod %q matched policy %q which denies injection", mutatecommon.PodString(pod), s.policies.targets[idx].name)
		return sourceResult{action: sourceDeny}
	}

	log.Debugf("Pod %q matched target %q", mutatecommon.PodString(pod), s.policies.targets[idx].name)
	return sourceResult{action: sourceInject, target: &s.policies.targets[idx]}
}

// remotePolicyTargetSource always occupies its priority slot. It atomically
// loads the latest RC policy source and abstains when RC is not configured.
type remotePolicyTargetSource struct {
	config             *Config
	wmeta              workloadmeta.Component
	defaultLibVersions []libInfo
	policies           atomic.Pointer[policyTargetSource]
}

func (s *remotePolicyTargetSource) setPolicies(ps []policies.Policy) error {
	if len(ps) == 0 {
		s.clearPolicies()
		return nil
	}

	targets, err := buildInternalTargetsFromPolicies(s.config, ps, s.defaultLibVersions)
	if err != nil {
		return err
	}
	s.policies.Store(&policyTargetSource{policies: policySet{
		targets: targets,
		matcher: newPolicyMatcher(ps, s.wmeta),
	}})
	return nil
}

func (s *remotePolicyTargetSource) clearPolicies() {
	s.policies.Store(nil)
}

func (s *remotePolicyTargetSource) configured() bool {
	return s != nil && s.policies.Load() != nil
}

func (s *remotePolicyTargetSource) resolve(pod *corev1.Pod) sourceResult {
	policies := s.policies.Load()
	if policies == nil {
		return sourceResult{action: sourceAbstain}
	}
	return policies.resolve(pod)
}

// injectAllTargetSource is active only when SSI is enabled and neither RC nor
// static targeting is configured. Those activation conditions belong to this
// source rather than to the mutator's priority declaration.
type injectAllTargetSource struct {
	enabled bool
	target  *targetInternal
	remote  *remotePolicyTargetSource
	static  *policyTargetSource
}

func (s *injectAllTargetSource) resolve(_ *corev1.Pod) sourceResult {
	if !s.enabled || s.target == nil || s.remote.configured() || s.static.configured() {
		return sourceResult{action: sourceAbstain}
	}
	return sourceResult{action: sourceInject, target: s.target}
}

type annotationTargetSource struct {
	containerRegistry  string
	defaultLibVersions []libInfo
	mutateUnlabelled   bool
}

func (s *annotationTargetSource) resolve(pod *corev1.Pod) sourceResult {
	enabled, exists := getEnabledLabel(pod)
	if exists && !enabled {
		return sourceResult{action: sourceDeny}
	}
	if !exists && !s.mutateUnlabelled {
		return sourceResult{action: sourceAbstain}
	}

	if libraries := extractLibrariesFromAnnotations(pod, s.containerRegistry); len(libraries) > 0 {
		return sourceResult{action: sourceInject, target: &targetInternal{
			libVersions: libraries,
			envVars:     extractTracerConfigsFromAnnotations(pod),
		}}
	}

	injectAllAnnotation := strings.ToLower(annotation.LibraryVersion.Format("all"))
	if _, found := pod.Annotations[injectAllAnnotation]; found {
		return sourceResult{action: sourceInject, target: &targetInternal{
			libVersions: s.defaultLibVersions,
			envVars:     extractTracerConfigsFromAnnotations(pod),
		}}
	}

	return sourceResult{action: sourceAbstain}
}

type ddiTargetSource struct {
	provider           DDITargetProvider
	containerRegistry  string
	defaultLibVersions []libInfo
}

func (s *ddiTargetSource) resolve(pod *corev1.Pod) sourceResult {
	if s.provider == nil {
		return sourceResult{action: sourceAbstain}
	}

	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return sourceResult{action: sourceAbstain}
	}
	rootKind, rootName := kubernetes.ResolvePodRootOwner(ref.Kind, ref.Name, pod.Labels)
	workload := ssi.DDICRTarget{Kind: rootKind, Namespace: pod.Namespace, Name: rootName}
	config, ok := s.provider.GetTarget(workload)
	if !ok {
		return sourceResult{action: sourceAbstain}
	}
	if !config.Enabled {
		return sourceResult{action: sourceDeny}
	}

	return sourceResult{action: sourceInject, target: s.targetFromConfig(workload, config)}
}

func (s *ddiTargetSource) targetFromConfig(workload ssi.DDICRTarget, config ssi.DDIAPMConfig) *targetInternal {
	libVersions := s.defaultLibVersions
	usesDefaultLibs := true
	if len(config.TracerVersions) > 0 {
		pinned := getPinnedLibraries(config.TracerVersions, s.containerRegistry, true)
		libVersions = pinned.libs
		usesDefaultLibs = pinned.areSetToDefaults
	}

	name := fmt.Sprintf("datadoginstrumentation:%s", config.CR)
	payload := struct {
		Name           string            `json:"name"`
		Workload       ssi.DDICRTarget   `json:"workload"`
		TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
		TracerConfigs  []corev1.EnvVar   `json:"ddTraceConfigs,omitempty"`
	}{
		Name:           name,
		Workload:       workload,
		TracerVersions: config.TracerVersions,
		TracerConfigs:  config.TracerConfigs,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Warnf("error marshalling DDI target %q: %v", name, err)
	}

	return &targetInternal{
		name:            name,
		libVersions:     libVersions,
		envVars:         config.TracerConfigs,
		json:            string(data),
		usesDefaultLibs: usesDefaultLibs,
	}
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

func buildInternalTargets(config *Config, targets []Target, defaultLibVersions []libInfo) ([]targetInternal, error) {
	internalTargets := make([]targetInternal, len(targets))
	for i, t := range targets {
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

func createDefaultTarget(namespaces []string, pinnedLibVersions map[string]string) Target {
	target := Target{Name: "default"}
	if len(pinnedLibVersions) > 0 {
		target.TracerVersions = pinnedLibVersions
	}
	if len(namespaces) > 0 {
		target.NamespaceSelector = &NamespaceSelector{MatchNames: namespaces}
	}
	return target
}

func createJSON(t Target) string {
	data, err := json.Marshal(t)
	if err != nil {
		log.Errorf("error marshalling target %q: %v", t.Name, err)
		return fmt.Sprintf("error marshalling target %q: %v", t.Name, err)
	}
	return string(data)
}

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

func getEnabledLabel(pod *corev1.Pod) (bool, bool) {
	val, found := pod.GetLabels()[common.EnabledLabelKey]
	if !found {
		return false, false
	}
	return val == "true", true
}

// getAllLatestDefaultLibraries returns the tracing libraries included in the default/all bundle.
func getAllLatestDefaultLibraries(containerRegistry string) []libInfo {
	var libsToInject []libInfo
	for _, lang := range defaultInjectedLanguages {
		libsToInject = append(libsToInject, lang.defaultLibInfo(containerRegistry, ""))
	}
	return libsToInject
}

// extractTracerConfigsFromAnnotations parses the tracer-configs annotation into env vars to inject
// alongside the locally injected libraries. It is the annotation-based equivalent of a target's
// ddTraceConfigs. Invalid input (malformed JSON or a name without an allowed prefix) is logged and skipped.
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
		if !hasAllowedTracerConfigPrefix(tc.Name) {
			log.Errorf("tracer config %q from %q annotation does not start with DD_ or OTEL_, skipping", tc.Name, annotation.TracerConfigs)
			continue
		}
		envVars = append(envVars, tc.AsEnvVar())
	}
	return envVars
}

func extractLibrariesFromAnnotations(pod *corev1.Pod, registry string) []libInfo {
	var libs []libInfo
	for _, supportedLang := range ssi.SupportedLanguages {
		lang := language(supportedLang)
		if customImage, found := annotation.Get(pod, annotation.LibraryImage.Format(string(lang))); found {
			libs = append(libs, lang.libInfo("", customImage))
		}
		if libVersion, found := annotation.Get(pod, annotation.LibraryVersion.Format(string(lang))); found {
			libs = append(libs, lang.libInfoWithResolver("", registry, libVersion))
		}

		for _, container := range pod.Spec.Containers {
			if customImage, found := annotation.Get(pod, annotation.LibraryContainerImage.Format(container.Name, string(lang))); found {
				libs = append(libs, lang.libInfo(container.Name, customImage))
			}
			if libVersion, found := annotation.Get(pod, annotation.LibraryContainerVersion.Format(container.Name, string(lang))); found {
				libs = append(libs, lang.libInfoWithResolver(container.Name, registry, libVersion))
			}
		}
	}
	return libs
}
