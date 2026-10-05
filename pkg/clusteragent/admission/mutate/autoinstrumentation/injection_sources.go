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
	"sync"

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

// injectionSourceName identifies a source in the injection resolution chain.
type injectionSourceName string

const (
	injectionSourceAnnotation             injectionSourceName = "annotation"
	injectionSourceDatadogInstrumentation injectionSourceName = "datadog-instrumentation"
	injectionSourceRemoteConfig           injectionSourceName = "remote-config"
	injectionSourceGPU                    injectionSourceName = "gpu"
	injectionSourceStatic                 injectionSourceName = "static"
	injectionSourceInjectAll              injectionSourceName = "inject-all"
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
	sourcePass sourceAction = iota
	sourceInject
	sourceDeny
)

type sourceResult struct {
	action sourceAction
	plan   *injectionPlan
}

type injectionSource interface {
	resolve(*corev1.Pod) sourceResult
}

type injectionSourceEntry struct {
	name              injectionSourceName
	determinesSSIMode bool
	source            injectionSource
}

// injectionPlan describes the libraries and tracer configuration selected
// by an injection source.
type injectionPlan struct {
	name                      string
	libraries                 []libInfo
	tracerEnvVars             []corev1.EnvVar
	appliedMetadataJSON       string
	languageDetectionEligible bool
	// blocked is true when a remote-config policy denied injection. The plan is
	// retained so the mutator can record the blocking policy on the pod.
	blocked bool
}

// policySource keeps matcher policies aligned with injection plans by index.
type policySource struct {
	plans   []injectionPlan
	matcher *policyMatcher
}

func (s *policySource) configured() bool {
	return s != nil && len(s.plans) > 0 && s.matcher != nil
}

func (s *policySource) resolve(pod *corev1.Pod) sourceResult {
	if !s.configured() {
		return sourceResult{action: sourcePass}
	}

	idx := s.matcher.matchIndex(pod)
	if idx < 0 || idx >= len(s.plans) {
		return sourceResult{action: sourcePass}
	}

	if !s.matcher.policies[idx].Outcome.Inject {
		log.Debugf("Pod %q matched policy %q which denies injection", mutatecommon.PodString(pod), s.plans[idx].name)
		return sourceResult{action: sourceDeny, plan: &s.plans[idx]}
	}

	log.Debugf("Pod %q matched target %q", mutatecommon.PodString(pod), s.plans[idx].name)
	return sourceResult{action: sourceInject, plan: &s.plans[idx]}
}

// remotePolicySource owns the live RC policies. The mutator holds its read
// lock across resolution so matching and inject-all fallback see the same policies.
type remotePolicySource struct {
	config           *Config
	wmeta            workloadmeta.Component
	defaultLibraries []libInfo
	current          atomic.Pointer[policySource]
	mu               sync.RWMutex
}

func (s *remotePolicySource) setPolicies(ps []policies.Policy) error {
	if len(ps) == 0 {
		s.clearPolicies()
		return nil
	}

	plans, err := buildInjectionPlansFromPolicies(s.config, ps, s.defaultLibraries)
	if err != nil {
		return err
	}
	next := &policySource{
		plans:   plans,
		matcher: newPolicyMatcher(ps, s.wmeta),
	}
	s.mu.Lock()
	s.current.Store(next)
	s.mu.Unlock()
	return nil
}

func (s *remotePolicySource) clearPolicies() {
	s.mu.Lock()
	s.current.Store(nil)
	s.mu.Unlock()
}

func (s *remotePolicySource) resolve(pod *corev1.Pod) sourceResult {
	return s.current.Load().resolve(pod)
}

// injectAllSource is active only when SSI is enabled and neither RC nor
// static targeting is configured. Those activation conditions belong to this
// source rather than to the mutator's priority declaration.
type injectAllSource struct {
	enabled bool
	plan    *injectionPlan
	static  *policySource
	remote  *atomic.Pointer[policySource]
}

