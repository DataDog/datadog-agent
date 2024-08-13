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

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	volumeName = "datadog-auto-instrumentation"
	mountPath  = "/datadog-lib"

	// defaultMilliCPURequest defines default milli cpu request number.
	defaultMilliCPURequest int64 = 50 // 0.05 core
	// defaultMemoryRequest defines default memory request size.
	defaultMemoryRequest int64 = 20 * 1024 * 1024 // 20 MB

	webhookName = "lib_injection"
)

// Webhook is the auto instrumentation webhook
type Webhook struct {
	name                     string
	isEnabled                bool
	endpoint                 string
	resources                []string
	operations               []admiv1.OperationType
	initSecurityContext      *corev1.SecurityContext
	initResourceRequirements corev1.ResourceRequirements
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

	initResourceRequirements, err := initResources()
	if err != nil {
		return nil, err
	}

	v, err := instrumentationVersion(config.Datadog().GetString("apm_config.instrumentation.version"))
	if err != nil {
		return nil, fmt.Errorf("invalid version for key apm_config.instrumentation.version: %w", err)
	}

	containerRegistry := mutatecommon.ContainerRegistry("admission_controller.auto_instrumentation.container_registry")

	return &Webhook{
		name:                     webhookName,
		isEnabled:                config.Datadog().GetBool("admission_controller.auto_instrumentation.enabled"),
		endpoint:                 config.Datadog().GetString("admission_controller.auto_instrumentation.endpoint"),
		resources:                []string{"pods"},
		operations:               []admiv1.OperationType{admiv1.Create},
		initSecurityContext:      initSecurityContext,
		initResourceRequirements: initResourceRequirements,
		injectionFilter:          filter,
		containerRegistry:        containerRegistry,
		injectorImageTag:         config.Datadog().GetString("apm_config.instrumentation.injector_image_tag"),
		pinnedLibraries:          getPinnedLibraries(containerRegistry),
		version:                  v,
		wmeta:                    wmeta,
	}, nil
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
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
func (w *Webhook) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return mutatecommon.DefaultLabelSelectors(useNamespaceSelector)
}

// MutateFunc returns the function that mutates the resources
func (w *Webhook) MutateFunc() admission.WebhookFunc {
	return w.injectAutoInstrumentation
}

// injectAutoInstrumentation injects APM libraries into pods
func (w *Webhook) injectAutoInstrumentation(request *admission.MutateRequest) ([]byte, error) {
	return mutatecommon.Mutate(request.Raw, request.Namespace, w.Name(), w.inject, request.DynamicClient)
}

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}

func (w *Webhook) isPodEligible(pod *corev1.Pod) bool {
	return w.injectionFilter.ShouldMutatePod(pod)
}

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
		return strconv.FormatBool(config.Datadog().GetBool(key))
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
			getVal:    config.Datadog().GetString,
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
	singleStepLibraryVersions := config.Datadog().
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
	libs    []libInfo
	enabled bool

	injected bool
}

func (l *libInfoLanguageDetection) podMutator(v version) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		if l == nil || len(l.libs) == 0 || !v.usesInjector() || l.injected {
			return nil
		}

		var libs []string
		for _, lib := range l.libs {
			libs = append(libs, string(lib.lang))
		}

		_ = mutatecommon.InjectEnv(pod, corev1.EnvVar{
			Name:  "DD_INSTRUMENTATION_LANGUAGES_DETECTED",
			Value: strings.Join(libs, ","),
		})
		_ = mutatecommon.InjectEnv(pod, corev1.EnvVar{
			Name:  "DD_INSTRUMENTATION_LANGUAGE_DETECTION_ENABLED",
			Value: strconv.FormatBool(l.enabled),
		})

		return nil
	})
}

