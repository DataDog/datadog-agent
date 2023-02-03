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
	volumeName = "datadog-auto-instrumentation"
	mountPath  = "/datadog-lib"

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

	libVersionAnnotationKeyFormat    = "admission.datadoghq.com/%s-lib.version"
	customLibAnnotationKeyFormat     = "admission.datadoghq.com/%s-lib.custom-image"
	libVersionAnnotationKeyCtrFormat = "admission.datadoghq.com/%s.%s-lib.version"
	customLibAnnotationKeyCtrFormat  = "admission.datadoghq.com/%s.%s-lib.custom-image"
)

var (
	supportedLanguages = []language{java, js, python}
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

	for _, lang := range supportedLanguages {
		if containsInitContainer(pod, initContainerName(lang)) {
			// The admission can be reinvocated for the same pod
			// Fast return if we injected the library already
			log.Debugf("Init container %q already exists in pod %q", initContainerName(lang), podString(pod))
			return nil
		}
	}

	libsToInject := extractLibInfo(pod, config.Datadog.GetString("admission_controller.auto_instrumentation.container_registry"))
	if len(libsToInject) == 0 {
		return nil
	}

	return injectAutoInstruConfig(pod, libsToInject)
}

type libInfo struct {
	ctrName string // empty means all containers
	lang    language
	image   string
}

// extractLibInfo returns the language, the image,
// and a boolean indicating whether the library should be injected into the pod
func extractLibInfo(pod *corev1.Pod, containerRegistry string) []libInfo {
	libInfoList := []libInfo{}
	podAnnotations := pod.GetAnnotations()
	for _, lang := range supportedLanguages {
		customLibAnnotation := strings.ToLower(fmt.Sprintf(customLibAnnotationKeyFormat, lang))
		if image, found := podAnnotations[customLibAnnotation]; found {
			libInfoList = append(libInfoList, libInfo{
				lang:  lang,
				image: image,
			})
		}

		libVersionAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyFormat, lang))
		if version, found := podAnnotations[libVersionAnnotation]; found {
			image := fmt.Sprintf("%s/dd-lib-%s-init:%s", containerRegistry, lang, version)
			libInfoList = append(libInfoList, libInfo{
				lang:  lang,
				image: image,
			})
		}

		for _, ctr := range pod.Spec.Containers {
			customLibAnnotation := strings.ToLower(fmt.Sprintf(customLibAnnotationKeyCtrFormat, ctr.Name, lang))
			if image, found := podAnnotations[customLibAnnotation]; found {
				libInfoList = append(libInfoList, libInfo{
					ctrName: ctr.Name,
					lang:    lang,
					image:   image,
				})
			}

			libVersionAnnotation := strings.ToLower(fmt.Sprintf(libVersionAnnotationKeyCtrFormat, ctr.Name, lang))
			if version, found := podAnnotations[libVersionAnnotation]; found {
				image := fmt.Sprintf("%s/dd-lib-%s-init:%s", containerRegistry, lang, version)
				libInfoList = append(libInfoList, libInfo{
					ctrName: ctr.Name,
					lang:    lang,
					image:   image,
				})
			}
		}

	}

	return libInfoList
}

func injectAutoInstruConfig(pod *corev1.Pod, libsToInject []libInfo) error {
	var lastError error

	initContainerToInject := make(map[language]string)

	for _, lib := range libsToInject {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected))
		}()

		var err error
		switch lib.lang {
		case java:
			err = injectLibRequirements(pod, lib.ctrName, javaToolOptionsKey, javaEnvValFunc)
		case js:
			err = injectLibRequirements(pod, lib.ctrName, nodeOptionsKey, jsEnvValFunc)
		case python:
			err = injectLibRequirements(pod, lib.ctrName, pythonPathKey, pythonEnvValFunc)
		default:
			metrics.LibInjectionErrors.Inc(langStr)
			lastError = fmt.Errorf("language %q is not supported. Supported languages are %v", lib.lang, supportedLanguages)
			continue
		}

		if err != nil {
			metrics.LibInjectionErrors.Inc(langStr)
			lastError = err
		}

		initContainerToInject[lib.lang] = lib.image

		injected = true
	}

	for lang, image := range initContainerToInject {
		injectLibInitContainer(pod, image, lang)
		err := injectLibConfig(pod, lang)
		if err != nil {
			metrics.LibInjectionErrors.Inc(string(lang))
			lastError = err
		}
	}

	injectLibVolume(pod)

	return lastError
}

func injectLibInitContainer(pod *corev1.Pod, image string, lang language) {
	initCtrName := initContainerName(lang)
	log.Debugf("Injecting init container named %q with image %q into pod %s", initCtrName, image, podString(pod))
	pod.Spec.InitContainers = append([]corev1.Container{
		{
			Name:    initCtrName,
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
func injectLibRequirements(pod *corev1.Pod, ctrName string, envKey string, envVal envValFunc) error {
	for i, ctr := range pod.Spec.Containers {
		if ctrName != "" && ctrName != ctr.Name {
			continue
		}

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
