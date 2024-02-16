// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package mutate implements the mutations needed by the auto-instrumentation feature.
package mutate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	apiServerCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	// Shared config
	volumeName = "datadog-auto-instrumentation"
	mountPath  = "/datadog-lib"

	// Java config
	javaToolOptionsKey   = "JAVA_TOOL_OPTIONS"
	javaToolOptionsValue = " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/continuousprofiler/tmp/hs_err_pid_%p.log"

	// Node config
	nodeOptionsKey   = "NODE_OPTIONS"
	nodeOptionsValue = " --require=/datadog-lib/node_modules/dd-trace/init"

	// Python config
	pythonPathKey   = "PYTHONPATH"
	pythonPathValue = "/datadog-lib/"

	// Dotnet config
	dotnetClrEnableProfilingKey   = "CORECLR_ENABLE_PROFILING"
	dotnetClrEnableProfilingValue = "1"

	dotnetClrProfilerIDKey   = "CORECLR_PROFILER"
	dotnetClrProfilerIDValue = "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}"

	dotnetClrProfilerPathKey   = "CORECLR_PROFILER_PATH"
	dotnetClrProfilerPathValue = "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so"

	dotnetTracerHomeKey   = "DD_DOTNET_TRACER_HOME"
	dotnetTracerHomeValue = "/datadog-lib"

	dotnetTracerLogDirectoryKey   = "DD_TRACE_LOG_DIRECTORY"
	dotnetTracerLogDirectoryValue = "/datadog-lib/logs"

	dotnetProfilingLdPreloadKey   = "LD_PRELOAD"
	dotnetProfilingLdPreloadValue = "/datadog-lib/continuousprofiler/Datadog.Linux.ApiWrapper.x64.so"

	// Ruby config
	rubyOptKey   = "RUBYOPT"
	rubyOptValue = " -r/datadog-lib/auto_inject"
)

type language string

const (
	java   language = "java"
	js     language = "js"
	python language = "python"
	dotnet language = "dotnet"
	ruby   language = "ruby"

	libVersionAnnotationKeyFormat    = "admission.datadoghq.com/%s-lib.version"
	customLibAnnotationKeyFormat     = "admission.datadoghq.com/%s-lib.custom-image"
	libVersionAnnotationKeyCtrFormat = "admission.datadoghq.com/%s.%s-lib.version"
	customLibAnnotationKeyCtrFormat  = "admission.datadoghq.com/%s.%s-lib.custom-image"

	// defaultMilliCPURequest defines default milli cpu request number.
	defaultMilliCPURequest int64 = 50 // 0.05 core
	// defaultMemoryRequest defines default memory request size.
	defaultMemoryRequest int64 = 20 * 1024 * 1024 // 20 MB

	// Env vars
	instrumentationInstallTypeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TYPE"
	instrumentationInstallTimeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TIME"
	instrumentationInstallIDEnvVarName   = "DD_INSTRUMENTATION_INSTALL_ID"

	// Values for Env variable DD_INSTRUMENTATION_INSTALL_TYPE
	singleStepInstrumentationInstallType   = "k8s_single_step"
	localLibraryInstrumentationInstallType = "k8s_lib_injection"
)

var (
	supportedLanguages = []language{java, js, python, dotnet, ruby}

	singleStepInstrumentationInstallTypeEnvVar = corev1.EnvVar{
		Name:  instrumentationInstallTypeEnvVarName,
		Value: singleStepInstrumentationInstallType,
	}

	localLibraryInstrumentationInstallTypeEnvVar = corev1.EnvVar{
		Name:  instrumentationInstallTypeEnvVarName,
		Value: localLibraryInstrumentationInstallType,
	}

	// We need a global variable to store the APMInstrumentationWebhook instance
	// because other webhooks depend on it. The "config" and the "tags" webhooks
	// depend on the "auto_instrumentation" webhook to decide if a pod should be
	// injected. They first check if the pod has the label to enable mutations.
	// If it doesn't, they mutate the pod if the option to mutate unlabeled is
	// set to true or if APM SSI is enabled in the namespace.
	apmInstrumentationWebhook *APMInstrumentationWebhook
	errInitAPMInstrumentation error
	initOnce                  sync.Once
)

// APMInstrumentationWebhook is the main handler for the APM instrumentation mutating webhook endpoints
type APMInstrumentationWebhook struct {
	// filter is used to filter the pods to instrument
	filter *containers.Filter
}

