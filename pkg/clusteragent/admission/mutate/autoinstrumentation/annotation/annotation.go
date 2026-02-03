// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package annotation provides shared annotation utilities for APM injection.
package annotation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// User configurable annotations.
const (
	// InjectorVersion sets the injector version in Local SDK Injection.
	// Example value: 0.52.0
	InjectorVersion = "admission.datadoghq.com/apm-inject.version"
	// InjectorImage sets the complete injector image in Local SDK Injection.
	// Example value: gcr.io/datadoghq/apm-inject:0.52.0
	InjectorImage = "admission.datadoghq.com/apm-inject.custom-image"
	// EnableDebug adds debug environment variables to the pod during mutation.
	// Example value: true
	EnableDebug = "admission.datadoghq.com/apm-inject.debug"
	// InjectionMode specifies the injection mode (e.g. init_container, csi).
	// Example value: csi
	InjectionMode = "admission.datadoghq.com/apm-inject.injection-mode"
	// LibraryVersion sets the library to use during Local SDK Injection.
	// Example annotation: admission.datadoghq.com/python-lib.version
	// Example value: v3
	LibraryVersion LibraryAnnotationFormat = "admission.datadoghq.com/%s-lib.version"
	// LibraryImage sets the complete library image to use during Local SDK Injection.
	// Example annotation: admission.datadoghq.com/python-lib.custom-image
	// Example value: gcr.io/datadoghq/dd-lib-python-init:v3
	LibraryImage LibraryAnnotationFormat = "admission.datadoghq.com/%s-lib.custom-image"
	// LibraryConfigV1 sets library configurations that will get passed to SDKs.
	// Example annotation: admission.datadoghq.com/python-lib.config.v1
	// Example value: {"runtime_metrics_enabled":true,"tracing_rate_limit":50}
	LibraryConfigV1 LibraryAnnotationFormat = "admission.datadoghq.com/%s-lib.config.v1"
	// LibraryContainerVersion will only set the library version in the specified container.
	// Example container: app
	// Example annotation: admission.datadoghq.com/app.python-lib.version
	// Example value: v3
	LibraryContainerVersion LibraryContainerAnnotationFormat = "admission.datadoghq.com/%s.%s-lib.version"
	// LibraryContainerImage will only set the library image in the specified container.
	// Example container: app
	// Example annotation: admission.datadoghq.com/app.python-lib.custom-image
	// Example value: gcr.io/datadoghq/dd-lib-python-init:v3
	LibraryContainerImage LibraryContainerAnnotationFormat = "admission.datadoghq.com/%s.%s-lib.custom-image"
)

// Annotations written by the webhook.
const (
	// AppliedTarget is the JSON of the target that was applied to the pod.
	// Example value: {"name":"python","podSelector":{"matchLabels":{"language":"python"}},"ddTraceVersions":{"python ":"3"}}
	AppliedTarget = "internal.apm.datadoghq.com/applied-target"
	// InjectionError is set by the webhook when there was an error during mutation.
	// Example value: The overall pod's containers limit is too low, cpu pod_limit=5m needed=50m, memory pod_limit=4Mi needed=100Mi
	InjectionError = "internal.apm.datadoghq.com/injection-error"
	// InjectorCanonicalVersion is set with the actual version of the injector as opposed to a resolved digest.
	// Example value: 0.52.0
	InjectorCanonicalVersion = "internal.apm.datadoghq.com/injector-canonical-version"
	// LibraryCanonicalVersion is set with the actual version of the library as opposed to a resolved digest.
	// Example value: 3.19.2
	LibraryCanonicalVersion LibraryAnnotationFormat = "internal.apm.datadoghq.com/%s-canonical-version"
)

// LibraryAnnotationFormat is a helper type to format an annotation with a language.
type LibraryAnnotationFormat string

// Format returns the annotation formatted with the language name.
func (f LibraryAnnotationFormat) Format(lang string) string {
	return fmt.Sprintf(string(f), lang)
}

// LibraryContainerAnnotationFormat is a helper type to format an annotation with a container and language.
type LibraryContainerAnnotationFormat string

// Format returns the annotation formatted with the container and language.
func (f LibraryContainerAnnotationFormat) Format(container string, lang string) string {
	return fmt.Sprintf(string(f), container, lang)
}

// Get will return the annotation value and if the key was found in the pod annotations.
func Get(pod *corev1.Pod, key string) (string, bool) {
	if pod.Annotations == nil {
		return "", false
	}

	value, found := pod.Annotations[key]
	if found {
		log.Debugf("Found annotation %s=%s for Single Step Instrumentation.", key, value)
	}

	return value, found
}

// Set sets the value to the provided key in the pod annotations.
func Set(pod *corev1.Pod, key string, value string) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	pod.Annotations[key] = value
	log.Debugf("Set annotation %s=%s for Single Step Instrumentation.", key, value)
}
