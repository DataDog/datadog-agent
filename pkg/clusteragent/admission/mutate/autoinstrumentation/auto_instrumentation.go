// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoinstrumentation implements the webhook that injects APM libraries
// into pods
package autoinstrumentation

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	volumeName = "datadog-auto-instrumentation"
	mountPath  = "/datadog-lib"

	// apmInjectionErrorAnnotationKey this annotation is added when the apm auto-instrumentation admission controller failed to mutate the Pod.
	apmInjectionErrorAnnotationKey = "apm.datadoghq.com/injection-error"
)

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
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

// getAllLatestDefaultLibraries returns all supported by APM Instrumentation tracing libraries
// that should be enabled by default
func getAllLatestDefaultLibraries(containerRegistry string) []libInfo {
	var libsToInject []libInfo
	for _, lang := range supportedLanguages {
		libsToInject = append(libsToInject, lang.defaultLibInfo(containerRegistry, ""))
	}

	return libsToInject
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

func initContainerIsSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// podSumRessourceRequirements computes the sum of cpu/memory necessary for the whole pod.
// This is computed as max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
// for both limit and request
// https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/#resource-sharing-within-containers
func podSumRessourceRequirements(pod *corev1.Pod) corev1.ResourceRequirements {
	ressourceRequirement := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	for _, k := range [2]corev1.ResourceName{corev1.ResourceMemory, corev1.ResourceCPU} {
		// Take max(initContainer ressource)
		maxInitContainerLimit := resource.Quantity{}
		maxInitContainerRequest := resource.Quantity{}
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if initContainerIsSidecar(c) {
				// This is a sidecar container, since it will run alongside the main containers
				// we need to add it's resources to the main container's resources
				continue
			}
			if limit, ok := c.Resources.Limits[k]; ok {
				if limit.Cmp(maxInitContainerLimit) == 1 {
					maxInitContainerLimit = limit
				}
			}
			if request, ok := c.Resources.Requests[k]; ok {
				if request.Cmp(maxInitContainerRequest) == 1 {
					maxInitContainerRequest = request
				}
			}
		}

		// Take sum(container resources) + sum(sidecar containers)
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
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if !initContainerIsSidecar(c) {
				continue
			}
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}

		// Take max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
		if limitSum.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = limitSum
		}
		if reqSum.Cmp(maxInitContainerRequest) == 1 {
			maxInitContainerRequest = reqSum
		}

		// Ensure that the limit is greater or equal to the request
		if maxInitContainerRequest.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = maxInitContainerRequest
		}

		if maxInitContainerLimit.CmpInt64(0) == 1 {
			ressourceRequirement.Limits[k] = maxInitContainerLimit
		}
		if maxInitContainerRequest.CmpInt64(0) == 1 {
			ressourceRequirement.Requests[k] = maxInitContainerRequest
		}
	}

	return ressourceRequirement
}

type injectionResourceRequirementsDecision struct {
	skipInjection bool
	message       string
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
func initContainerResourceRequirements(pod *corev1.Pod, conf initResourceRequirementConfiguration) (requirements corev1.ResourceRequirements, decision injectionResourceRequirementsDecision) {
	requirements = corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}
	podRequirements := podSumRessourceRequirements(pod)
	insufficientResourcesMessage := "The overall pod's containers limit is too low"
	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		if q, ok := conf[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
		} else {
			if maxPodLim, ok := podRequirements.Limits[k]; ok {
				// If the pod before adding instrumentation init containers would have had a limits smaller than
				// a certain amount, we just don't do anything, for two reasons:
				// 1. The init containers need quite a lot of memory/CPU in order to not OOM or initialize in reasonnable time
				// 2. The APM libraries themselves will increase footprint of the container by a
				//   non trivial amount, and we don't want to cause issues for constrained apps
				switch k {
				case corev1.ResourceMemory:
					if minimumMemoryLimit.Cmp(maxPodLim) == 1 {
						decision.skipInjection = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumMemoryLimit.String())
					}
				case corev1.ResourceCPU:
					if minimumCPULimit.Cmp(maxPodLim) == 1 {
						decision.skipInjection = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumCPULimit.String())
					}
				default:
					// We don't support other resources
				}
				requirements.Limits[k] = maxPodLim
			}
			if maxPodReq, ok := podRequirements.Requests[k]; ok {
				requirements.Requests[k] = maxPodReq
			}
		}
	}
	if decision.skipInjection {
		log.Debug(insufficientResourcesMessage)
		decision.message = insufficientResourcesMessage
	}
	return requirements, decision
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}
