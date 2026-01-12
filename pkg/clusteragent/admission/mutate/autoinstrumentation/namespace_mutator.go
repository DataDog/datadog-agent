// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	libraryinjection "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/library_injection"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type mutatorCore struct {
	config        *Config
	wmeta         workloadmeta.Component
	filter        mutatecommon.MutationFilter
	imageResolver ImageResolver
}

func newMutatorCore(config *Config, wmeta workloadmeta.Component, filter mutatecommon.MutationFilter, imageResolver ImageResolver) *mutatorCore {
	return &mutatorCore{
		config:        config,
		wmeta:         wmeta,
		filter:        filter,
		imageResolver: imageResolver,
	}
}

func (m *mutatorCore) mutatePodContainers(pod *corev1.Pod, cm containerMutator, includeInitContainers bool) error {
	return mutatePodContainers(pod, filteredContainerMutator(m.config.containerFilter, cm), includeInitContainers)
}

func (m *mutatorCore) injectTracers(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}

	autoDetected := config.source.isFromLanguageDetection()
	injectionType := config.source.injectionType()

	// Apply all mutations in order
	var lastError error
	for _, mutator := range []podMutator{
		// Injects DD_INSTRUMENTATION_INSTALL_TYPE, DD_INSTRUMENTATION_INSTALL_TIME, DD_INSTRUMENTATION_INSTALL_ID
		m.kpiEnvVarsMutator(config),
		// Injects APM injector + language-specific library init containers, volumes, and env vars
		m.apmInjectionMutator(config, autoDetected, injectionType),
		// Injects DD_VERSION and DD_ENV from pod labels/annotations
		m.ustEnvVarsPodMutator(),
		// Injects language detection annotations
		m.languageDetectionMutator(config),
		// Injects library config from annotations (admission.datadoghq.com/all-lib.config.v1)
		m.libConfigFromAnnotationsMutator(config, autoDetected, injectionType),
		// Injects default library config for SSI-eligible namespaces
		m.defaultLibConfigMutator(pod.Namespace),
	} {
		if err := mutator.mutatePod(pod); err != nil {
			lastError = err
		}
	}

	return lastError
}

// kpiEnvVarsMutator returns a mutator that injects KPI-related env vars.
// (DD_INSTRUMENTATION_INSTALL_TYPE, DD_INSTRUMENTATION_INSTALL_TIME, DD_INSTRUMENTATION_INSTALL_ID)
func (m *mutatorCore) kpiEnvVarsMutator(config extractedPodLibInfo) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		return m.mutatePodContainers(pod, config.source.containerMutator(), true)
	})
}

// apmInjectionMutator returns a mutator that injects the APM injector and language-specific libraries.
func (m *mutatorCore) apmInjectionMutator(config extractedPodLibInfo, autoDetected bool, injectionType string) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		// Convert libInfo to LibraryConfig here because library_injection cannot
		// import autoinstrumentation (circular dependency).
		libs := make([]libraryinjection.LibraryConfig, len(config.libs))
		for i, lib := range config.libs {
			libs[i] = libraryinjection.LibraryConfig{
				Language:      string(lib.lang),
				ResolvedImage: m.resolveLibraryImage(lib),
				ContainerName: lib.ctrName,
			}
		}

		return libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
			DefaultResourceRequirements: m.config.defaultResourceRequirements,
			InitSecurityContext:         m.config.initSecurityContext,
			ContainerFilter:             m.config.containerFilter,
			Wmeta:                       m.wmeta,
			Debug:                       m.isDebugEnabled(pod),
			AutoDetected:                autoDetected,
			InjectionType:               injectionType,
			Injector: libraryinjection.InjectorConfig{
				ResolvedImage: m.resolveInjectorImage(pod),
			},
			Libraries: libs,
		})
	})
}

// isDebugEnabled checks if debug mode is enabled via pod annotation.
func (m *mutatorCore) isDebugEnabled(pod *corev1.Pod) bool {
	if debugEnabled, found := GetAnnotation(pod, AnnotationEnableDebug); found {
		if debug, err := strconv.ParseBool(debugEnabled); err == nil {
			return debug
		}
	}
	return false
}