// GetAPMInstrumentationWebhook returns the APMInstrumentationWebhook instance,
// creating it if it doesn't exist
func GetAPMInstrumentationWebhook() (*APMInstrumentationWebhook, error) {
	initOnce.Do(func() {
		if apmInstrumentationWebhook == nil {
			apmInstrumentationWebhook, errInitAPMInstrumentation = newAPMInstrumentationWebhook()
		}
	})

	return apmInstrumentationWebhook, errInitAPMInstrumentation
}

// newAPMInstrumentationWebhook returns a new instance of APMInstrumentationWebhook
func newAPMInstrumentationWebhook() (*APMInstrumentationWebhook, error) {
	filter, err := apmSSINamespaceFilter()
	if err != nil {
		return nil, err
	}

	return &APMInstrumentationWebhook{
		filter: filter,
	}, nil
}

// apmSSINamespaceFilter returns the filter used by APM SSI to filter namespaces.
// The filter excludes two namespaces by default: "kube-system" and the
// namespace where datadog is installed.
// Cases:
// - No enabled namespaces and no disabled namespaces: inject in all namespaces
// except the 2 namespaces excluded by default.
// - Enabled namespaces and no disabled namespaces: inject only in the
// namespaces specified in the list of enabled namespaces. If one of the
// namespaces excluded by default is included in the list, it will be injected.
// - Disabled namespaces and no enabled namespaces: inject only in the
// namespaces that are not included in the list of disabled namespaces and that
// are not one of the ones disabled by default.
// - Enabled and disabled namespaces: return error.
func apmSSINamespaceFilter() (*containers.Filter, error) {
	apmEnabledNamespaces := config.Datadog.GetStringSlice("apm_config.instrumentation.enabled_namespaces")
	apmDisabledNamespaces := config.Datadog.GetStringSlice("apm_config.instrumentation.disabled_namespaces")

	if len(apmEnabledNamespaces) > 0 && len(apmDisabledNamespaces) > 0 {
		return nil, fmt.Errorf("apm.instrumentation.enabled_namespaces and apm.instrumentation.disabled_namespaces configuration cannot be set together")
	}

	// Prefix the namespaces as needed by the containers.Filter.
	prefix := containers.KubeNamespaceFilterPrefix
	apmEnabledNamespacesWithPrefix := make([]string, len(apmEnabledNamespaces))
	apmDisabledNamespacesWithPrefix := make([]string, len(apmDisabledNamespaces))

	for i := range apmEnabledNamespaces {
		apmEnabledNamespacesWithPrefix[i] = prefix + apmEnabledNamespaces[i]
	}
	for i := range apmDisabledNamespaces {
		apmDisabledNamespacesWithPrefix[i] = prefix + apmDisabledNamespaces[i]
	}

	disabledByDefault := []string{
		prefix + "kube-system",
		prefix + apiServerCommon.GetResourcesNamespace(),
	}

	var filterExcludeList []string
	if len(apmEnabledNamespacesWithPrefix) > 0 && len(apmDisabledNamespacesWithPrefix) == 0 {
		// In this case, we want to include only the namespaces in the enabled list.
		// In the containers.Filter, the include list is checked before the
		// exclude list, that's why we set the exclude list to all namespaces.
		filterExcludeList = []string{prefix + ".*"}
	} else {
		filterExcludeList = append(apmDisabledNamespacesWithPrefix, disabledByDefault...)
	}

	return containers.NewFilter(containers.GlobalFilter, apmEnabledNamespacesWithPrefix, filterExcludeList)
}

// InjectAutoInstrumentation injects APM libraries into pods
func (w *APMInstrumentationWebhook) InjectAutoInstrumentation(rawPod []byte, _ string, ns string, _ *authenticationv1.UserInfo, dc dynamic.Interface, _ k8s.Interface) ([]byte, error) {
	return Mutate(rawPod, ns, w.injectAutoInstrumentation, dc)
}

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}

func libImageName(registry string, lang language, tag string) string {
	imageFormat := "%s/dd-lib-%s-init:%s"
	return fmt.Sprintf(imageFormat, registry, lang, tag)
}

