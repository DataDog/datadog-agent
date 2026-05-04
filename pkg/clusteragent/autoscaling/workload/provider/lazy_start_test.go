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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
)

func TestRegisterAutoscalingGateHandlers(t *testing.T) {
	testDPA := &datadoghq.DatadogPodAutoscaler{
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
	}

	testDPAClusterProfile := &datadoghq.DatadogPodAutoscalerClusterProfile{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscalerClusterProfile",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "profile-0",
		},
	}

	tests := []struct {
		name           string
		seedObjects    []runtime.Object
		expectedEnable bool
		timeout        time.Duration
	}{
		{
			name:           "enables gate on first DPA",
			seedObjects:    []runtime.Object{testDPA},
			expectedEnable: true,
			timeout:        2 * time.Second,
		},
		{
			name:           "enables gate on first DPAClusterProfile",
			seedObjects:    []runtime.Object{testDPAClusterProfile},
			expectedEnable: true,
			timeout:        2 * time.Second,
		},
		{
			name:           "does not enable without resources",
			seedObjects:    nil,
			expectedEnable: false,
			// shorter timeout because in this test we always wait the full
			// duration
			timeout: 50 * time.Millisecond,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleDynamicClient(kscheme.Scheme, test.seedObjects...)
			informer := dynamicinformer.NewDynamicSharedInformerFactory(fakeClient, 0)

			gate := autoscalinggate.New()
			require.NoError(t, RegisterAutoscalingGateHandlers(informer, gate))

			stopCh := make(chan struct{})
			defer close(stopCh)
			informer.Start(stopCh)

			ctx, cancel := context.WithTimeout(context.Background(), test.timeout)
			defer cancel()
			assert.Equal(t, test.expectedEnable, gate.WaitForEnable(ctx))
		})
	}
}
