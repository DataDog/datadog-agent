// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s/helm"
)

// syntheticHelmReleaseCRDs describes the synthetic HelmRelease resource.
func syntheticHelmReleaseCRDs() []runtime.Object {
	str := v1.JSONSchemaProps{Type: "string"}

	specSchema := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"spec": {
				Type: "object",
				Properties: map[string]v1.JSONSchemaProps{
					"releaseName":   str,
					"revision":      {Type: "integer"},
					"chart":         str,
					"chartVersion":  str,
					"appVersion":    str,
					"status":        str,
					"firstDeployed": str,
					"lastDeployed":  str,
					"chartRef": {
						Type: "object",
						Properties: map[string]v1.JSONSchemaProps{
							"name":    str,
							"version": str,
						},
					},
				},
			},
		},
	}

	crd := &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "helmreleases." + helm.HelmReleaseGroup,
			UID:             types.UID("00000000-0000-0000-0000-0000000000a1"),
			ResourceVersion: "1",
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: helm.HelmReleaseGroup,
			Names: v1.CustomResourceDefinitionNames{
				Kind:   helm.HelmReleaseKind,
				Plural: "helmreleases",
			},
			Scope: v1.NamespaceScoped,
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    helm.HelmReleaseVersion,
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &specSchema,
					},
				},
			},
		},
	}

	return []runtime.Object{crd}
}

// syntheticHelmChartCRDs describes the synthetic HelmChart resource.
func syntheticHelmChartCRDs() []runtime.Object {
	str := v1.JSONSchemaProps{Type: "string"}
	preserve := boolPtr(true)

	specSchema := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"spec": {
				Type: "object",
				Properties: map[string]v1.JSONSchemaProps{
					"name":        str,
					"version":     str,
					"appVersion":  str,
					"apiVersion":  str,
					"description": str,
					// Arbitrary-shaped content: preserve so it is not pruned.
					"defaultValues": {Type: "object", XPreserveUnknownFields: preserve},
					"templates": {
						Type: "array",
						Items: &v1.JSONSchemaPropsOrArray{
							Schema: &v1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: preserve},
						},
					},
					"dependencies": {
						Type: "array",
						Items: &v1.JSONSchemaPropsOrArray{
							Schema: &v1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: preserve},
						},
					},
				},
			},
		},
	}

	crd := &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "helmcharts." + helm.HelmReleaseGroup,
			UID:             types.UID("00000000-0000-0000-0000-0000000000a2"),
			ResourceVersion: "1",
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: helm.HelmReleaseGroup,
			Names: v1.CustomResourceDefinitionNames{
				Kind:   helm.HelmChartKind,
				Plural: "helmcharts",
			},
			Scope: v1.ClusterScoped,
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    helm.HelmReleaseVersion,
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &specSchema,
					},
				},
			},
		},
	}

	return []runtime.Object{crd}
}

func boolPtr(b bool) *bool { return &b }
