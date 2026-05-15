// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"testing"
	"time"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

const (
	enableTimeout   = 2 * time.Second
	noEnableTimeout = 50 * time.Millisecond // Shorter because we wait for the full duration
)

func TestRegisterAutoscalingGateHandlers(t *testing.T) {
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	require.NoError(t, err)

	discoveryCl := kubefake.NewSimpleClientset().Discovery()

	tests := []struct {
		name                      string
		kubernetesObjects         []runtime.Object
		kubernetesMetadataObjects []runtime.Object
		expectedEnable            bool
	}{
		{
			name: "enables gate on DPA",
			kubernetesObjects: []runtime.Object{
				&datadoghq.DatadogPodAutoscaler{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DatadogPodAutoscaler",
						APIVersion: "datadoghq.com/v1alpha2",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dpa-0",
					},
					Spec: datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: autoscalingv2.CrossVersionObjectReference{
							Kind:       "Deployment",
							Name:       "app",
							APIVersion: "apps/v1",
						},
						Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
					},
				},
			},
			expectedEnable: true,
		},
		{
			name: "does not enable on DPAClusterProfile",
			kubernetesObjects: []runtime.Object{
				&datadoghq.DatadogPodAutoscalerClusterProfile{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DatadogPodAutoscalerClusterProfile",
						APIVersion: "datadoghq.com/v1alpha2",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "profile-0",
					},
				},
			},
			expectedEnable: false,
		},
		{
			name: "enables gate on labeled Deployment",
			kubernetesMetadataObjects: []runtime.Object{
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "app",
						Labels:    map[string]string{model.ProfileLabelKey: "high-cpu"},
					},
				},
			},
			expectedEnable: true,
		},
		{
			name: "enables gate on labeled StatefulSet",
			kubernetesMetadataObjects: []runtime.Object{
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "app",
						Labels:    map[string]string{model.ProfileLabelKey: "high-cpu"},
					},
				},
			},
			expectedEnable: true,
		},
		{
			name: "enables gate on labeled Namespace",
			kubernetesMetadataObjects: []runtime.Object{
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: corev1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "ns-prod",
						Labels: map[string]string{model.ProfileLabelKey: "high-cpu"},
					},
				},
			},
			expectedEnable: true,
		},
		{
			name: "does not enable on unlabeled Deployment",
			kubernetesMetadataObjects: []runtime.Object{
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "app",
						// No profile label
					},
				},
			},
			expectedEnable: false,
		},
		{
			name:           "does not enable without resources",
			expectedEnable: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dynamicClient := dynamicfake.NewSimpleDynamicClient(kscheme.Scheme, test.kubernetesObjects...)
			dynamicInformer := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)
			metadataClient := metadatafake.NewSimpleMetadataClient(scheme, test.kubernetesMetadataObjects...)

			timeout := noEnableTimeout
			if test.expectedEnable {
				timeout = enableTimeout
			}
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			workloadResources := SupportedWorkloadResources(discoveryCl)
			gate := autoscalinggate.New()
			require.NoError(t, RegisterAutoscalingGateHandlers(ctx, dynamicInformer, metadataClient, workloadResources, gate))

			dynamicInformer.Start(ctx.Done())
			assert.Equal(t, test.expectedEnable, gate.WaitForEnable(ctx))
		})
	}
}
