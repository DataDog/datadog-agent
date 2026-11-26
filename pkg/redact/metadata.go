// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package redact

import "sync"

const consulOriginalPodAnnotation = "consul.hashicorp.com/original-pod"

// hardcode the annotation to avoid importing k8s.io/api
const kubectlLastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

var (
	sensitiveAnnotationsAndLabels      = []string{kubectlLastAppliedConfigAnnotation, consulOriginalPodAnnotation}
	sensitiveAnnotationsAndLabelsMutex sync.Mutex
)

// UpdateSensitiveAnnotationsAndLabels adds new sensitive annotations or labels key to the list to redact.
func UpdateSensitiveAnnotationsAndLabels(annotationsAndLabels []string) {
	sensitiveAnnotationsAndLabelsMutex.Lock()
	defer sensitiveAnnotationsAndLabelsMutex.Unlock()
	sensitiveAnnotationsAndLabels = append(sensitiveAnnotationsAndLabels, annotationsAndLabels...)
}

// GetSensitiveAnnotationsAndLabels returns the list of sensitive annotations and labels.
func GetSensitiveAnnotationsAndLabels() []string {
	sensitiveAnnotationsAndLabelsMutex.Lock()
	defer sensitiveAnnotationsAndLabelsMutex.Unlock()
	return sensitiveAnnotationsAndLabels
}

// RemoveSensitiveAnnotationsAndLabels redacts sensitive annotations and labels like the whole
// "kubectl.kubernetes.io/last-applied-configuration" annotation value. As it
// may contain duplicate information and secrets.
func RemoveSensitiveAnnotationsAndLabels(annotations map[string]string, labels map[string]string) {
	for _, v := range GetSensitiveAnnotationsAndLabels() {
		if _, found := annotations[v]; found {
			annotations[v] = redactedAnnotationValue
		}
		if _, found := labels[v]; found {
			labels[v] = redactedAnnotationValue
		}
	}
}