func (w *APMInstrumentationWebhook) injectAutoInstrumentation(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	if pod == nil {
		return errors.New("cannot inject lib into nil pod")
	}
	injectApmTelemetryConfig(pod)

	if w.isEnabled(pod.Namespace) {
		// if Single Step Instrumentation is enabled, pods can still opt out using the label
		if pod.GetLabels()[common.EnabledLabelKey] == "false" {
			log.Debugf("Skipping single step instrumentation of pod %q due to label", podString(pod))
			return nil
		}
	} else if !shouldMutatePod(pod) {
		log.Debugf("Skipping auto instrumentation of pod %q because pod mutation is not allowed", podString(pod))
		return nil
	}
	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			// The admission can be reinvocated for the same pod
			// Fast return if we injected the library already
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), podString(pod))
			return nil
		}
	}

	containerRegistry := config.Datadog.GetString("admission_controller.auto_instrumentation.container_registry")
	libsToInject, autoDetected := w.extractLibInfo(pod, containerRegistry)
	if len(libsToInject) == 0 {
		return nil
	}
	// Inject env variables used for Onboarding KPIs propagation
	var injectionType string
	if w.isEnabled(pod.Namespace) {
		// if Single Step Instrumentation is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_single_step
		_ = injectEnv(pod, singleStepInstrumentationInstallTypeEnvVar)
		injectionType = singleStepInstrumentationInstallType
	} else {
		// if local library injection is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_lib_injection
		_ = injectEnv(pod, localLibraryInstrumentationInstallTypeEnvVar)
		injectionType = localLibraryInstrumentationInstallType
	}

	return w.injectAutoInstruConfig(pod, libsToInject, autoDetected, injectionType)
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
	_ = injectEnv(pod, instrumentationInstallTimeEnvVar)

	// inject DD_INSTRUMENTATION_INSTALL_ID with UUID created during the Agent install time
	instrumentationInstallIDEnvVar := corev1.EnvVar{
		Name:  instrumentationInstallIDEnvVarName,
		Value: os.Getenv(instrumentationInstallIDEnvVarName),
	}
	_ = injectEnv(pod, instrumentationInstallIDEnvVar)
}

func getAllLibsToInject(registry string) map[string]libInfo {
	libsToInject := map[string]libInfo{}
	var libVersion string
	singleStepLibraryVersions := config.Datadog.GetStringMapString("apm_config.instrumentation.lib_versions")

	for _, lang := range supportedLanguages {
		libVersion = "latest"

		if version, ok := singleStepLibraryVersions[string(lang)]; ok {
			log.Warnf("Library version %s is specified for language %s", version, string(lang))
			libVersion = version
		}

		libsToInject[string(lang)] = libInfo{
			lang:  lang,
			image: libImageName(registry, lang, libVersion),
		}
	}
	return libsToInject
}

type libInfo struct {
	ctrName string // empty means all containers
	lang    language
	image   string
}

// extractLibInfo returns the language, the image,
// and a boolean indicating whether the library should be injected into the pod
func (w *APMInstrumentationWebhook) extractLibInfo(pod *corev1.Pod, containerRegistry string) ([]libInfo, bool) {
	libInfoMap := map[string]libInfo{}
	var libInfoList []libInfo
	var autoDetected = false

	// Inject all libraries if Single Step Instrumentation is enabled
	if w.isEnabled(pod.Namespace) {
		libInfoMap = getAllLibsToInject(containerRegistry)
		if len(libInfoMap) > 0 {
			log.Debugf("Single Step Instrumentation: Injecting all libraries into pod %q in namespace %q", podString(pod), pod.Namespace)
		}
	}

	// The library version specified via annotation on the Pod takes precedence over libraries injected with Single Step Instrumentation
	if shouldInject(pod) {
		libInfoList = extractLibrariesFromAnnotations(pod, containerRegistry, libInfoMap)

		// If user doesn't provide langages information, try getting the languages from process languages auto-detection. The langages information are available in workloadmeta-store and attached on the pod's owner.
		if len(libInfoList) == 0 && config.Datadog.GetBool("admission_controller.inject_auto_detected_libraries") {
			libInfoList = extractLibrariesFromOwnerAnnotations(pod, containerRegistry)
			if len(libInfoList) > 0 {
				autoDetected = true
			}
		}
	}

	if len(libInfoList) == 0 {
		// Inject all if admission.datadoghq.com/all-lib.version exists
		// without any other language-specific annotations.
		// This annotation is typically expected to be set via remote-config
		// for batch instrumentation without language detection.
		injectAllAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyFormat, "all"))
		if version, found := pod.Annotations[injectAllAnnotation]; found {
			// This logic will be updated once we bundle all libs in
			// one single init container. Versions will be supported by then.
			if version != "latest" {
				log.Warnf("Ignoring version %q. To inject all libs, the only supported version is latest for now", version)
				version = "latest"
			}
			for _, lang := range supportedLanguages {
				libInfoList = append(libInfoList, libInfo{
					lang:  lang,
					image: libImageName(containerRegistry, lang, version),
				})
			}
		}
	}

	return libInfoList, autoDetected
}