func (s *injectAllSource) resolve(_ *corev1.Pod) sourceResult {
	if !s.enabled || s.plan == nil || s.remote.Load() != nil || s.static.configured() {
		return sourceResult{action: sourcePass}
	}
	return sourceResult{action: sourceInject, plan: s.plan}
}

type annotationSource struct {
	containerRegistry string
	defaultLibraries  []libInfo
	mutateUnlabelled  bool
}

func (s *annotationSource) resolve(pod *corev1.Pod) sourceResult {
	enabled, exists := getEnabledLabel(pod)
	if exists && !enabled {
		return sourceResult{action: sourceDeny}
	}
	if !exists && !s.mutateUnlabelled {
		return sourceResult{action: sourcePass}
	}

	if libraries := extractLibrariesFromAnnotations(pod, s.containerRegistry); len(libraries) > 0 {
		return sourceResult{action: sourceInject, plan: &injectionPlan{
			libraries:     libraries,
			tracerEnvVars: extractTracerConfigsFromAnnotations(pod),
		}}
	}

	injectAllAnnotation := strings.ToLower(annotation.LibraryVersion.Format("all"))
	if _, found := pod.Annotations[injectAllAnnotation]; found {
		return sourceResult{action: sourceInject, plan: &injectionPlan{
			libraries:     s.defaultLibraries,
			tracerEnvVars: extractTracerConfigsFromAnnotations(pod),
		}}
	}

	return sourceResult{action: sourcePass}
}

type ddiSource struct {
	provider          DDITargetProvider
	containerRegistry string
	defaultLibraries  []libInfo
}

func (s *ddiSource) resolve(pod *corev1.Pod) sourceResult {
	if s.provider == nil {
		return sourceResult{action: sourcePass}
	}

	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return sourceResult{action: sourcePass}
	}
	rootKind, rootName := kubernetes.ResolvePodRootOwner(ref.Kind, ref.Name, pod.Labels)
	workload := ssi.DDICRTarget{Kind: rootKind, Namespace: pod.Namespace, Name: rootName}
	ddiConfig, ok := s.provider.GetTarget(workload)
	if !ok {
		return sourceResult{action: sourcePass}
	}
	if !ddiConfig.Enabled {
		return sourceResult{action: sourceDeny}
	}

	return sourceResult{action: sourceInject, plan: s.planFromDDI(workload, ddiConfig)}
}

func (s *ddiSource) planFromDDI(workload ssi.DDICRTarget, ddiConfig ssi.DDIAPMConfig) *injectionPlan {
	libraries := s.defaultLibraries
	languageDetectionEligible := true
	if len(ddiConfig.TracerVersions) > 0 {
		pinned := getPinnedLibraries(ddiConfig.TracerVersions, s.containerRegistry, true)
		libraries = pinned.libs
		languageDetectionEligible = pinned.areSetToDefaults
	}

	name := fmt.Sprintf("datadoginstrumentation:%s", ddiConfig.CR)
	payload := struct {
		Name           string            `json:"name"`
		Workload       ssi.DDICRTarget   `json:"workload"`
		TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
		TracerConfigs  []corev1.EnvVar   `json:"ddTraceConfigs,omitempty"`
	}{
		Name:           name,
		Workload:       workload,
		TracerVersions: ddiConfig.TracerVersions,
		TracerConfigs:  ddiConfig.TracerConfigs,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Warnf("error marshalling DDI target %q: %v", name, err)
	}

	return &injectionPlan{
		name:                      name,
		libraries:                 libraries,
		tracerEnvVars:             ddiConfig.TracerConfigs,
		appliedMetadataJSON:       string(data),
		languageDetectionEligible: languageDetectionEligible,
	}
}

