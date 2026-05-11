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
)

// syntheticBuiltinCRDs returns synthetic CRD definitions for built-in Kubernetes
// RBAC resources so the backend can index their fields the same way it does for
// real custom resources.
func syntheticBuiltinCRDs() []runtime.Object {
	stringArray := v1.JSONSchemaProps{
		Type: "array",
		Items: &v1.JSONSchemaPropsOrArray{
			Schema: &v1.JSONSchemaProps{Type: "string"},
		},
	}

	policyRule := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"apiGroups":       stringArray,
			"resources":       stringArray,
			"verbs":           stringArray,
			"resourceNames":   stringArray,
			"nonResourceURLs": stringArray,
		},
	}

	rulesSchema := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"rules": {
				Type:  "array",
				Items: &v1.JSONSchemaPropsOrArray{Schema: &policyRule},
			},
		},
	}

	subject := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"kind":      {Type: "string"},
			"name":      {Type: "string"},
			"namespace": {Type: "string"},
			"apiGroup":  {Type: "string"},
		},
	}

	bindingSchema := v1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]v1.JSONSchemaProps{
			"subjects": {
				Type:  "array",
				Items: &v1.JSONSchemaPropsOrArray{Schema: &subject},
			},
			"roleRef": {
				Type: "object",
				Properties: map[string]v1.JSONSchemaProps{
					"kind":     {Type: "string"},
					"name":     {Type: "string"},
					"apiGroup": {Type: "string"},
				},
			},
		},
	}

	return []runtime.Object{
		syntheticCRD("clusterroles.rbac.authorization.k8s.io", "rbac.authorization.k8s.io", "ClusterRole", "clusterroles", v1.ClusterScoped, "00000000-0000-0000-0000-000000000001", rulesSchema),
		syntheticCRD("clusterrolebindings.rbac.authorization.k8s.io", "rbac.authorization.k8s.io", "ClusterRoleBinding", "clusterrolebindings", v1.ClusterScoped, "00000000-0000-0000-0000-000000000002", bindingSchema),
		syntheticCRD("roles.rbac.authorization.k8s.io", "rbac.authorization.k8s.io", "Role", "roles", v1.NamespaceScoped, "00000000-0000-0000-0000-000000000003", rulesSchema),
		syntheticCRD("rolebindings.rbac.authorization.k8s.io", "rbac.authorization.k8s.io", "RoleBinding", "rolebindings", v1.NamespaceScoped, "00000000-0000-0000-0000-000000000004", bindingSchema),
	}
}

func syntheticCRD(name, group, kind, plural string, scope v1.ResourceScope, uid string, schema v1.JSONSchemaProps) *v1.CustomResourceDefinition {
	return &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			UID:             types.UID(uid),
			ResourceVersion: "1",
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: group,
			Names: v1.CustomResourceDefinitionNames{
				Kind:   kind,
				Plural: plural,
			},
			Scope: scope,
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &schema,
					},
				},
			},
		},
	}
}