// extractLibrariesFromOwnerAnnotations constructs the libraries to be injected if the languages
// were stored in workloadmeta store based on owner annotations (for example: Deployment, Daemonset, Statefulset).
func extractLibrariesFromOwnerAnnotations(pod *corev1.Pod, registry string) []libInfo {
	libList := []libInfo{}

	ownerName, ownerKind, found := getOwnerNameAndKind(pod)
	if !found {
		return libList
	}

	// TODO [Workloadmeta][Component Framework]: Use workloadmeta store as a component
	store := workloadmeta.GetGlobalStore()
	if store == nil {
		return libList
	}

	// Currently we only support deployments
	switch ownerKind {
	case "Deployment":
		libList = getLibListFromDeploymentAnnotations(store, ownerName, pod.Namespace, registry)
	default:
		log.Debugf("This ownerKind:%s is not yet supported by the process language auto-detection feature", ownerKind)
	}

	return libList
}

func extractLibrariesFromAnnotations(
	pod *corev1.Pod,
	registry string,
	libInfoMap map[string]libInfo,
) []libInfo {
	annotations := pod.Annotations
	libList := []libInfo{}
	for _, lang := range supportedLanguages {
		customLibAnnotation := strings.ToLower(fmt.Sprintf(customLibAnnotationKeyFormat, lang))
		if image, found := annotations[customLibAnnotation]; found {
			if li, ok := libInfoMap[string(lang)]; ok {
				log.Debugf(
					"Found %s library annotation %s, will overwrite %s injected with Single Step Instrumentation",
					string(lang), image, li.image,
				)
			}
			libInfoMap[string(lang)] = libInfo{
				lang:  lang,
				image: image,
			}
		}

		libVersionAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyFormat, lang))
		if version, found := annotations[libVersionAnnotation]; found {
			image := fmt.Sprintf("%s/dd-lib-%s-init:%s", registry, lang, version)
			if li, ok := libInfoMap[string(lang)]; ok {
				log.Debugf(
					"Found %s library annotation for version %s, will overwrite %s injected with Single Step Instrumentation",
					string(lang), version, li.image,
				)
			}
			libInfoMap[string(lang)] = libInfo{
				lang:  lang,
				image: image,
			}
		}

		for _, ctr := range pod.Spec.Containers {
			customLibAnnotation := strings.ToLower(fmt.Sprintf(customLibAnnotationKeyCtrFormat, ctr.Name, lang))
			if image, found := annotations[customLibAnnotation]; found {
				log.Debugf(
					"Found custom library annotation for %s, will inject %s to container %s",
					string(lang), image, ctr.Name,
				)
				libList = append(libList, libInfo{ctrName: ctr.Name, lang: lang, image: image})
			}

			libVersionAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyCtrFormat, ctr.Name, lang))
			if version, found := annotations[libVersionAnnotation]; found {
				image := libImageName(registry, lang, version)
				log.Debugf(
					"Found version library annotation for %s, will inject %s to container %s",
					string(lang), image, ctr.Name,
				)
				libList = append(libList, libInfo{ctrName: ctr.Name, lang: lang, image: image})
			}
		}
	}

	for _, li := range libInfoMap {
		libList = append(libList, li)
	}
	return libList
}

// isEnabled indicates if Single Step Instrumentation is enabled for the namespace in the cluster
func (w *APMInstrumentationWebhook) isEnabled(namespace string) bool {
	apmInstrumentationEnabled := config.Datadog.GetBool("apm_config.instrumentation.enabled")

	if !apmInstrumentationEnabled {
		log.Debugf("APM Instrumentation is disabled")
		return false
	}

	return !w.filter.IsExcluded(nil, "", "", namespace)
}

