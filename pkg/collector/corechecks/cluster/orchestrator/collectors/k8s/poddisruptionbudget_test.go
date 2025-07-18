// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestPodDisruptionBudgetCollector(t *testing.T) {
	iVal := int32(95)
	sVal := "reshape"
	iOSI := intstr.FromInt32(iVal)
	iOSS := intstr.FromString(sVal)
	var labels = map[string]string{"reshape": "all"}
	ePolicy := policyv1.AlwaysAllow
	t0 := CreateTestTime()
	t1 := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))

	podDisruptionBudget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "gwern",
			Namespace:       "kog",
			UID:             "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion: "1219",
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   &iOSI,
			MaxUnavailable: &iOSS,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UnhealthyPodEvictionPolicy: &ePolicy,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 3,
			DisruptedPods:      map[string]metav1.Time{"liborio": t0},
			DisruptionsAllowed: 4,
			CurrentHealthy:     5,
			DesiredHealthy:     6,
			ExpectedPods:       7,
			Conditions: []metav1.Condition{
				{
					Type:               "regular",
					Status:             metav1.ConditionUnknown,
					ObservedGeneration: 2,
					LastTransitionTime: t1,
					Reason:             "why not",
					Message:            "instant",
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewPodDisruptionBudgetCollectorVersion(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{podDisruptionBudget},
		ExpectedMetadataType:       &model.CollectorPodDisruptionBudget{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
