// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	builtinReconcilePeriod = 5 * time.Minute

	// builtinLabelKey marks CRD objects managed by the built-in profile manager.
	builtinLabelKey = "autoscaling.datadoghq.com/managed-by"
	// builtinLabelValue is the value set on the BuiltinLabelKey label.
	builtinLabelValue = "datadog"
)

var (
	strategyMax = datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect

	builtinProfiles = []datadoghq.DatadogPodAutoscalerClusterProfile{
		{
			TypeMeta: podAutoscalerClusterProfileMeta,
			ObjectMeta: metav1.ObjectMeta{
				Name:   "datadog-optimize-cost",
				Labels: map[string]string{builtinLabelKey: builtinLabelValue},
			},
			Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
				Template: datadoghq.DatadogPodAutoscalerTemplate{
					ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
						Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
						ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							StabilizationWindowSeconds: 300,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 50, PeriodSeconds: 120},
							},
						},
						ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy:                   &strategyMax,
							StabilizationWindowSeconds: 300,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 50, PeriodSeconds: 120},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](1),
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
									Utilization: pointer.Ptr[int32](85),
								},
							},
						},
					},
				},
			},
		},
		{
			TypeMeta: podAutoscalerClusterProfileMeta,
			ObjectMeta: metav1.ObjectMeta{
				Name:   "datadog-optimize-balance",
				Labels: map[string]string{builtinLabelKey: builtinLabelValue},
			},
			Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
				Template: datadoghq.DatadogPodAutoscalerTemplate{
					ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
						Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
						ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy:                   &strategyMax,
							StabilizationWindowSeconds: 600,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 50, PeriodSeconds: 120},
							},
						},
						ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy:                   &strategyMax,
							StabilizationWindowSeconds: 600,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 20, PeriodSeconds: 1200},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
									Utilization: pointer.Ptr[int32](70),
								},
							},
						},
					},
				},
			},
		},
		{
			TypeMeta: podAutoscalerClusterProfileMeta,
			ObjectMeta: metav1.ObjectMeta{
				Name:   "datadog-optimize-performance",
				Labels: map[string]string{builtinLabelKey: builtinLabelValue},
			},
			Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
				Template: datadoghq.DatadogPodAutoscalerTemplate{
					ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
						Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
						ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy:                   &strategyMax,
							StabilizationWindowSeconds: 900,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 50, PeriodSeconds: 120},
							},
						},
						ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy:                   &strategyMax,
							StabilizationWindowSeconds: 900,
							Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
								{Type: datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType, Value: 10, PeriodSeconds: 1800},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](3),
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
									Utilization: pointer.Ptr[int32](60),
								},
							},
						},
					},
				},
			},
		},
	}
)

// BuiltinProfileManager ensures hardcoded built-in profiles exist as CRDs in
// the cluster. It periodically creates missing profiles and resets any whose
// spec has drifted from the hardcoded definition.
type BuiltinProfileManager struct {
	client   dynamic.Interface
	isLeader func() bool

	// expectedHashes maps profile name to its expected template hash,
	// computed once at construction time.
	expectedHashes map[string]string
}

// NewBuiltinProfileManager creates a new BuiltinProfileManager.
func NewBuiltinProfileManager(client dynamic.Interface, isLeader func() bool) *BuiltinProfileManager {
	hashes := make(map[string]string, len(builtinProfiles))
	for i := range builtinProfiles {
		h, err := autoscaling.ObjectHash(&builtinProfiles[i].Spec.Template)
		if err != nil {
			log.Errorf("Failed to hash built-in profile %s: %v", builtinProfiles[i].Name, err)
			continue
		}
		hashes[builtinProfiles[i].Name] = h
	}
	return &BuiltinProfileManager{
		client:         client,
		isLeader:       isLeader,
		expectedHashes: hashes,
	}
}

// Run starts the reconcile loop. It blocks until ctx is cancelled.
func (m *BuiltinProfileManager) Run(ctx context.Context) {
	log.Info("Starting built-in profile manager")
	m.reconcile(ctx)

	ticker := time.NewTicker(builtinReconcilePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if m.isLeader() {
				m.reconcile(ctx)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *BuiltinProfileManager) reconcile(ctx context.Context) {
	resource := m.client.Resource(podAutoscalerClusterProfileGVR)
	for i := range builtinProfiles {
		p := &builtinProfiles[i]
		m.ensureProfile(ctx, resource, p)
	}
}

func (m *BuiltinProfileManager) ensureProfile(ctx context.Context, resource dynamic.ResourceInterface, desired *datadoghq.DatadogPodAutoscalerClusterProfile) {
	existing, err := resource.Get(ctx, desired.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		obj, err := autoscaling.ToUnstructured(desired)
		if err != nil {
			log.Errorf("Failed to convert built-in profile %s: %v", desired.Name, err)
			return
		}
		if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			log.Errorf("Failed to create built-in profile %s: %v", desired.Name, err)
		} else {
			log.Infof("Created built-in profile %s", desired.Name)
		}
		return
	}
	if err != nil {
		log.Errorf("Failed to get built-in profile %s: %v", desired.Name, err)
		return
	}

	// These names are reserved — check if spec or labels need updating.
	var existingProfile datadoghq.DatadogPodAutoscalerClusterProfile
	if err := autoscaling.FromUnstructured(existing, &existingProfile); err != nil {
		log.Errorf("Failed to parse built-in profile %s: %v", desired.Name, err)
		return
	}
	existingHash, _ := autoscaling.ObjectHash(&existingProfile.Spec.Template)
	labelsMatch := existing.GetLabels()[builtinLabelKey] == builtinLabelValue
	if existingHash == m.expectedHashes[desired.Name] && labelsMatch {
		return
	}

	obj, err := autoscaling.ToUnstructured(desired)
	if err != nil {
		log.Errorf("Failed to convert built-in profile %s: %v", desired.Name, err)
		return
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	if _, err := resource.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		log.Errorf("Failed to update built-in profile %s: %v", desired.Name, err)
	} else {
		log.Infof("Updated built-in profile %s (drift detected)", desired.Name)
	}
}
