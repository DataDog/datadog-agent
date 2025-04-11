// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NamespaceMutator is an autoinstrumentation mutator that mutates pods based on the enabled namespaces.
type NamespaceMutator struct {
	config          *Config
	filter          mutatecommon.MutationFilter
	wmeta           workloadmeta.Component
	pinnedLibraries pinnedLibraries
	core            *mutatorCore
}

// NewNamespaceMutator creates a new injector interface for the auto-instrumentation injector.
func NewNamespaceMutator(config *Config, wmeta workloadmeta.Component) (*NamespaceMutator, error) {
	filter, err := NewFilter(config)
	if err != nil {
		return nil, err
	}

	pinnedLibraries := getPinnedLibraries(config.Instrumentation.LibVersions, config.containerRegistry, true)
	return &NamespaceMutator{
		config:          config,
		filter:          filter,
		wmeta:           wmeta,
		pinnedLibraries: pinnedLibraries,
		core:            newMutatorCore(config, wmeta, filter),
	}, nil
}

// MutatePod implements the common.Mutator interface for the auto-instrumentation injector. It injects all of the
// required tracer libraries into the pod template.
func (m *NamespaceMutator) MutatePod(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	log.Debugf("Mutating pod in namespace mutator %q", mutatecommon.PodString(pod))

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}
	if pod.Namespace == "" {
		pod.Namespace = ns
	}

	if !m.isPodEligible(pod) {
		return false, nil
	}

	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			// The admission can be re-run for the same pod
			// Fast return if we injected the library already
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), mutatecommon.PodString(pod))
			return false, nil
		}
	}

	extractedLibInfo := m.extractLibInfo(pod)
	if len(extractedLibInfo.libs) == 0 {
		return false, nil
	}

	for _, mutator := range m.config.securityClientLibraryPodMutators {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for security client: %w", err)
		}
	}

	for _, mutator := range m.config.profilingClientLibraryPodMutators {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for profiling client: %w", err)
		}
	}

	if err := m.core.injectTracers(pod, extractedLibInfo); err != nil {
		log.Errorf("failed to inject auto instrumentation configurations: %v", err)
		return false, errors.New(metrics.ConfigInjectionError)
	}

	return true, nil
}

// ShouldMutatePod implements the common.MutationFilter interface for the auto-instrumentation injector.
func (m *NamespaceMutator) ShouldMutatePod(pod *corev1.Pod) bool {
	if !m.config.Instrumentation.Enabled {
		return false
	}
	return m.filter.ShouldMutatePod(pod)
}

// IsNamespaceEligible implements the common.MutationFilter interface for the auto-instrumentation injector.
func (m *NamespaceMutator) IsNamespaceEligible(ns string) bool {
	if !m.config.Instrumentation.Enabled {
		return false
	}
	return m.filter.IsNamespaceEligible(ns)
}

type mutatorCore struct {
	config *Config
	wmeta  workloadmeta.Component
	filter mutatecommon.MutationFilter
}

func newMutatorCore(config *Config, wmeta workloadmeta.Component, filter mutatecommon.MutationFilter) *mutatorCore {
	return &mutatorCore{
		config: config,
		wmeta:  wmeta,
		filter: filter,
	}
}

func (m *mutatorCore) injectTracers(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}

	requirements, injectionDecision := initContainerResourceRequirements(pod, m.config.defaultResourceRequirements)
	if injectionDecision.skipInjection {
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[apmInjectionErrorAnnotationKey] = injectionDecision.message
		return nil
	}

	var (
		lastError      error
		startTime      = time.Now()
		configInjector = &libConfigInjector{}
		injectionType  = config.source.injectionType()
		autoDetected   = config.source.isFromLanguageDetection()

		initContainerMutators = m.newInitContainerMutators(requirements)
		injectorOptions       = libRequirementOptions{
			initContainerMutators: initContainerMutators,
		}

		injector          = m.newInjector(pod, startTime, injectorOptions)
		containerMutators = containerMutators{
			config.languageDetection.containerMutator(m.config.version),
		}
	)

	// Inject env variables used for Onboarding KPIs propagation...
	// if Single Step Instrumentation is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_single_step
	// if local library injection is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_lib_injection
	if err := config.source.mutatePod(pod); err != nil {
		return err
	}

	if err := injector.podMutator(m.config.version).mutatePod(pod); err != nil {
		// setting the language tag to `injector` because this injection is not related to a specific supported language
		metrics.LibInjectionErrors.Inc("injector", strconv.FormatBool(autoDetected), injectionType)
		lastError = err
		log.Errorf("Cannot inject library injector into pod %s: %s", mutatecommon.PodString(pod), err)
	}

	for _, lib := range config.libs {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected), strconv.FormatBool(autoDetected), injectionType)
		}()

		if err := lib.podMutator(m.config.version, libRequirementOptions{
			containerMutators:     containerMutators,
			initContainerMutators: initContainerMutators,
			podMutators:           []podMutator{configInjector.podMutator(lib.lang)},
		}).mutatePod(pod); err != nil {
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			lastError = err
			continue
		}

		injected = true
	}

	if err := configInjector.podMutator(language("all")).mutatePod(pod); err != nil {
		metrics.LibInjectionErrors.Inc("all", strconv.FormatBool(autoDetected), injectionType)
		lastError = err
		log.Errorf("Cannot inject library configuration into pod %s: %s", mutatecommon.PodString(pod), err)
	}

	if m.filter.IsNamespaceEligible(pod.Namespace) {
		_ = basicLibConfigInjector{}.mutatePod(pod)
	}

	return lastError
}

