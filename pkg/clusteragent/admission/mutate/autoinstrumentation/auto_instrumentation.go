// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoinstrumentation implements the webhook that injects APM libraries
// into pods
package autoinstrumentation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	volumeName = "datadog-auto-instrumentation"
	mountPath  = "/datadog-lib"

	minimumMemoryRequest float64 = 100 * 1024 * 1024 // 100 MB (recommended minimum by Alpine)

	webhookName = "lib_injection"
)

// Webhook is the auto instrumentation webhook
type Webhook struct {
	name                     string
	isEnabled                bool
	endpoint                 string
	resources                []string
	operations               []admissionregistrationv1.OperationType
	initSecurityContext      *corev1.SecurityContext
	initResourceRequirements initResourceRequirementConfiguration
	containerRegistry        string
	injectorImageTag         string
	injectionFilter          mutatecommon.InjectionFilter
	pinnedLibraries          []libInfo
	version                  version
	wmeta                    workloadmeta.Component
}

// NewWebhook returns a new Webhook dependent on the injection filter.
func NewWebhook(wmeta workloadmeta.Component, filter mutatecommon.InjectionFilter) (*Webhook, error) {
	// Note: the webhook is not functional with the filter being disabled--
	//       and the filter is _global_! so we need to make sure that it was
	//       initialized as it validates the configuration itself.
	if filter.NSFilter == nil {
		return nil, errors.New("filter required for auto_instrumentation webhook")
	} else if err := filter.NSFilter.Err(); err != nil {
		return nil, err
	}

	initSecurityContext, err := parseInitSecurityContext()
	if err != nil {
		return nil, err
	}

	initResource, err := getInitResourceConfiguration()
	if err != nil {
		return nil, err
	}

	v, err := instrumentationVersion(pkgconfigsetup.Datadog().GetString("apm_config.instrumentation.version"))
	if err != nil {
		return nil, fmt.Errorf("invalid version for key apm_config.instrumentation.version: %w", err)
	}

	var (
		isEnabled         = pkgconfigsetup.Datadog().GetBool("admission_controller.auto_instrumentation.enabled")
		containerRegistry = mutatecommon.ContainerRegistry("admission_controller.auto_instrumentation.container_registry")
		pinnedLibraries   []libInfo
	)

	if isEnabled {
		pinnedLibraries = getPinnedLibraries(containerRegistry)
	}

	return &Webhook{
		name:                     webhookName,
		isEnabled:                isEnabled,
		endpoint:                 pkgconfigsetup.Datadog().GetString("admission_controller.auto_instrumentation.endpoint"),
		resources:                []string{"pods"},
		operations:               []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		initSecurityContext:      initSecurityContext,
		initResourceRequirements: initResource,
		injectionFilter:          filter,
		containerRegistry:        containerRegistry,
		injectorImageTag:         pkgconfigsetup.Datadog().GetString("apm_config.instrumentation.injector_image_tag"),
		pinnedLibraries:          pinnedLibraries,
		version:                  v,
		wmeta:                    wmeta,
	}, nil
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *Webhook) WebhookType() common.WebhookType {
	return common.MutatingWebhook
}

// IsEnabled returns whether the webhook is enabled
func (w *Webhook) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *Webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *Webhook) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Raw, request.Namespace, w.Name(), w.inject, request.DynamicClient))
	}
}

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}

// isPodEligible checks whether we are allowed to inject in this pod.
func (w *Webhook) isPodEligible(pod *corev1.Pod) bool {
	return w.injectionFilter.ShouldMutatePod(pod)
}

// isEnabledInNamespace checks whether this namespace is opted into or out of
// single step (auto_instrumentation) outside pod-specific annotations.
func (w *Webhook) isEnabledInNamespace(namespace string) bool {
	return w.injectionFilter.NSFilter.IsNamespaceEligible(namespace)
}

func (w *Webhook) inject(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}
	if pod.Namespace == "" {
		pod.Namespace = ns
	}
	injectApmTelemetryConfig(pod)

	if !w.isPodEligible(pod) {
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

	extractedLibInfo := w.extractLibInfo(pod)
	if len(extractedLibInfo.libs) == 0 {
		return false, nil
	}

	for _, mutator := range securityClientLibraryConfigMutators() {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for security client: %w", err)
		}
	}

	for _, mutator := range profilingClientLibraryConfigMutators() {
		if err := mutator.mutatePod(pod); err != nil {
			return false, fmt.Errorf("error mutating pod for profiling client: %w", err)
		}
	}

	if err := w.injectAutoInstruConfig(pod, extractedLibInfo); err != nil {
		log.Errorf("failed to inject auto instrumentation configurations: %v", err)
		return false, errors.New(metrics.ConfigInjectionError)
	}

	return true, nil
}

