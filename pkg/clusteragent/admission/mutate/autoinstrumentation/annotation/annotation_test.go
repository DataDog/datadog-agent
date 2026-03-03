// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package annotation_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestLibraryAnnotationFormat(t *testing.T) {
	tests := map[string]struct {
		format   annotation.LibraryAnnotationFormat
		lang     string
		expected string
	}{
		"library version formats correctly": {
			format:   annotation.LibraryVersion,
			lang:     "python",
			expected: "admission.datadoghq.com/python-lib.version",
		},
		"library image formats correctly": {
			format:   annotation.LibraryImage,
			lang:     "python",
			expected: "admission.datadoghq.com/python-lib.custom-image",
		},
		"library canonical version formats correctly": {
			format:   annotation.LibraryCanonicalVersion,
			lang:     "python",
			expected: "internal.apm.datadoghq.com/python-canonical-version",
		},
		"library config v1 formats correctly": {
			format:   annotation.LibraryConfigV1,
			lang:     "python",
			expected: "admission.datadoghq.com/python-lib.config.v1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := test.format.Format(test.lang)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestLibraryContainerAnnotationFormat(t *testing.T) {
	tests := map[string]struct {
		format    annotation.LibraryContainerAnnotationFormat
		container string
		lang      string
		expected  string
	}{
		"library container version formats correctly": {
			format:    annotation.LibraryContainerVersion,
			container: "app",
			lang:      "python",
			expected:  "admission.datadoghq.com/app.python-lib.version",
		},
		"library container image formats correctly": {
			format:    annotation.LibraryContainerImage,
			container: "app",
			lang:      "python",
			expected:  "admission.datadoghq.com/app.python-lib.custom-image",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := test.format.Format(test.container, test.lang)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	type expected struct {
		value string
		found bool
	}
	tests := map[string]struct {
		pod      *corev1.Pod
		key      string
		expected expected
	}{
		"existing annotation returns value": {
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"foo": "bar",
				},
			}.Create(),
			key: "foo",
			expected: expected{
				value: "bar",
				found: true,
			},
		},
		"empty value for key returns found value": {
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"foo": "",
				},
			}.Create(),
			key: "foo",
			expected: expected{
				value: "",
				found: true,
			},
		},
		"missing key returns not found": {
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"bar": "zed",
				},
			}.Create(),
			key: "foo",
			expected: expected{
				value: "",
				found: false,
			},
		},
		"empty map returns unfound annotation": {
			pod: &corev1.Pod{},
			key: "foo",
			expected: expected{
				value: "",
				found: false,
			},
		},
		"nil map returns unfound annotation": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			key: "foo",
			expected: expected{
				value: "",
				found: false,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			value, found := annotation.Get(test.pod, test.key)
			require.Equal(t, test.expected.value, value, "the annotation value did not match expected")
			require.Equal(t, test.expected.found, found, "the annotation was not found")
		})
	}
}

func TestSetAnnotation(t *testing.T) {
	tests := map[string]struct {
		pod   *corev1.Pod
		key   string
		value string
	}{
		"annotation is set successfully": {
			pod:   common.FakePodSpec{}.Create(),
			key:   "foo",
			value: "bar",
		},
		"pod with annotations is extended successfully": {
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"foo": "bar",
				},
			}.Create(),
			key:   "zed",
			value: "idk",
		},
		"pod with annotation with the same key is replaced": {
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"foo": "bar",
				},
			}.Create(),
			key:   "foo",
			value: "idk",
		},
		"empty pod spec can still be set": {
			pod:   &corev1.Pod{},
			key:   "foo",
			value: "bar",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			annotation.Set(test.pod, test.key, test.value)
			actual, found := annotation.Get(test.pod, test.key)
			require.True(t, found, "annotation was not found")
			require.Equal(t, test.value, actual, "the value does not match")
		})
	}
}
