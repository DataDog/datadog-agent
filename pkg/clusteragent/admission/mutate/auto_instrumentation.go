// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

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
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

const (
	// Shared config
	initContainerName = "datadog-lib-init"
	volumeName        = "datadog-auto-instrumentation"
	mountPath         = "/datadog-lib"

	// Java config
	javaToolOptionsKey   = "JAVA_TOOL_OPTIONS"
	javaToolOptionsValue = " -javaagent:/datadog-lib/dd-java-agent.jar"

	// Node config
	nodeOptionsKey   = "NODE_OPTIONS"
	nodeOptionsValue = " --require=/datadog-lib/node_modules/dd-trace/init"

	// Python config
	pythonPathKey   = "PYTHONPATH"
	pythonPathValue = "/datadog-lib/"
)

type language string

const (
	java   language = "java"
	js     language = "js"
	python language = "python"
)

var (
	customLibAnnotationKeyFormat = "admission.datadoghq.com/%s-lib.custom-image"
	supportedLanguages           = []language{java, js, python}
)

// InjectAutoInstrumentation injects APM libraries into pods
func InjectAutoInstrumentation(rawPod []byte, ns string, dc dynamic.Interface) ([]byte, error) {
	return mutate(rawPod, ns, injectAutoInstrumentation, dc)
}

func injectAutoInstrumentation(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	if pod == nil {
		return errors.New("cannot inject lib into nil pod")
	}

	if containsInitContainer(pod, initContainerName) {
		// The admission can be reinvocated for the same pod
		// Fast return if we injected the library already
		log.Debugf("Init container %q already exists in pod %q", initContainerName, podString(pod))
		return nil
	}

	language, image, shouldInject := extractLibInfo(pod, config.Datadog.GetString("admission_controller.auto_instrumentation.container_registry"))
	if !shouldInject {
		return nil
	}

	return injectAutoInstruConfig(pod, language, image)
}

// extractLibInfo returns the language, the image,
// and a boolean indicating whether the library should be injected into the pod
func extractLibInfo(pod *corev1.Pod, containerRegistry string) (language, string, bool) {
	podAnnotations := pod.GetAnnotations()
	for _, lang := range supportedLanguages {
		customLibAnnotation := strings.ToLower(fmt.Sprintf(customLibAnnotationKeyFormat, lang))
		if image, found := podAnnotations[customLibAnnotation]; found {
			return lang, image, true
		}

		libVersionAnnotation := strings.ToLower(fmt.Sprintf(common.LibVersionAnnotKeyFormat, lang))
		if version, found := podAnnotations[libVersionAnnotation]; found {
			image := fmt.Sprintf("%s/dd-lib-%s-init:%s", containerRegistry, lang, version)
			return lang, image, true
		}
	}

	return "", "", false
}

func injectAutoInstruConfig(pod *corev1.Pod, lang language, image string) error {
	injected := false
	langStr := string(lang)
	defer func() {
		metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected))
	}()

	var langEnvKey string
	var langEnvFunc envValFunc
	switch lang {
	case java:
		langEnvKey = javaToolOptionsKey
		langEnvFunc = javaEnvValFunc
	case js:
		langEnvKey = nodeOptionsKey
		langEnvFunc = jsEnvValFunc
	case python:
		langEnvKey = pythonPathKey
		langEnvFunc = pythonEnvValFunc
	default:
		metrics.LibInjectionErrors.Inc(langStr)
		return fmt.Errorf("language %q is not supported. Supported languages are %v", lang, supportedLanguages)
	}
	injectLibInitContainer(pod, image)
	err := injectLibRequirements(pod, langEnvKey, langEnvFunc)
	if err != nil {
		metrics.LibInjectionErrors.Inc(langStr)
		return err
	}
	err = injectLibConfig(pod, lang)
	if err != nil {
		metrics.LibInjectionErrors.Inc(langStr)
		return err
	}
	injectLibVolume(pod)
	injected = true

	return nil
}

func injectLibInitContainer(pod *corev1.Pod, image string) {
	log.Debugf("Injecting init container named %q with image %q into pod %s", initContainerName, image, podString(pod))
	pod.Spec.InitContainers = append([]corev1.Container{
		{
			Name:    initContainerName,
			Image:   image,
			Command: []string{"sh", "copy-lib.sh", mountPath},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      volumeName,
					MountPath: mountPath,
				},
			},
		},
	}, pod.Spec.InitContainers...)
}

// injectLibRequirements injects the minimal config requirements to enable instrumentation
func injectLibRequirements(pod *corev1.Pod, envKey string, envVal envValFunc) error {
	for i, ctr := range pod.Spec.Containers {
		index := envIndex(ctr.Env, envKey)
		if index < 0 {
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, corev1.EnvVar{
				Name:  envKey,
				Value: envVal(""),
			})
		} else {
			if pod.Spec.Containers[i].Env[index].ValueFrom != nil {
				return fmt.Errorf("%q is defined via ValueFrom", envKey)
			}

			pod.Spec.Containers[i].Env[index].Value = envVal(pod.Spec.Containers[i].Env[index].Value)
		}

		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{Name: volumeName, MountPath: mountPath})
	}

	return nil
}

// injectLibConfig injects additional library configuration extracted from pod annotations
func injectLibConfig(pod *corev1.Pod, lang language) error {
	configAnnotKey := fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, lang)
	confString, found := pod.GetAnnotations()[configAnnotKey]
	if !found {
		log.Debugf("Config annotation key %q not found on pod %s, skipping config injection", configAnnotKey, podString(pod))
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

type envValFunc func(string) string

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