func (w *APMInstrumentationWebhook) injectAutoInstruConfig(pod *corev1.Pod, libsToInject []libInfo, autoDetected bool, injectionType string) error {
	var lastError error

	initContainerToInject := make(map[language]string)

	for _, lib := range libsToInject {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected), strconv.FormatBool(autoDetected), injectionType)
			metrics.MutationAttempts.Inc(metrics.LibInjectionMutationType, strconv.FormatBool(injected), langStr, strconv.FormatBool(autoDetected))
		}()

		_ = injectEnv(pod, localLibraryInstrumentationInstallTypeEnvVar)
		var err error
		switch lib.lang {
		case java:
			err = injectLibRequirements(pod, lib.ctrName, []envVar{
				{
					key:     javaToolOptionsKey,
					valFunc: javaEnvValFunc,
				}})
		case js:
			err = injectLibRequirements(pod, lib.ctrName, []envVar{
				{
					key:     nodeOptionsKey,
					valFunc: jsEnvValFunc,
				}})
		case python:
			err = injectLibRequirements(pod, lib.ctrName, []envVar{
				{
					key:     pythonPathKey,
					valFunc: pythonEnvValFunc,
				}})
		case dotnet:
			err = injectLibRequirements(pod, lib.ctrName, []envVar{
				{
					key:     dotnetClrEnableProfilingKey,
					valFunc: identityValFunc(dotnetClrEnableProfilingValue),
				},
				{
					key:     dotnetClrProfilerIDKey,
					valFunc: identityValFunc(dotnetClrProfilerIDValue),
				},
				{
					key:     dotnetClrProfilerPathKey,
					valFunc: identityValFunc(dotnetClrProfilerPathValue),
				},
				{
					key:     dotnetTracerHomeKey,
					valFunc: identityValFunc(dotnetTracerHomeValue),
				},
				{
					key:     dotnetTracerLogDirectoryKey,
					valFunc: identityValFunc(dotnetTracerLogDirectoryValue),
				},
				{
					key:     dotnetProfilingLdPreloadKey,
					valFunc: dotnetProfilingLdPreloadEnvValFunc,
				}})
		case ruby:
			err = injectLibRequirements(pod, lib.ctrName, []envVar{
				{
					key:     rubyOptKey,
					valFunc: rubyEnvValFunc,
				}})
		default:
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "unsupported language", langStr, strconv.FormatBool(autoDetected))
			lastError = fmt.Errorf("language %q is not supported. Supported languages are %v", lib.lang, supportedLanguages)
			continue
		}

		if err != nil {
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "requirements config error", langStr, strconv.FormatBool(autoDetected))
			lastError = err
			log.Errorf("Error injecting library config requirements: %s", err)
		}

		initContainerToInject[lib.lang] = lib.image

		injected = true
	}

	for lang, image := range initContainerToInject {
		err := injectLibInitContainer(pod, image, lang)
		if err != nil {
			langStr := string(lang)
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject init container", langStr, strconv.FormatBool(autoDetected))
			lastError = err
			log.Errorf("Cannot inject init container into pod %s: %s", podString(pod), err)
		}
		err = injectLibConfig(pod, lang)
		if err != nil {
			langStr := string(lang)
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject lib config", langStr, strconv.FormatBool(autoDetected))
			lastError = err
			log.Errorf("Cannot inject library configuration into pod %s: %s", podString(pod), err)
		}
	}

	// try to inject all if the annotation is set
	if err := injectLibConfig(pod, "all"); err != nil {
		metrics.LibInjectionErrors.Inc("all", strconv.FormatBool(autoDetected), injectionType)
		metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject lib config", "all", strconv.FormatBool(autoDetected))
		lastError = err
		log.Errorf("Cannot inject library configuration into pod %s: %s", podString(pod), err)
	}

	injectLibVolume(pod)

	if w.isEnabled(pod.Namespace) {
		libConfig := basicConfig()
		if name, err := getServiceNameFromPod(pod); err == nil {
			// Set service name if it can be derived from a pod
			libConfig.ServiceName = pointer.Ptr(name)
		}
		for _, env := range libConfig.ToEnvs() {
			_ = injectEnv(pod, env)
		}
	}

	return lastError
}

func injectLibInitContainer(pod *corev1.Pod, image string, lang language) error {
	initCtrName := initContainerName(lang)
	log.Debugf("Injecting init container named %q with image %q into pod %s", initCtrName, image, podString(pod))
	initContainer := corev1.Container{
		Name:    initCtrName,
		Image:   image,
		Command: []string{"sh", "copy-lib.sh", mountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: mountPath,
			},
		},
	}

	resources, err := initResources()
	if err != nil {
		return err
	}
	initContainer.Resources = resources
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
	return nil
}