// newInitContainerMutators constructs container mutators for behavior
// that is common and passed to the init containers we create.
//
// At this point in time it is: resource requirements and security contexts.
func (m *mutatorCore) newInitContainerMutators(requirements corev1.ResourceRequirements) containerMutators {
	return containerMutators{
		containerResourceRequirements{requirements},
		containerSecurityContext{m.config.initSecurityContext},
	}
}

// newInjector creates an injector instance for this pod.
func (m *mutatorCore) newInjector(pod *corev1.Pod, startTime time.Time, lopts libRequirementOptions) *injector {
	opts := []injectorOption{
		injectorWithLibRequirementOptions(lopts),
		injectorWithImageTag(m.config.Instrumentation.InjectorImageTag),
	}

	for _, e := range []annotationExtractor[injectorOption]{
		injectorVersionAnnotationExtractor,
		injectorImageAnnotationExtractor,
		injectorDebugAnnotationExtractor,
	} {
		opt, err := e.extract(pod)
		if err != nil {
			if !isErrAnnotationNotFound(err) {
				log.Warnf("error extracting injector annotation %s in single step", e.key)
			}
			continue
		}
		opts = append(opts, opt)
	}

	return newInjector(startTime, m.config.containerRegistry, opts...)
}

// isPodEligible checks whether we are allowed to inject in this pod.
func (m *NamespaceMutator) isPodEligible(pod *corev1.Pod) bool {
	return m.filter.ShouldMutatePod(pod)
}

// extractLibInfo metadata about what library information we should be
// injecting into the pod and where it came from.
func (m *NamespaceMutator) extractLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	extracted := m.core.initExtractedLibInfo(pod)

	libs := extractLibrariesFromAnnotations(pod, m.config.containerRegistry)
	if len(libs) > 0 {
		return extracted.withLibs(libs)
	}

	// if the user has pinned libraries for their configuration,
	// we prefer to use these and not override their behavior.
	//
	// N.B. this is empty if auto-instrumentation is disabled.
	if !m.pinnedLibraries.areSetToDefaults && len(m.pinnedLibraries.libs) > 0 {
		return extracted.withLibs(m.pinnedLibraries.libs)
	}

	// if the language_detection injection is enabled
	// (and we have things to filter to) we use that!
	if e, usingLanguageDetection := extracted.useLanguageDetectionLibs(); usingLanguageDetection {
		return e
	}

	if len(m.pinnedLibraries.libs) > 0 {
		return extracted.withLibs(m.pinnedLibraries.libs)
	}

	if extracted.source.isSingleStep() {
		return extracted.withLibs(getAllLatestDefaultLibraries(m.config.containerRegistry))
	}

	// Get libraries to inject for Remote Instrumentation
	// Inject all if "admission.datadoghq.com/all-lib.version" exists
	// without any other language-specific annotations.
	// This annotation is typically expected to be set via remote-config
	// for batch instrumentation without language detection.
	injectAllAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyFormat, "all"))
	if version, found := pod.Annotations[injectAllAnnotation]; found {
		if version != "latest" {
			log.Warnf("Ignoring version %q. To inject all libs, the only supported version is latest for now", version)
		}

		return extracted.withLibs(getAllLatestDefaultLibraries(m.config.containerRegistry))
	}

	return extractedPodLibInfo{}
}