// resolveInjectorImage determines the injector image to use based on configuration and pod annotations.
func (m *mutatorCore) resolveInjectorImage(pod *corev1.Pod) libraryinjection.ResolvedImage {
	// Check for the injector image being set via annotation (highest priority)
	if injectorImage, found := GetAnnotation(pod, AnnotationInjectorImage); found {
		return libraryinjection.ResolvedImage{Image: injectorImage}
	}

	// Check for the injector version set via annotation
	injectorTag := m.config.Instrumentation.InjectorImageTag
	if injectorVersion, found := GetAnnotation(pod, AnnotationInjectorVersion); found {
		injectorTag = injectorVersion
	}

	// Try to resolve via imageResolver (remote config)
	if m.imageResolver != nil {
		if resolved, ok := m.imageResolver.Resolve(m.config.containerRegistry, "apm-inject", injectorTag); ok {
			log.Debugf("Resolved injector image for %s/apm-inject:%s: %s", m.config.containerRegistry, injectorTag, resolved.FullImageRef)
			return libraryinjection.ResolvedImage{
				Image:            resolved.FullImageRef,
				CanonicalVersion: resolved.CanonicalVersion,
			}
		}
	}

	// Fall back to tag-based image
	return libraryinjection.ResolvedImage{
		Image: fmt.Sprintf("%s/apm-inject:%s", m.config.containerRegistry, injectorTag),
	}
}

// resolveLibraryImage resolves the library image using the imageResolver if available.
// Falls back to the pre-formatted image if resolution fails.
func (m *mutatorCore) resolveLibraryImage(lib libInfo) libraryinjection.ResolvedImage {
	if m.imageResolver != nil {
		if resolved, ok := m.imageResolver.Resolve(lib.registry, lib.repository, lib.tag); ok {
			log.Debugf("Resolved library image for %s/%s:%s: %s", lib.registry, lib.repository, lib.tag, resolved.FullImageRef)
			return libraryinjection.ResolvedImage{
				Image:            resolved.FullImageRef,
				CanonicalVersion: resolved.CanonicalVersion,
			}
		}
	}
	// Fall back to pre-formatted image
	return libraryinjection.ResolvedImage{Image: lib.image}
}

// libConfigFromAnnotationsMutator returns a mutator that reads library configuration
// from pod annotations (admission.datadoghq.com/<lang>-lib.config.v1) and injects
// the corresponding env vars. Reads config for each injected language + "all".
// This allows users to customize library behavior via annotations.
func (m *mutatorCore) libConfigFromAnnotationsMutator(config extractedPodLibInfo, autoDetected bool, injectionType string) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		configInjector := &libConfigInjector{}
		var lastError error

		// Inject config for each language
		for _, lib := range config.libs {
			if err := configInjector.podMutator(lib.lang).mutatePod(pod); err != nil {
				metrics.LibInjectionErrors.Inc(string(lib.lang), strconv.FormatBool(autoDetected), injectionType)
				log.Errorf("Cannot inject library configuration for %s into pod %s: %s", lib.lang, mutatecommon.PodString(pod), err)
				lastError = err
			}
		}

		// Inject config for "all" languages
		if err := configInjector.podMutator(language("all")).mutatePod(pod); err != nil {
			metrics.LibInjectionErrors.Inc("all", strconv.FormatBool(autoDetected), injectionType)
			log.Errorf("Cannot inject library configuration into pod %s: %s", mutatecommon.PodString(pod), err)
			lastError = err
		}

		return lastError
	})
}

// defaultLibConfigMutator returns a mutator that injects default library configuration
// for namespaces eligible to Single Step Instrumentation.
// Defaults: DD_TRACE_ENABLED=true, DD_LOGS_INJECTION=true,
// DD_TRACE_HEALTH_METRICS_ENABLED=true, DD_RUNTIME_METRICS_ENABLED=true.
func (m *mutatorCore) defaultLibConfigMutator(namespace string) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		if !m.filter.IsNamespaceEligible(namespace) {
			return nil
		}

		return m.mutatePodContainers(pod, basicLibConfigInjector{}.containerMutator(), true)
	})
}

// ustEnvVarsPodMutator returns a mutator that injects UST env vars (DD_VERSION, DD_ENV) to filtered containers.
func (m *mutatorCore) ustEnvVarsPodMutator() podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		return m.mutatePodContainers(pod, m.ustEnvVarMutator(pod), true)
	})
}

// languageDetectionMutator returns a mutator that applies language detection mutations to filtered containers.
func (m *mutatorCore) languageDetectionMutator(config extractedPodLibInfo) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		return m.mutatePodContainers(pod, config.languageDetection.containerMutator(), false)
	})
}

// serviceNameMutator will attempt to find a service name to
// inject into the pods containers if SSI is enabled.
//
// This is kind of gross, and would ideally not happen more than in
// one place but we made a decision to infer DD_SERVICE in the auto-instrumentation
// webhook a while ago and customers might be relying on this behavior.
//
// We have another webhook that does something really similar: tagsFromLabels and
// it this is where the responsibility should generally.
//
// The big difference between the two is that tagsFromLabels looks at the label
// metadata and we might override it and this one will look for the _name_ of the
// owner resource.
//
// The intention is to have this always run last so that we fallback to the owner
// name in cases of missing labels coming from the resource or its owner.
//
// We want to get rid of the behavior when we are triggering the fallback _and_
// it applies: https://datadoghq.atlassian.net/browse/INPLAT-458
func (m *mutatorCore) serviceNameMutator(pod *corev1.Pod) containerMutator {
	return newServiceNameMutator(pod, m.config.podMetaAsTags)
}

