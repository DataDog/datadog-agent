// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sensitiveObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: "app",
		Annotations: map[string]string{
			kubectlLastAppliedConfigAnnotation: "applied",
			"settings":                         `{"password":"sensitive-value"}`,
			"team":                             "platform",
		},
		Labels: map[string]string{
			consulOriginalPodAnnotation: "original",
			"app":                       "web",
		},
	}
}

func assertScrubbedObjectMeta(t *testing.T, meta metav1.ObjectMeta) {
	t.Helper()
	assert.Equal(t, "app", meta.Name)
	assert.Equal(t, map[string]string{
		kubectlLastAppliedConfigAnnotation: redactedAnnotationValue,
		"settings":                         `{"password":"********"}`,
		"team":                             "platform",
	}, meta.Annotations)
	assert.Equal(t, map[string]string{
		consulOriginalPodAnnotation: redactedAnnotationValue,
		"app":                       "web",
	}, meta.Labels)
}

func TestScrubObjectMeta(t *testing.T) {
	meta := sensitiveObjectMeta()
	ScrubObjectMeta(&meta, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, meta)
}

func TestScrubObjectMetaEmpty(t *testing.T) {
	meta := metav1.ObjectMeta{Name: "app"}
	ScrubObjectMeta(&meta, NewDefaultDataScrubber())
	assert.Equal(t, metav1.ObjectMeta{Name: "app"}, meta)
}
