// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redact

const consulOriginalPodAnnotation = "consul.hashicorp.com/original-pod"

// hardcode the annotation to avoid importing k8s.io/api
const kubectlLastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

var sensitiveAnnotationsAndLabels = []string{kubectlLastAppliedConfigAnnotation, consulOriginalPodAnnotation}

// UpdateSensitiveAnnotationsAndLabels adds new sensitive annotations or labels key to the list to redact.
func UpdateSensitiveAnnotationsAndLabels(annotationsAndLabels []string) {
	sensitiveAnnotationsAndLabels = append(sensitiveAnnotationsAndLabels, annotationsAndLabels...)
}

// GetSensitiveAnnotationsAndLabels returns the list of sensitive annotations and labels.
func GetSensitiveAnnotationsAndLabels() []string {
	return sensitiveAnnotationsAndLabels
}