// The config for the security products has three states: <unset> | true | false.
// This is because the products themselves have treat these cases differently:
// * <unset> - product disactivated but can be activated remotely
// * true - product activated, not overridable remotely
// * false - product disactivated, not overridable remotely
func securityClientLibraryConfigMutators() []podMutator {
	boolVal := func(key string) string {
		return strconv.FormatBool(pkgconfigsetup.Datadog().GetBool(key))
	}
	return []podMutator{
		configKeyEnvVarMutator{
			envKey:    "DD_APPSEC_ENABLED",
			configKey: "admission_controller.auto_instrumentation.asm.enabled",
			getVal:    boolVal,
		},
		configKeyEnvVarMutator{
			envKey:    "DD_IAST_ENABLED",
			configKey: "admission_controller.auto_instrumentation.iast.enabled",
			getVal:    boolVal,
		},
		configKeyEnvVarMutator{
			envKey:    "DD_APPSEC_SCA_ENABLED",
			configKey: "admission_controller.auto_instrumentation.asm_sca.enabled",
			getVal:    boolVal,
		},
	}
}

// The config for profiling has four states: <unset> | "auto" | "true" | "false".
// * <unset> - profiling not activated, but can be activated remotely
// * "true" - profiling activated unconditionally, not overridable remotely
// * "false" - profiling deactivated, not overridable remotely
// * "auto" - profiling activates per-process heuristically, not overridable remotely
func profilingClientLibraryConfigMutators() []podMutator {
	return []podMutator{
		configKeyEnvVarMutator{
			envKey:    "DD_PROFILING_ENABLED",
			configKey: "admission_controller.auto_instrumentation.profiling.enabled",
			getVal:    pkgconfigsetup.Datadog().GetString,
		},
	}
}

func injectApmTelemetryConfig(pod *corev1.Pod) {
	// inject DD_INSTRUMENTATION_INSTALL_TIME with current Unix time
	instrumentationInstallTime := os.Getenv(instrumentationInstallTimeEnvVarName)
	if instrumentationInstallTime == "" {
		instrumentationInstallTime = common.ClusterAgentStartTime
	}
	instrumentationInstallTimeEnvVar := corev1.EnvVar{
		Name:  instrumentationInstallTimeEnvVarName,
		Value: instrumentationInstallTime,
	}
	_ = mutatecommon.InjectEnv(pod, instrumentationInstallTimeEnvVar)

	// inject DD_INSTRUMENTATION_INSTALL_ID with UUID created during the Agent install time
	instrumentationInstallIDEnvVar := corev1.EnvVar{
		Name:  instrumentationInstallIDEnvVarName,
		Value: os.Getenv(instrumentationInstallIDEnvVarName),
	}
	_ = mutatecommon.InjectEnv(pod, instrumentationInstallIDEnvVar)
}

// getPinnedLibraries returns tracing libraries to inject as configured by apm_config.instrumentation.lib_versions
// given a registry.
func getPinnedLibraries(registry string) []libInfo {
	// If APM Instrumentation is enabled and configuration apm_config.instrumentation.lib_versions specified,
	// inject only the libraries from the configuration
	singleStepLibraryVersions := pkgconfigsetup.Datadog().
		GetStringMapString("apm_config.instrumentation.lib_versions")

	var res []libInfo
	for lang, version := range singleStepLibraryVersions {
		l := language(lang)
		if !l.isSupported() {
			log.Warnf("APM Instrumentation detected configuration for unsupported language: %s. Tracing library for %s will not be injected", lang, lang)
			continue
		}

		log.Infof("Library version %s is specified for language %s", version, lang)
		res = append(res, l.libInfo("", l.libImageName(registry, version)))
	}

	return res
}

type libInfoLanguageDetection struct {
	libs             []libInfo
	injectionEnabled bool
}