// ustEnvVarMutator will attempt to find a ust env var to inject into the pods containers if SSI is enabled.
//
// This is used to inject the version and env tags into the pods containers.
//
// The service tag/name is handled separately in the serviceNameMutator for legacy reasons.
func (m *mutatorCore) ustEnvVarMutator(pod *corev1.Pod) containerMutator {
	var mutators containerMutators
	if !m.filter.IsNamespaceEligible(pod.Namespace) {
		return mutators
	}

	for tag, envVarName := range map[string]string{
		tags.Version: kubernetes.VersionTagEnvVar,
		tags.Env:     kubernetes.EnvTagEnvVar,
	} {
		if mutator := ustEnvVarMutatorForPodMeta(pod, m.config.podMetaAsTags, tag, envVarName); mutator != nil {
			mutators = append(mutators, mutator)
		}
	}

	if mutator := m.serviceNameMutator(pod); mutator != nil {
		mutators = append(mutators, mutator)
	}

	return mutators
}

func extractLibrariesFromAnnotations(pod *corev1.Pod, registry string) []libInfo {
	libs := []libInfo{}

	// Check all supported languages for potential Local SDK Injection.
	for _, l := range supportedLanguages {
		// Check for a custom library image.
		customImage, found := GetAnnotation(pod, AnnotationLibraryImage.Format(string(l)))
		if found {
			libs = append(libs, l.libInfo("", customImage))
		}

		// Check for a custom library version.
		libVersion, found := GetAnnotation(pod, AnnotationLibraryVersion.Format(string(l)))
		if found {
			libs = append(libs, l.libInfoWithResolver("", registry, libVersion))
		}

		// Check all containers in the pod for container specific Local SDK Injection.
		for _, container := range pod.Spec.Containers {
			// Check for custom library image.
			customImage, found := GetAnnotation(pod, AnnotationLibraryContainerImage.Format(container.Name, string(l)))
			if found {
				libs = append(libs, l.libInfo(container.Name, customImage))
			}

			// Check for custom library version.
			libVersion, found := GetAnnotation(pod, AnnotationLibraryContainerVersion.Format(container.Name, string(l)))
			if found {
				libs = append(libs, l.libInfoWithResolver(container.Name, registry, libVersion))
			}
		}
	}

	return libs
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
func securityClientLibraryConfigMutators(datadogConfig config.Component) containerMutators {
	asmEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm.enabled")
	iastEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.iast.enabled")
	asmScaEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm_sca.enabled")

	var mutators []containerMutator
	if asmEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_ENABLED", asmEnabled))
	}

	if iastEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_IAST_ENABLED", iastEnabled))
	}

	if asmScaEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_SCA_ENABLED", asmScaEnabled))
	}

	return mutators
}

// The config for profiling has four states: <unset> | "auto" | "true" | "false".
// * <unset> - profiling not activated, but can be activated remotely
// * "true" - profiling activated unconditionally, not overridable remotely
// * "false" - profiling deactivated, not overridable remotely
// * "auto" - profiling activates per-process heuristically, not overridable remotely
func profilingClientLibraryConfigMutators(datadogConfig config.Component) containerMutators {
	profilingEnabled := getOptionalStringValue(datadogConfig, "admission_controller.auto_instrumentation.profiling.enabled")

	var mutators []containerMutator
	if profilingEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromStringMutator("DD_PROFILING_ENABLED", profilingEnabled))
	}

	return mutators
}

func getNamespaceLabels(wmeta workloadmeta.Component, name string) (map[string]string, error) {
	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", name)
	ns, err := wmeta.GetKubernetesMetadata(id)
	if err != nil {
		return nil, fmt.Errorf("error getting namespace metadata for ns=%s: %w", name, err)
	}

	return ns.EntityMeta.Labels, nil
}

type libInfoLanguageDetection struct {
	libs             []libInfo
	injectionEnabled bool
}

func (l *libInfoLanguageDetection) containerMutator() containerMutator {
	return containerMutatorFunc(func(c *corev1.Container) error {
		if l == nil {
			return nil
		}

		var langs []string
		for _, lib := range l.libs {
			if lib.ctrName == c.Name { // strict container name matching
				langs = append(langs, string(lib.lang))
			}
		}

		// N.B.
		// We report on the languages detected regardless
		// of if it is empty or not to disambiguate the empty state
		// language_detection reporting being disabled.
		if err := (containerMutators{
			envVar{
				key:     "DD_INSTRUMENTATION_LANGUAGES_DETECTED",
				valFunc: identityValFunc(strings.Join(langs, ",")),
			},
			envVar{
				key:     "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED",
				valFunc: identityValFunc(strconv.FormatBool(l.injectionEnabled)),
			},
		}).mutateContainer(c); err != nil {
			return err
		}

		return nil
	})
}