func newStaticPolicySource(config *Config, targets []Target, defaultLibraries []libInfo, wmeta workloadmeta.Component) (*policySource, error) {
	// Configuration targets are first-wins. Reverse so the last-TRUE-wins matcher
	// preserves that order. RC is already last-wins on the wire and is not reversed.
	targets = slices.Clone(targets)
	slices.Reverse(targets)

	plans, err := buildInjectionPlans(config, targets, defaultLibraries)
	if err != nil {
		return nil, err
	}
	return &policySource{
		plans:   plans,
		matcher: newPolicyMatcher(policiesFromTargets(targets), wmeta),
	}, nil
}

func buildInjectionPlans(config *Config, targets []Target, defaultLibraries []libInfo) ([]injectionPlan, error) {
	plans := make([]injectionPlan, len(targets))
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

		var libraries []libInfo
		languageDetectionEligible := false
		if len(t.TracerVersions) == 0 {
			libraries = defaultLibraries
			languageDetectionEligible = true
		} else {
			pinnedLibraries := getPinnedLibraries(t.TracerVersions, config.containerRegistry, true)
			languageDetectionEligible = pinnedLibraries.areSetToDefaults
			libraries = pinnedLibraries.libs
		}

		tracerEnvVars := make([]corev1.EnvVar, len(t.TracerConfigs))
		for j, tc := range t.TracerConfigs {
			if !hasAllowedTracerConfigPrefix(tc.Name) {
				return nil, fmt.Errorf("tracer config %q does not start with DD_ or OTEL_", tc.Name)
			}
			tracerEnvVars[j] = tc.AsEnvVar()
		}

		plans[i] = injectionPlan{
			name:                      t.Name,
			libraries:                 libraries,
			tracerEnvVars:             tracerEnvVars,
			appliedMetadataJSON:       createTargetJSON(t),
			languageDetectionEligible: languageDetectionEligible,
		}
	}

	return plans, nil
}

func buildInjectionPlansFromPolicies(config *Config, ps []policies.Policy, defaultLibraries []libInfo) ([]injectionPlan, error) {
	plans := make([]injectionPlan, len(ps))
	for i, p := range ps {
		var libraries []libInfo
		languageDetectionEligible := false
		if len(p.Outcome.TracerVersions) == 0 {
			libraries = defaultLibraries
			languageDetectionEligible = true
		} else {
			pinnedLibraries := getPinnedLibraries(p.Outcome.TracerVersions, config.containerRegistry, true)
			languageDetectionEligible = pinnedLibraries.areSetToDefaults
			libraries = pinnedLibraries.libs
		}

		tracerEnvVars := make([]corev1.EnvVar, len(p.Outcome.TracerConfigs))
		for j, tc := range p.Outcome.TracerConfigs {
			if !hasAllowedTracerConfigPrefix(tc.Name) {
				return nil, fmt.Errorf("tracer config %q does not start with DD_ or OTEL_", tc.Name)
			}
			tracerEnvVars[j] = corev1.EnvVar{Name: tc.Name, Value: tc.Value}
		}

		plans[i] = injectionPlan{
			name:                      p.Name,
			libraries:                 libraries,
			tracerEnvVars:             tracerEnvVars,
			appliedMetadataJSON:       createPolicyJSON(p),
			languageDetectionEligible: languageDetectionEligible,
			blocked:                   !p.Outcome.Inject,
		}
	}

	return plans, nil
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

func createTargetJSON(t Target) string {
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
		Blocked        bool              `json:"blocked,omitempty"`
	}{
		Name:           p.Name,
		ID:             p.ID,
		Version:        p.Version,
		TracerVersions: p.Outcome.TracerVersions,
		Blocked:        !p.Outcome.Inject,
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

	tracerEnvVars := make([]corev1.EnvVar, 0, len(tracerConfigs))
	for _, tc := range tracerConfigs {
		if !hasAllowedTracerConfigPrefix(tc.Name) {
			log.Errorf("tracer config %q from %q annotation does not start with DD_ or OTEL_, skipping", tc.Name, annotation.TracerConfigs)
			continue
		}
		tracerEnvVars = append(tracerEnvVars, tc.AsEnvVar())
	}
	return tracerEnvVars
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
