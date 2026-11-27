// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScrubObjectMeta scrubs sensitive annotations and labels.
func ScrubObjectMeta(meta *metav1.ObjectMeta, scrubber *DataScrubber) {
	RemoveSensitiveAnnotationsAndLabels(meta.Annotations, meta.Labels)
	scrubAnnotations(meta.Annotations, scrubber)
}