func extractLibrariesFromAnnotations(pod *corev1.Pod, containerRegistry string) []libInfo {
	var (
		libList        []libInfo
		extractLibInfo = func(e annotationExtractor[libInfo]) {
			i, err := e.extract(pod)
			if err != nil {
				if !isErrAnnotationNotFound(err) {
					log.Warnf("error extracting annotation for key %s", e.key)
				}
			} else {
				libList = append(libList, i)
			}
		}
	)

	for _, l := range supportedLanguages {
		extractLibInfo(l.customLibAnnotationExtractor())
		extractLibInfo(l.libVersionAnnotationExtractor(containerRegistry))
		for _, ctr := range pod.Spec.Containers {
			extractLibInfo(l.ctrCustomLibAnnotationExtractor(ctr.Name))
			extractLibInfo(l.ctrLibVersionAnnotationExtractor(ctr.Name, containerRegistry))
		}
	}

	return libList
}

func (m *mutatorCore) initExtractedLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	// it's possible to get here without single step being enabled, and the pod having
	// annotations on it to opt it into pod mutation, we disambiguate those two cases.
	var (
		source            = libInfoSourceLibInjection
		languageDetection *libInfoLanguageDetection
	)

	if m.filter.IsNamespaceEligible(pod.Namespace) {
		source = libInfoSourceSingleStepInstrumentation
		languageDetection = m.getLibrariesLanguageDetection(pod)
	}

	return extractedPodLibInfo{
		source:            source,
		languageDetection: languageDetection,
	}
}

// getLibrariesLanguageDetection returns the languages that were detected by process language detection.
//
// The languages information is available in workloadmeta-store
// and attached on the pod's owner.
func (m *mutatorCore) getLibrariesLanguageDetection(pod *corev1.Pod) *libInfoLanguageDetection {
	if !m.config.LanguageDetection.Enabled ||
		!m.config.LanguageDetection.ReportingEnabled {
		return nil
	}

	return &libInfoLanguageDetection{
		libs:             m.getAutoDetectedLibraries(pod),
		injectionEnabled: m.config.LanguageDetection.InjectDetected,
	}
}

// getAutoDetectedLibraries constructs the libraries to be injected if the languages
// were stored in workloadmeta store based on owner annotations
// (for example: Deployment, DaemonSet, StatefulSet).
func (m *mutatorCore) getAutoDetectedLibraries(pod *corev1.Pod) []libInfo {
	ownerName, ownerKind, found := getOwnerNameAndKind(pod)
	if !found {
		return nil
	}

	store := m.wmeta
	if store == nil {
		return nil
	}

	// Currently we only support deployments
	switch ownerKind {
	case "Deployment":
		return getLibListFromDeploymentAnnotations(store, ownerName, pod.Namespace, m.config.containerRegistry)
	default:
		log.Debugf("This ownerKind:%s is not yet supported by the process language auto-detection feature", ownerKind)
		return nil
	}
}

// The config for the security products has three states: <unset> | true | false.
// This is because the products themselves have treat these cases differently:
// * <unset> - product disactivated but can be activated remotely
// * true - product activated, not overridable remotely
// * false - product disactivated, not overridable remotely
func securityClientLibraryConfigMutators(datadogConfig config.Component) []podMutator {
	asmEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm.enabled")
	iastEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.iast.enabled")
	asmScaEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm_sca.enabled")

	var podMutators []podMutator
	if asmEnabled != nil {
		podMutators = append(podMutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_ENABLED", asmEnabled))
	}
	if iastEnabled != nil {
		podMutators = append(podMutators, newConfigEnvVarFromBoolMutator("DD_IAST_ENABLED", iastEnabled))
	}
	if asmScaEnabled != nil {
		podMutators = append(podMutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_SCA_ENABLED", asmScaEnabled))
	}

	return podMutators
}

// The config for profiling has four states: <unset> | "auto" | "true" | "false".
// * <unset> - profiling not activated, but can be activated remotely
// * "true" - profiling activated unconditionally, not overridable remotely
// * "false" - profiling deactivated, not overridable remotely
// * "auto" - profiling activates per-process heuristically, not overridable remotely
func profilingClientLibraryConfigMutators(datadogConfig config.Component) []podMutator {
	profilingEnabled := getOptionalStringValue(datadogConfig, "admission_controller.auto_instrumentation.profiling.enabled")

	var podMutators []podMutator
	if profilingEnabled != nil {
		podMutators = append(podMutators, newConfigEnvVarFromStringlMutator("DD_PROFILING_ENABLED", profilingEnabled))
	}

	return podMutators
}