// libInfoSource describes where we got the libraries from for
// injection and is used to set up metrics/telemetry. See
// Webhook.injectAutoInstruConfig for usage.
type libInfoSource int

const (
	// libInfoSourceLibInjection is when the user sets up annotations on their pods for
	// library injection and single step is disabled.
	libInfoSourceLibInjection libInfoSource = iota
	// libInfoSourceSingleStepInstrumentation is when we are using the instrumentation config
	// to determine which libraries to inject.
	libInfoSourceSingleStepInstrumentation
	// libInfoSourceSingleStepLanguageDetection is when we use the language detection
	// annotation to determine which libs to inject.
	libInfoSourceSingleStepLangaugeDetection
)

// injectionType produces a string to distinguish between if
// we're using "single step" or "lib injection" for metrics and logging.
func (s libInfoSource) injectionType() string {
	switch s {
	case libInfoSourceSingleStepInstrumentation, libInfoSourceSingleStepLangaugeDetection:
		return singleStepInstrumentationInstallType
	case libInfoSourceLibInjection:
		return localLibraryInstrumentationInstallType
	default:
		return "unknown"
	}
}

// isFromLanguageDetection tells us whether this source comes from
// the language detection reporting and annotation.
func (s libInfoSource) isFromLanguageDetection() bool {
	return s == libInfoSourceSingleStepLangaugeDetection
}

func (s libInfoSource) instrumentationInstallTime() string {
	instrumentationInstallTime := os.Getenv(instrumentationInstallTimeEnvVarName)
	if instrumentationInstallTime == "" {
		instrumentationInstallTime = common.ClusterAgentStartTime
	}

	return instrumentationInstallTime
}

// containerMutator creates a containerMutator for
// telemetry environment variables pertaining to:
//
// - installation_time
// - install_id
// - injection_type
func (s libInfoSource) containerMutator() containerMutator {
	return containerMutators{
		// inject DD_INSTRUMENTATION_INSTALL_TIME with current Unix time
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallTimeEnvVarName,
			Value: s.instrumentationInstallTime(),
		}),
		// inject DD_INSTRUMENTATION_INSTALL_ID with UUID created during the Agent install time
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallIDEnvVarName,
			Value: os.Getenv(instrumentationInstallIDEnvVarName),
		}),
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallTypeEnvVarName,
			Value: s.injectionType(),
		}),
	}
}

type extractedPodLibInfo struct {
	// libs are the libraries we are going to attempt to inject into the given pod.
	libs []libInfo
	// languageDetection is set when we ran/used the language-detection annotation.
	languageDetection *libInfoLanguageDetection
	// source is where we got the data from, used for telemetry later.
	source libInfoSource
}

func (e extractedPodLibInfo) withLibs(l []libInfo) extractedPodLibInfo {
	e.libs = l
	return e
}

func (e extractedPodLibInfo) useLanguageDetectionLibs() (extractedPodLibInfo, bool) {
	if e.languageDetection != nil && len(e.languageDetection.libs) > 0 && e.languageDetection.injectionEnabled {
		e.libs = e.languageDetection.libs
		e.source = libInfoSourceSingleStepLangaugeDetection
		return e, true
	}

	return e, false
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}

// getOwnerNameAndKind returns the name and kind of the first owner of the pod if it exists
// if the first owner is a replicaset, it returns the name
func getOwnerNameAndKind(pod *corev1.Pod) (string, string, bool) {
	owners := pod.GetOwnerReferences()

	if len(owners) == 0 {
		return "", "", false
	}

	owner := owners[0]
	ownerName, ownerKind := owner.Name, owner.Kind

	if ownerKind == "ReplicaSet" {
		deploymentName := kubernetes.ParseDeploymentForReplicaSet(ownerName)
		if deploymentName != "" {
			ownerKind = "Deployment"
			ownerName = deploymentName
		}
	}

	return ownerName, ownerKind, true
}

func getLibListFromDeploymentAnnotations(store workloadmeta.Component, deploymentName, ns, registry string) []libInfo {
	// populate libInfoList using the languages found in workloadmeta
	id := fmt.Sprintf("%s/%s", ns, deploymentName)
	deployment, err := store.GetKubernetesDeployment(id)
	if err != nil {
		return nil
	}

	var libList []libInfo
	for container, languages := range deployment.InjectableLanguages {
		for lang := range languages {
			// There's a mismatch between language detection and auto-instrumentation.
			// The Node language is a js lib.
			if lang == "node" {
				lang = "js"
			}

			l := language(lang)
			libList = append(libList, l.defaultLibInfo(registry, container.Name))
		}
	}

	return libList
}