// getLibrariesLanguageDetection runs process language auto-detection and returns languages
// to inject for APM Instrumentation.
//
// The languages information is available in workloadmeta-store
// and attached on the pod's owner.
func (w *Webhook) getLibrariesLanguageDetection(pod *corev1.Pod) libInfoLanguageDetection {
	return libInfoLanguageDetection{
		libs:    w.getAutoDetectedLibraries(pod),
		enabled: config.Datadog().GetBool("admission_controller.auto_instrumentation.inject_auto_detected_libraries"),
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

type libInfoSource int

const (
	libInfoSourceNone libInfoSource = iota
	libInfoSourceLibInjection
	libInfoSourceSingleStepInstrumentation
	libInfoSourceSingleStepLangaugeDetection
)

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

func (s libInfoSource) autodetected() bool {
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

// extractLibInfo metadata about what library information we should be
// injecting into the pod and where it came from.
func (w *Webhook) extractLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	var (
		ssiEnabled        = w.isEnabledInNamespace(pod.Namespace)
		langaugeDetection = w.getLibrariesLanguageDetection(pod)
		extracted         = extractedPodLibInfo{
			source:            libInfoSourceLibInjection,
			languageDetection: &langaugeDetection,
		}
	)

	if ssiEnabled {
		extracted.source = libInfoSourceSingleStepInstrumentation
	}

	// If the pod is "injectable" and annotated with libraries to inject, use those.
	if w.isPodEligible(pod) {
		// The library version specified via annotation on the Pod takes precedence
		// over libraries injected with Single Step Instrumentation
		libs := w.extractLibrariesFromAnnotations(pod)
		if len(libs) > 0 {
			return extracted.withLibs(libs)
		}
	}

	// If auto-instrumentation is enabled in the namespace and nothing has been overridden,
	//
	// 1. We check for pinned libraries in the config
	// 2. We use language detection (if enabled)
	// 3. We fall back to "latest"
	if ssiEnabled {
		if len(w.pinnedLibraries) > 0 {
			return extracted.withLibs(w.pinnedLibraries)
		}

		if langaugeDetection.enabled && len(langaugeDetection.libs) > 0 {
			extracted.source = libInfoSourceSingleStepLangaugeDetection
			return extracted.withLibs(langaugeDetection.libs)
		}

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

func (w *Webhook) initContainerMutators() []containerMutator {
	return []containerMutator{
		containerResourceRequirements{w.initResourceRequirements},
		containerSecurityContext{w.initSecurityContext},
	}
}

func (w *Webhook) newInjector(startTime time.Time, pod *corev1.Pod) podMutator {
	var opts []injectorOption
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

func (w *Webhook) injectAutoInstruConfig(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}

	var (
		lastError             error
		configInjector        = &libConfigInjector{}
		initContainerMutators = w.initContainerMutators()
		injector              = w.newInjector(time.Now(), pod)

		injectionType = config.source.injectionType()
		autoDetected  = config.source.autodetected()

		detectedLangsMutator = config.languageDetection.podMutator(w.version)
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

		if err := lib.podMutator(w.version, initContainerMutators, []podMutator{
			configInjector.podMutator(lib.lang),
			injector,
			detectedLangsMutator,
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

func initResources() (corev1.ResourceRequirements, error) {

	var resources = corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}

	if config.Datadog().IsSet("admission_controller.auto_instrumentation.init_resources.cpu") {
		quantity, err := resource.ParseQuantity(config.Datadog().GetString("admission_controller.auto_instrumentation.init_resources.cpu"))
		if err != nil {
			return resources, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
	} else {
		resources.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(defaultMilliCPURequest, resource.DecimalSI)
		resources.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(defaultMilliCPURequest, resource.DecimalSI)
	}

	if config.Datadog().IsSet("admission_controller.auto_instrumentation.init_resources.memory") {
		quantity, err := resource.ParseQuantity(config.Datadog().GetString("admission_controller.auto_instrumentation.init_resources.memory"))
		if err != nil {
			return resources, err
		}
		resources.Requests[corev1.ResourceMemory] = quantity
		resources.Limits[corev1.ResourceMemory] = quantity
	} else {
		resources.Requests[corev1.ResourceMemory] = *resource.NewQuantity(defaultMemoryRequest, resource.DecimalSI)
		resources.Limits[corev1.ResourceMemory] = *resource.NewQuantity(defaultMemoryRequest, resource.DecimalSI)
	}

	return resources, nil
}

func parseInitSecurityContext() (*corev1.SecurityContext, error) {
	securityContext := corev1.SecurityContext{}
	confKey := "admission_controller.auto_instrumentation.init_security_context"

	if config.Datadog().IsSet(confKey) {
		confValue := config.Datadog().GetString(confKey)
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
