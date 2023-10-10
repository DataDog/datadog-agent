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
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	admCommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
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

	imageFormat = "%s/dd-lib-%s-init:%s"
)

var (
	supportedLanguages = []language{java, js, python, dotnet, ruby}
	targetNamespaces   = config.Datadog.GetStringSlice("admission_controller.auto_instrumentation.inject_all.namespaces")
)

// InjectAutoInstrumentation injects APM libraries into pods
func InjectAutoInstrumentation(rawPod []byte, ns string, dc dynamic.Interface) ([]byte, error) {
	return mutate(rawPod, ns, injectAutoInstrumentation, dc)
}

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}

func injectAutoInstrumentation(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	if pod == nil {
		return errors.New("cannot inject lib into nil pod")
	}

	if isApmInstrumentationEnabled(pod.Namespace) {
		// if Single Step Instrumentation is enabled, pods can still opt out using the label
		if pod.GetLabels()[admCommon.EnabledLabelKey] == "false" {
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
	libsToInject := extractLibInfo(pod, containerRegistry)
	if len(libsToInject) == 0 {
		return nil
	}

	return injectAutoInstruConfig(pod, libsToInject)
}

func isNsTargeted(ns string) bool {
	if len(targetNamespaces) == 0 {
		return false
	}
	for _, targetNs := range targetNamespaces {
		if ns == targetNs {
			return true
		}
	}
	return false
}

func getAllLibsToInject(ns, registry string) map[string]libInfo {
	libsToInject := map[string]libInfo{}
	var libVersion string
	if isNsTargeted(ns) || isApmInstrumentationEnabled(ns) {
		singleStepLibraryVersions := config.Datadog.GetStringMapString("apm_config.instrumentation.lib_versions")
		for _, lang := range supportedLanguages {
			libVersion = "latest"

			// if Single Step Instrumentation enabled, check if some libraries have pinned versions
			if isApmInstrumentationEnabled(ns) {
				if version, ok := singleStepLibraryVersions[string(lang)]; ok {
					log.Warnf("Library version %s is specified for language %s", version, string(lang))
					libVersion = version
				}
			}
			libsToInject[string(lang)] = libInfo{
				lang:  lang,
				image: fmt.Sprintf(imageFormat, registry, lang, libVersion),
			}
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
func extractLibInfo(pod *corev1.Pod, containerRegistry string) []libInfo {
	libInfoMap := map[string]libInfo{}
	var libInfoList []libInfo

	// Inject all libraries if Single Step Instrumentation is enabled
	if isApmInstrumentationEnabled(pod.Namespace) {
		libInfoMap = getAllLibsToInject(pod.Namespace, containerRegistry)
		if len(libInfoMap) > 0 {
			log.Debugf("Single Step Instrumentation: Injecting all libraries into pod %q in namespace %q", podString(pod), pod.Namespace)
		}
	}

	// The library version specified via annotation on the Pod takes precedence over libraries injected with Single Step Instrumentation
	if shouldInject(pod) {
		libInfoList = extractLibrariesFromAnnotations(pod, containerRegistry, libInfoMap)
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
					image: fmt.Sprintf(imageFormat, containerRegistry, lang, version),
				})
			}
		}
	}

	return libInfoList
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
				image := fmt.Sprintf(imageFormat, registry, lang, version)
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

func injectAutoInstruConfig(pod *corev1.Pod, libsToInject []libInfo) error {
	var lastError error

	initContainerToInject := make(map[language]string)

	for _, lib := range libsToInject {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected))
			metrics.MutationAttempts.Inc(metrics.LibInjectionMutationType, strconv.FormatBool(injected), langStr)
		}()

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
			metrics.LibInjectionErrors.Inc(langStr)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "unsupported language", langStr)
			lastError = fmt.Errorf("language %q is not supported. Supported languages are %v", lib.lang, supportedLanguages)
			continue
		}

		if err != nil {
			metrics.LibInjectionErrors.Inc(langStr)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "requirements config error", langStr)
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
			metrics.LibInjectionErrors.Inc(langStr)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject init container", langStr)
			lastError = err
			log.Errorf("Cannot inject init container into pod %s: %s", podString(pod), err)
		}
		err = injectLibConfig(pod, lang)
		if err != nil {
			langStr := string(lang)
			metrics.LibInjectionErrors.Inc(langStr)
			metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject lib config", langStr)
			lastError = err
			log.Errorf("Cannot inject library configuration into pod %s: %s", podString(pod), err)
		}
	}

	// try to inject all if the annotation is set
	if err := injectLibConfig(pod, "all"); err != nil {
		metrics.LibInjectionErrors.Inc("all")
		metrics.MutationErrors.Inc(metrics.LibInjectionMutationType, "cannot inject lib config", "all")
		lastError = err
		log.Errorf("Cannot inject library configuration into pod %s: %s", podString(pod), err)
	}

	injectLibVolume(pod)

	if isApmInstrumentationEnabled(pod.Namespace) {
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
	resources, hasResources, err := initResources()
	if err != nil {
		return err
	}
	if hasResources {
		initContainer.Resources = resources
	}
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
	return nil
}

func initResources() (corev1.ResourceRequirements, bool, error) {
	hasResources := false
	var resources = corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if config.Datadog.IsSet("admission_controller.auto_instrumentation.init_resources.cpu") {
		quantity, err := resource.ParseQuantity(config.Datadog.GetString("admission_controller.auto_instrumentation.init_resources.cpu"))
		if err != nil {
			return resources, hasResources, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
		hasResources = true
	}
	if config.Datadog.IsSet("admission_controller.auto_instrumentation.init_resources.memory") {
		quantity, err := resource.ParseQuantity(config.Datadog.GetString("admission_controller.auto_instrumentation.init_resources.memory"))
		if err != nil {
			return resources, hasResources, err
		}
		resources.Requests[corev1.ResourceMemory] = quantity
		resources.Limits[corev1.ResourceMemory] = quantity
		hasResources = true
	}
	return resources, hasResources, nil
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
		Tracing:             pointer.Ptr(true),
		LogInjection:        pointer.Ptr(true),
		HealthMetrics:       pointer.Ptr(true),
		RuntimeMetrics:      pointer.Ptr(true),
		TracingSamplingRate: pointer.Ptr(1.0),
		TracingRateLimit:    pointer.Ptr(100),
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