func initResources() (corev1.ResourceRequirements, error) {

	var resources = corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}

	if config.Datadog.IsSet("admission_controller.auto_instrumentation.init_resources.cpu") {
		quantity, err := resource.ParseQuantity(config.Datadog.GetString("admission_controller.auto_instrumentation.init_resources.cpu"))
		if err != nil {
			return resources, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
	} else {
		resources.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(defaultMilliCPURequest, resource.DecimalSI)
		resources.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(defaultMilliCPURequest, resource.DecimalSI)
	}

	if config.Datadog.IsSet("admission_controller.auto_instrumentation.init_resources.memory") {
		quantity, err := resource.ParseQuantity(config.Datadog.GetString("admission_controller.auto_instrumentation.init_resources.memory"))
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

// injectLibRequirements injects the minimal config requirements (env vars and volume mounts) to enable instrumentation
func injectLibRequirements(pod *corev1.Pod, ctrName string, envVars []envVar) error {
	for i, ctr := range pod.Spec.Containers {
		if ctrName != "" && ctrName != ctr.Name {
			continue
		}

		for _, envVarPair := range envVars {
			index := envIndex(ctr.Env, envVarPair.key)
			if index < 0 {
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, corev1.EnvVar{
					Name:  envVarPair.key,
					Value: envVarPair.valFunc(""),
				})
			} else {
				if pod.Spec.Containers[i].Env[index].ValueFrom != nil {
					return fmt.Errorf("%q is defined via ValueFrom", envVarPair.key)
				}

				pod.Spec.Containers[i].Env[index].Value = envVarPair.valFunc(pod.Spec.Containers[i].Env[index].Value)
			}
		}

		volumeAlreadyMounted := false
		for _, vol := range pod.Spec.Containers[i].VolumeMounts {
			if vol.Name == volumeName {
				volumeAlreadyMounted = true
				break
			}
		}
		if !volumeAlreadyMounted {
			pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{Name: volumeName, MountPath: mountPath})
		}
	}

	return nil
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

// basicConfig returns the default tracing config to inject into application pods
// when no other config has been provided.
func basicConfig() common.LibConfig {
	return common.LibConfig{
		Tracing:        pointer.Ptr(true),
		LogInjection:   pointer.Ptr(true),
		HealthMetrics:  pointer.Ptr(true),
		RuntimeMetrics: pointer.Ptr(true),
	}
}

// injectLibConfig injects additional library configuration extracted from pod annotations
func injectLibConfig(pod *corev1.Pod, lang language) error {
	configAnnotKey := fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, lang)
	confString, found := pod.GetAnnotations()[configAnnotKey]
	if !found {
		log.Tracef("Config annotation key %q not found on pod %s, skipping config injection", configAnnotKey, podString(pod))
		return nil
	}
	log.Infof("Config annotation key %q found on pod %s, config: %q", configAnnotKey, podString(pod), confString)
	var libConfig common.LibConfig
	err := json.Unmarshal([]byte(confString), &libConfig)
	if err != nil {
		return fmt.Errorf("invalid json config in annotation %s=%s: %w", configAnnotKey, confString, err)
	}
	for _, env := range libConfig.ToEnvs() {
		_ = injectEnv(pod, env)
	}

	return nil
}

func injectLibVolume(pod *corev1.Pod) {
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}

type envVar struct {
	key     string
	valFunc envValFunc
}

type envValFunc func(string) string

func identityValFunc(s string) envValFunc {
	return func(string) string { return s }
}

func javaEnvValFunc(predefinedVal string) string {
	return predefinedVal + javaToolOptionsValue
}

func jsEnvValFunc(predefinedVal string) string {
	return predefinedVal + nodeOptionsValue
}

func pythonEnvValFunc(predefinedVal string) string {
	if predefinedVal == "" {
		return pythonPathValue
	}
	return fmt.Sprintf("%s:%s", pythonPathValue, predefinedVal)
}

func dotnetProfilingLdPreloadEnvValFunc(predefinedVal string) string {
	if predefinedVal == "" {
		return dotnetProfilingLdPreloadValue
	}
	return fmt.Sprintf("%s:%s", dotnetProfilingLdPreloadValue, predefinedVal)
}

func rubyEnvValFunc(predefinedVal string) string {
	return predefinedVal + rubyOptValue
}