func (l *libInfoLanguageDetection) containerMutator(v version) containerMutator {
	return containerMutatorFunc(func(c *corev1.Container) error {
		if !v.usesInjector() || l == nil {
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

// getLibrariesLanguageDetection returns the languages that were detected by process language detection.
//
// The languages information is available in workloadmeta-store
// and attached on the pod's owner.
func (w *Webhook) getLibrariesLanguageDetection(pod *corev1.Pod) *libInfoLanguageDetection {
	if !pkgconfigsetup.Datadog().GetBool("language_detection.enabled") ||
		!pkgconfigsetup.Datadog().GetBool("language_detection.reporting.enabled") {
		return nil
	}

	return &libInfoLanguageDetection{
		libs:             w.getAutoDetectedLibraries(pod),
		injectionEnabled: pkgconfigsetup.Datadog().GetBool("admission_controller.auto_instrumentation.inject_auto_detected_libraries"),
	}
}

// getAllLatestLibraries returns all supported by APM Instrumentation tracing libraries
func (w *Webhook) getAllLatestLibraries() []libInfo {
	var libsToInject []libInfo
	for _, lang := range supportedLanguages {
		libsToInject = append(libsToInject, lang.defaultLibInfo(w.containerRegistry, ""))
	}

	return libsToInject
}

// libInfoSource describes where we got the libraries from for
// injection and is used to set up metrics/telemetry. See
// Webhook.injectAutoInstruConfig for usage.
type libInfoSource int

const (
	// libInfoSourceNone is no source provided.
	libInfoSourceNone libInfoSource = iota
	// libInfoSourceLibInjection is when the user sets up annotations on their pods for
	// library injection and single step is disabled.
	libInfoSourceLibInjection
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

func (s libInfoSource) isSingleStep() bool {
	return s.injectionType() == singleStepInstrumentationInstallType
}

// isFromLanguageDetection tells us whether this source comes from
// the language detection reporting and annotation.
func (s libInfoSource) isFromLanguageDetection() bool {
	return s == libInfoSourceSingleStepLangaugeDetection
}

func (s libInfoSource) mutatePod(pod *corev1.Pod) error {
	_ = mutatecommon.InjectEnv(pod, corev1.EnvVar{
		Name:  instrumentationInstallTypeEnvVarName,
		Value: s.injectionType(),
	})
	return nil
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

func (w *Webhook) initExtractedLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	// it's possible to get here without single step being enabled, and the pod having
	// annotations on it to opt it into pod mutation, we disambiguate those two cases.
	var (
		source            = libInfoSourceLibInjection
		languageDetection *libInfoLanguageDetection
	)

	if w.isEnabledInNamespace(pod.Namespace) {
		source = libInfoSourceSingleStepInstrumentation
		languageDetection = w.getLibrariesLanguageDetection(pod)
	}

	return extractedPodLibInfo{
		source:            source,
		languageDetection: languageDetection,
	}
}

// extractLibInfo metadata about what library information we should be
// injecting into the pod and where it came from.
func (w *Webhook) extractLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	extracted := w.initExtractedLibInfo(pod)

	libs := w.extractLibrariesFromAnnotations(pod)
	if len(libs) > 0 {
		return extracted.withLibs(libs)
	}

	// if the user has pinned libraries for their configuration,
	// we prefer to use these and not override their behavior.
	//
	// N.B. this is empty if auto-instrumentation is disabled.
	if len(w.pinnedLibraries) > 0 {
		return extracted.withLibs(w.pinnedLibraries)
	}

	// if the language_detection injection is enabled
	// (and we have things to filter to) we use that!
	if e, usingLanguageDetection := extracted.useLanguageDetectionLibs(); usingLanguageDetection {
		return e
	}

	if extracted.source.isSingleStep() {
		return extracted.withLibs(w.getAllLatestLibraries())
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

		return extracted.withLibs(w.getAllLatestLibraries())
	}

	return extractedPodLibInfo{}
}

// getAutoDetectedLibraries constructs the libraries to be injected if the languages
// were stored in workloadmeta store based on owner annotations
// (for example: Deployment, DaemonSet, StatefulSet).
func (w *Webhook) getAutoDetectedLibraries(pod *corev1.Pod) []libInfo {
	ownerName, ownerKind, found := getOwnerNameAndKind(pod)
	if !found {
		return nil
	}

	store := w.wmeta
	if store == nil {
		return nil
	}

	// Currently we only support deployments
	switch ownerKind {
	case "Deployment":
		return getLibListFromDeploymentAnnotations(store, ownerName, pod.Namespace, w.containerRegistry)
	default:
		log.Debugf("This ownerKind:%s is not yet supported by the process language auto-detection feature", ownerKind)
		return nil
	}
}

func (w *Webhook) extractLibrariesFromAnnotations(pod *corev1.Pod) []libInfo {
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
		extractLibInfo(l.libVersionAnnotationExtractor(w.containerRegistry))
		for _, ctr := range pod.Spec.Containers {
			extractLibInfo(l.ctrCustomLibAnnotationExtractor(ctr.Name))
			extractLibInfo(l.ctrLibVersionAnnotationExtractor(ctr.Name, w.containerRegistry))
		}
	}

	return libList
}

func (w *Webhook) initContainerMutators(requirements corev1.ResourceRequirements) containerMutators {
	return containerMutators{
		containerResourceRequirements{requirements},
		containerSecurityContext{w.initSecurityContext},
	}
}

func (w *Webhook) newInjector(startTime time.Time, pod *corev1.Pod, opts ...injectorOption) podMutator {
	for _, e := range []annotationExtractor[injectorOption]{
		injectorVersionAnnotationExtractor,
		injectorImageAnnotationExtractor,
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

	return newInjector(startTime, w.containerRegistry, w.injectorImageTag, opts...).
		podMutator(w.version)
}

// podSumRessourceRequirements computes the sum of cpu/memory necessary for the whole pod.
// This is computed as max(max(initContainer resources), sum(container resources)) for both limit and request
func podSumRessourceRequirements(pod *corev1.Pod) corev1.ResourceRequirements {
	ressourceRequirement := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	keys := []corev1.ResourceName{corev1.ResourceMemory, corev1.ResourceCPU}
	for _, k := range keys {
		// Take max(initContainer ressource)
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if limit, ok := c.Resources.Limits[k]; ok {
				existing := ressourceRequirement.Limits[k]
				if limit.Cmp(existing) == 1 {
					ressourceRequirement.Limits[k] = limit
				}
			}
			if request, ok := c.Resources.Requests[k]; ok {
				existing := ressourceRequirement.Requests[k]
				if request.Cmp(existing) == 1 {
					ressourceRequirement.Requests[k] = request
				}
			}
		}

		limitSum := resource.Quantity{}
		reqSum := resource.Quantity{}
		for i := range pod.Spec.Containers {
			c := &pod.Spec.Containers[i]
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}
		// Take max(sum of container resources)
		existingLimit := ressourceRequirement.Limits[k]
		if limitSum.Cmp(existingLimit) == 1 {
			ressourceRequirement.Limits[k] = limitSum
		}

		existingReq := ressourceRequirement.Requests[k]
		if reqSum.Cmp(existingReq) == 1 {
			ressourceRequirement.Requests[k] = reqSum
		}
	}

	return ressourceRequirement
}

// initContainerResourceRequirements computes init container cpu/memory requests and limits.
// There are two cases:
//
//  1. If a resource quantity was set in config, we use it
//
//  2. If no quantity was set, we try to use as much of the resource as we can without impacting
//     pod scheduling.
//     Init containers are run one after another. This means that any single init container can use
//     the maximum amount of the resource requested by the original pod wihtout changing how much of
//     this resource is necessary.
//     In particular, for the QoS Guaranteed Limits and Requests have to be equal for every container.
//     which means that the max amount of request/limits that we compute is going to be equal to each other
//     so our init container will also have request == limit.
//
//     In the 2nd case, of we wouldn't have enough memory, we bail on injection
func initContainerResourceRequirements(pod *corev1.Pod, conf initResourceRequirementConfiguration) (requirements corev1.ResourceRequirements, skipInjection bool) {
	requirements = corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}
	podRequirements := podSumRessourceRequirements(pod)

	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		if q, ok := conf[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
		} else {
			if maxPodLim, ok := podRequirements.Limits[k]; ok {
				if k == corev1.ResourceMemory {
					// If the pod before adding instrumentation init containers would have had a memory limit smaller than
					// a certain amount, we just don't do anything, for two reasons:
					// 1. The init containers need quite a lot of memory in order to not OOM
					// 2. The APM libraries themselves will increase memory footprint of the container by a
					//   non trivial amount, and we don't want to crash memory constrained apps
					if maxPodLim.AsApproximateFloat64() <= minimumMemoryRequest {
						return corev1.ResourceRequirements{}, true
					}
				}
				requirements.Limits[k] = maxPodLim
			}
			if maxPodReq, ok := podRequirements.Requests[k]; ok {
				requirements.Requests[k] = maxPodReq
			}
		}
	}
	return requirements, false
}

func (w *Webhook) injectAutoInstruConfig(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}
	requirements, skipInjection := initContainerResourceRequirements(pod, w.initResourceRequirements)
	if skipInjection {
		return nil
	}

	var (
		lastError      error
		configInjector = &libConfigInjector{}
		injectionType  = config.source.injectionType()
		autoDetected   = config.source.isFromLanguageDetection()

		initContainerMutators = w.initContainerMutators(requirements)
		injector              = w.newInjector(time.Now(), pod, injectorWithLibRequirementOptions(libRequirementOptions{
			initContainerMutators: initContainerMutators,
		}))
		containerMutators = containerMutators{
			config.languageDetection.containerMutator(w.version),
		}
	)

	// Inject env variables used for Onboarding KPIs propagation...
	// if Single Step Instrumentation is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_single_step
	// if local library injection is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_lib_injection
	if err := config.source.mutatePod(pod); err != nil {
		return err
	}

	for _, lib := range config.libs {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected), strconv.FormatBool(autoDetected), injectionType)
		}()

		if err := lib.podMutator(w.version, libRequirementOptions{
			containerMutators:     containerMutators,
			initContainerMutators: initContainerMutators,
			podMutators:           []podMutator{configInjector.podMutator(lib.lang), injector},
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

	if w.isEnabledInNamespace(pod.Namespace) {
		_ = basicLibConfigInjector{}.mutatePod(pod)
	}

	return lastError
}

type initResourceRequirementConfiguration map[corev1.ResourceName]resource.Quantity

func getInitResourceConfiguration() (initResourceRequirementConfiguration, error) {
	conf := initResourceRequirementConfiguration{}

	if pkgconfigsetup.Datadog().IsSet("admission_controller.auto_instrumentation.init_resources.cpu") {
		quantity, err := resource.ParseQuantity(pkgconfigsetup.Datadog().GetString("admission_controller.auto_instrumentation.init_resources.cpu"))
		if err != nil {
			return conf, err
		}
		conf[corev1.ResourceCPU] = quantity
	}

	if pkgconfigsetup.Datadog().IsSet("admission_controller.auto_instrumentation.init_resources.memory") {
		quantity, err := resource.ParseQuantity(pkgconfigsetup.Datadog().GetString("admission_controller.auto_instrumentation.init_resources.memory"))
		if err != nil {
			return conf, err
		}
		conf[corev1.ResourceMemory] = quantity
	}

	return conf, nil
}

func parseInitSecurityContext() (*corev1.SecurityContext, error) {
	securityContext := corev1.SecurityContext{}
	confKey := "admission_controller.auto_instrumentation.init_security_context"

	if pkgconfigsetup.Datadog().IsSet(confKey) {
		confValue := pkgconfigsetup.Datadog().GetString(confKey)
		err := json.Unmarshal([]byte(confValue), &securityContext)
		if err != nil {
			return nil, fmt.Errorf("failed to get init security context from configuration, %s=`%s`: %v", confKey, confValue, err)
		}
	}

	return &securityContext, nil
}

// Returns the name of Kubernetes resource that owns the pod
func getServiceNameFromPod(pod *corev1.Pod) (string, error) {
	ownerReferences := pod.ObjectMeta.OwnerReferences
	if len(ownerReferences) != 1 {
		return "", fmt.Errorf("pod should be owned by one resource; current owners: %v+", ownerReferences)
	}

	switch owner := ownerReferences[0]; owner.Kind {
	case "StatefulSet":
		fallthrough
	case "Job":
		fallthrough
	case "CronJob":
		fallthrough
	case "DaemonSet":
		return owner.Name, nil
	case "ReplicaSet":
		return kubernetes.ParseDeploymentForReplicaSet(owner.Name), nil
	}

	return "", nil
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}
