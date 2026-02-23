// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestNewCRDParser(t *testing.T) {
	tests := []struct {
		name     string
		crd      *apiextensionsv1.CustomResourceDefinition
		expected *workloadmeta.CRD
	}{
		{
			name: "single_version_with_storage",
			crd: &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "datadoghq.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind: "DatadogAgent",
					},
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v2alpha1",
							Storage: true,
						},
					},
				},
			},
			expected: &workloadmeta.CRD{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindCRD,
					ID:   "datadogagents.datadoghq.com",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Group:   "datadoghq.com",
				Kind:    "DatadogAgent",
				Version: "v2alpha1",
			},
		},
		{
			name: "multiple_versions_with_storage",
			crd: &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "datadoghq.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind: "DatadogAgent",
					},
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Storage: false,
						},
						{
							Name:    "v1",
							Storage: true,
						},
					},
				},
			},
			expected: &workloadmeta.CRD{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindCRD,
					ID:   "datadogagents.datadoghq.com",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Group:   "datadoghq.com",
				Kind:    "DatadogAgent",
				Version: "v1",
			},
		},
		{
			name: "multiple_versions_without_storage",
			crd: &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "datadoghq.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind: "DatadogAgent",
					},
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Storage: false,
						},
						{
							Name:    "v1",
							Storage: false,
						},
					},
				},
			},
			expected: &workloadmeta.CRD{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindCRD,
					ID:   "datadogagents.datadoghq.com",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "datadogagents.datadoghq.com",
					Namespace:   "datadog",
					Labels:      map[string]string{"app": "datadog", "app.kubernetes.io/name": "datadogCRDs"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					UID:         "12345",
				},
				Group:   "datadoghq.com",
				Kind:    "DatadogAgent",
				Version: "v1alpha1",
			},
		},
	}

	parser := NewCRDParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := parser.Parse(tt.crd)

			crdEntity, ok := entity.(*workloadmeta.CRD)
			require.True(t, ok, "expected entity to be of type *workloadmeta.CRD but got %T", entity)

			require.Equal(t, tt.expected, crdEntity, "parsed CRD does not match expected")
		})
	}
}
