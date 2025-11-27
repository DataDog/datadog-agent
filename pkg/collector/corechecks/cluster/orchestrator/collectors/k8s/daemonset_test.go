// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestDaemonSetCollector(t *testing.T) {
	testIntOrStrPercent := intstr.FromString("1%")
	timestamp := metav1.NewTime(CreateTestTime().Time)

	daemonSet := &v1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "daemonset",
			Namespace:       "namespace",
			Labels:          map[string]string{"app": "my-app"},
			Annotations:     map[string]string{"annotation": "my-annotation"},
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
			ResourceVersion: "1208",
		},
		Spec: v1.DaemonSetSpec{
			UpdateStrategy: v1.DaemonSetUpdateStrategy{
				Type: v1.DaemonSetUpdateStrategyType("RollingUpdate"),
				RollingUpdate: &v1.RollingUpdateDaemonSet{
					MaxUnavailable: &testIntOrStrPercent,
				},
			},
		},
		Status: v1.DaemonSetStatus{
			Conditions: []v1.DaemonSetCondition{
				{
					Type:               "Test",
					Status:             corev1.ConditionFalse,
					LastTransitionTime: timestamp,
					Reason:             "test reason",
					Message:            "test message",
				},
			},
			CurrentNumberScheduled: 1,
			NumberReady:            1,
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewDaemonSetCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{daemonSet},
		ExpectedMetadataType:       &model.CollectorDaemonSet{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
