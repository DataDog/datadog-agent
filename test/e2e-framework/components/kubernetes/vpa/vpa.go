// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package vpa

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiextensions "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

func DeployCRD(e config.Env, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*apiextensions.CustomResourceDefinition, error) {
	opts = append(opts, pulumi.Providers(kubeProvider), pulumi.DeletedWith(kubeProvider))

	// This is the definition from https://github.com/kubernetes/autoscaler/blob/6d1a1514af4406bb967ae6f50e9589f7d41f9af8/vertical-pod-autoscaler/deploy/vpa-v1-crd-gen.yaml
	return apiextensions.NewCustomResourceDefinition(e.Ctx(), "vertical-pod-autoscaler", &apiextensions.CustomResourceDefinitionArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("verticalpodautoscalers.autoscaling.k8s.io"),
			Annotations: pulumi.StringMap{
				"api-approved.kubernetes.io": pulumi.String("https://github.com/kubernetes/kubernetes/pull/63797"),
			},
		},
		Spec: &apiextensions.CustomResourceDefinitionSpecArgs{
			Group: pulumi.String("autoscaling.k8s.io"),
			Scope: pulumi.String("Namespaced"),
			Names: &apiextensions.CustomResourceDefinitionNamesArgs{
				Kind:       pulumi.String("VerticalPodAutoscaler"),
				ListKind:   pulumi.String("VerticalPodAutoscalerList"),
				Singular:   pulumi.String("verticalpodautoscaler"),
				Plural:     pulumi.String("verticalpodautoscalers"),
				ShortNames: pulumi.StringArray{pulumi.String("vpa")},
			},
			Versions: &apiextensions.CustomResourceDefinitionVersionArray{
				&apiextensions.CustomResourceDefinitionVersionArgs{
					Name:    pulumi.String("v1beta2"),
					Served:  pulumi.Bool(true),
					Storage: pulumi.Bool(false),
					Schema: &apiextensions.CustomResourceValidationArgs{
						OpenAPIV3Schema: &apiextensions.JSONSchemaPropsArgs{
							Type: pulumi.String("object"),
							Properties: apiextensions.JSONSchemaPropsMap{
								"spec": apiextensions.JSONSchemaPropsArgs{
									Type: pulumi.String("object"),
									Properties: apiextensions.JSONSchemaPropsMap{
										"targetRef": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
											Properties: apiextensions.JSONSchemaPropsMap{
												"apiVersion": apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
												"kind":       apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
												"name":       apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
											},
											Required: pulumi.StringArray{
												pulumi.String("apiVersion"),
												pulumi.String("kind"),
												pulumi.String("name"),
											},
										},
										"updatePolicy": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
											Properties: apiextensions.JSONSchemaPropsMap{
												"updateMode": apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
											},
										},
										"resourcePolicy": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
										},
									},
									Required: pulumi.StringArray{pulumi.String("targetRef")},
								},
								"status": apiextensions.JSONSchemaPropsArgs{
									Type: pulumi.String("object"),
								},
							},
						},
					},
				},
				&apiextensions.CustomResourceDefinitionVersionArgs{
					Name:    pulumi.String("v1"),
					Served:  pulumi.Bool(true),
					Storage: pulumi.Bool(true),
					AdditionalPrinterColumns: apiextensions.CustomResourceColumnDefinitionArray{
						&apiextensions.CustomResourceColumnDefinitionArgs{
							Name:        pulumi.String("Target"),
							Type:        pulumi.String("string"),
							JsonPath:    pulumi.String(".spec.targetRef.name"),
							Description: pulumi.String("Name of the target resource"),
						},
						&apiextensions.CustomResourceColumnDefinitionArgs{
							Name:     pulumi.String("Age"),
							Type:     pulumi.String("date"),
							JsonPath: pulumi.String(".metadata.creationTimestamp"),
						},
					},
					Schema: &apiextensions.CustomResourceValidationArgs{
						OpenAPIV3Schema: &apiextensions.JSONSchemaPropsArgs{
							Type: pulumi.String("object"),
							Properties: apiextensions.JSONSchemaPropsMap{
								"spec": apiextensions.JSONSchemaPropsArgs{
									Type: pulumi.String("object"),
									Properties: apiextensions.JSONSchemaPropsMap{
										"targetRef": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
											Properties: apiextensions.JSONSchemaPropsMap{
												"apiVersion": apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
												"kind":       apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
												"name":       apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
											},
											Required: pulumi.StringArray{
												pulumi.String("apiVersion"),
												pulumi.String("kind"),
												pulumi.String("name"),
											},
										},
										"updatePolicy": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
											Properties: apiextensions.JSONSchemaPropsMap{
												"updateMode": apiextensions.JSONSchemaPropsArgs{Type: pulumi.String("string")},
											},
										},
										"resourcePolicy": apiextensions.JSONSchemaPropsArgs{
											Type: pulumi.String("object"),
										},
									},
									Required: pulumi.StringArray{pulumi.String("targetRef")},
								},
								"status": apiextensions.JSONSchemaPropsArgs{
									Type: pulumi.String("object"),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
}
