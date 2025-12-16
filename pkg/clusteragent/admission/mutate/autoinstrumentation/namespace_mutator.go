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
	"time"

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

const (
	// apmInjectionErrorAnnotationKey this annotation is added when the apm auto-instrumentation admission controller failed to mutate the Pod.
	apmInjectionErrorAnnotationKey = "apm.datadoghq.com/injection-error"
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

func (m *mutatorCore) mutatePodContainers(pod *corev1.Pod, cm containerMutator) error {
	return mutatePodContainers(pod, filteredContainerMutator(m.config.containerFilter, cm))
}

func (m *mutatorCore) injectTracers(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}
	var (
		lastError      error
		startTime      = time.Now()
		configInjector = &libConfigInjector{}
		injectionType  = config.source.injectionType()
		autoDetected   = config.source.isFromLanguageDetection()

		// Get provider based on pod annotation (if present) or use default
		provider = m.config.LibraryInjectionRegistry.GetProviderForPod(pod)
	)

	// Inject env variables used for Onboarding KPIs propagation
	if err := m.mutatePodContainers(pod, config.source.containerMutator()); err != nil {
		return err
	}

	// Inject the APM injector using the provider
	injectorConfig := m.getInjectorConfig(pod)
	injectorResult := provider.InjectInjector(pod, injectorConfig)
	switch injectorResult.Status {
	case libraryinjection.MutationStatusSkipped:
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[apmInjectionErrorAnnotationKey] = injectorResult.Message
		return nil
	case libraryinjection.MutationStatusError:
		metrics.LibInjectionErrors.Inc("injector", strconv.FormatBool(autoDetected), injectionType)
		lastError = fmt.Errorf("failed to inject injector: %s", injectorResult.Message)
		log.Errorf("Cannot inject library injector into pod %s: %s", mutatecommon.PodString(pod), injectorResult.Message)
	}

	// Add injector environment variables (LD_PRELOAD, DD_INJECT_*, debug vars)
	// These are common to all providers and are added here to avoid duplication.
	// We use the envVar type to properly handle LD_PRELOAD concatenation when it already exists.
	injectorEnvConfig := m.getInjectorEnvConfig(pod, startTime)
	injectorEnvMutators := buildInjectorEnvVarMutators(injectorEnvConfig)
	for i := range pod.Spec.Containers {
		ctr := &pod.Spec.Containers[i]
		if m.config.containerFilter != nil && !m.config.containerFilter(ctr) {
			continue
		}
		for _, mutator := range injectorEnvMutators {
			_ = mutator.mutateContainer(ctr)
		}
	}

	// Apply UST env var mutator to app containers
	ustEnvVarMutator := m.ustEnvVarMutator(pod)
	if err := m.mutatePodContainers(pod, ustEnvVarMutator); err != nil {
		return err
	}

	// Apply language detection mutator
	if err := m.mutatePodContainers(pod, config.languageDetection.containerMutator()); err != nil {
		return err
	}

	// Inject each language library using the provider
	// Use the same security context as the injector
	libSecurityContext := injectorConfig.InitSecurityContext

	for _, lib := range config.libs {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected), strconv.FormatBool(autoDetected), injectionType)
		}()

		// Resolve the image if needed
		libConfig := m.getLibraryConfig(lib, libSecurityContext)
		if m.imageResolver != nil {
			if resolvedImage, ok := m.imageResolver.Resolve(lib.registry, lib.repository, lib.tag); ok {
				libConfig.Image = resolvedImage.FullImageRef
			}
		}

		libResult := provider.InjectLibrary(pod, libConfig)
		switch libResult.Status {
		case libraryinjection.MutationStatusError:
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			lastError = fmt.Errorf("language %s: %s", lib.lang, libResult.Message)
			continue
		case libraryinjection.MutationStatusSkipped:
			continue
		}

		// Apply lib config mutator
		if err := configInjector.podMutator(lib.lang).mutatePod(pod); err != nil {
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

// newInjector creates an injector instance for this pod.
func (m *mutatorCore) newInjector(pod *corev1.Pod, startTime time.Time, lopts libRequirementOptions) *injector {
	opts := []injectorOption{
		injectorWithLibRequirementOptions(lopts),
		injectorWithImageTag(m.config.Instrumentation.InjectorImageTag, m.imageResolver),
	}

	for _, e := range []annotationExtractor[injectorOption]{
		injectorVersionAnnotationExtractorFunc(m.imageResolver),
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

// getInjectorConfig builds the InjectorConfig for the library injection provider.
// It extracts image settings from pod annotations or falls back to defaults.
func (m *mutatorCore) getInjectorConfig(pod *corev1.Pod) libraryinjection.InjectorConfig {
	cfg := libraryinjection.InjectorConfig{
		Registry: m.config.containerRegistry,
	}

	// Start with the default image from config
	if m.imageResolver != nil {
		if resolvedImage, ok := m.imageResolver.Resolve(m.config.containerRegistry, "apm-inject", m.config.Instrumentation.InjectorImageTag); ok {
			cfg.Image = resolvedImage.FullImageRef
		} else {
			cfg.Image = fmt.Sprintf("%s/apm-inject:%s", m.config.containerRegistry, m.config.Instrumentation.InjectorImageTag)
		}
	} else {
		cfg.Image = fmt.Sprintf("%s/apm-inject:%s", m.config.containerRegistry, m.config.Instrumentation.InjectorImageTag)
	}

	// Check for custom image annotation
	if customImage, ok := pod.Annotations["admission.datadoghq.com/apm-inject.custom-image"]; ok && customImage != "" {
		cfg.Image = customImage
	}

	// Check for version annotation (overrides default tag but not custom image)
	if _, hasCustomImage := pod.Annotations["admission.datadoghq.com/apm-inject.custom-image"]; !hasCustomImage {
		if version, ok := pod.Annotations["admission.datadoghq.com/apm-inject.version"]; ok && version != "" {
			if m.imageResolver != nil {
				if resolvedImage, ok := m.imageResolver.Resolve(m.config.containerRegistry, "apm-inject", version); ok {
					cfg.Image = resolvedImage.FullImageRef
				} else {
					cfg.Image = fmt.Sprintf("%s/apm-inject:%s", m.config.containerRegistry, version)
				}
			} else {
				cfg.Image = fmt.Sprintf("%s/apm-inject:%s", m.config.containerRegistry, version)
			}
		}
	}

	// Compute security context for the namespace
	cfg.InitSecurityContext = m.getInitSecurityContextForNamespace(pod.Namespace)

	return cfg
}

// getInitSecurityContextForNamespace returns the appropriate security context for init containers
// based on the namespace's security settings.
func (m *mutatorCore) getInitSecurityContextForNamespace(nsName string) *corev1.SecurityContext {
	// If a global security context is configured, use it
	if m.config.initSecurityContext != nil {
		return m.config.initSecurityContext
	}

	// Check if the namespace requires a restricted security context
	nsLabels, err := getNamespaceLabels(m.wmeta, nsName)
	if err != nil {
		log.Warnf("error getting labels for namespace=%s: %s", nsName, err)
		return nil
	}

	if val, ok := nsLabels["pod-security.kubernetes.io/enforce"]; ok && val == "restricted" {
		// https://datadoghq.atlassian.net/browse/INPLAT-492
		return defaultRestrictedSecurityContext
	}

	return nil
}

// getInjectorEnvConfig builds the InjectorEnvConfig for generating injector environment variables.
// It extracts debug settings from pod annotations.
func (m *mutatorCore) getInjectorEnvConfig(pod *corev1.Pod, startTime time.Time) libraryinjection.InjectorEnvConfig {
	cfg := libraryinjection.InjectorEnvConfig{
		InjectTime: startTime,
		Debug:      false,
	}

	// Check for debug annotation
	if debugStr, ok := pod.Annotations["admission.datadoghq.com/apm-inject.debug"]; ok {
		if debug, err := strconv.ParseBool(debugStr); err == nil {
			cfg.Debug = debug
		}
	}

	return cfg
}

// buildInjectorEnvVarMutators creates container mutators for injector environment variables.
// This uses the envVar type to properly handle LD_PRELOAD concatenation when it already exists.
func buildInjectorEnvVarMutators(cfg libraryinjection.InjectorEnvConfig) []containerMutator {
	ldPreloadPath := libraryinjection.AsAbsPath(libraryinjection.InjectorFilePath("launcher.preload.so"))

	mutators := []containerMutator{
		// LD_PRELOAD should be concatenated with ":" if it already exists
		envVar{
			key:     "LD_PRELOAD",
			valFunc: joinValFunc(ldPreloadPath, ":"),
		},
		envVar{
			key:     "DD_INJECT_SENDER_TYPE",
			valFunc: identityValFunc("k8s"),
		},
		envVar{
			key:     "DD_INJECT_START_TIME",
			valFunc: identityValFunc(strconv.FormatInt(cfg.InjectTime.Unix(), 10)),
		},
	}

	// Add debug env vars if enabled
	if cfg.Debug {
		mutators = append(mutators,
			envVar{key: "DD_APM_INSTRUMENTATION_DEBUG", valFunc: trueValFunc()},
			envVar{key: "DD_TRACE_STARTUP_LOGS", valFunc: trueValFunc()},
			envVar{key: "DD_TRACE_DEBUG", valFunc: trueValFunc()},
		)
	}

	return mutators
}

// getLibraryConfig builds the LibraryConfig for a specific library.
func (m *mutatorCore) getLibraryConfig(lib libInfo, securityContext *corev1.SecurityContext) libraryinjection.LibraryConfig {
	return libraryinjection.LibraryConfig{
		Language:            string(lib.lang),
		Image:               lib.image,
		Registry:            lib.registry,
		Repository:          lib.repository,
		Tag:                 lib.tag,
		ContainerName:       lib.ctrName,
		InitSecurityContext: securityContext,
	}
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
