// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractIntOrString(t *testing.T) {
	iVal := int32(95)
	sVal := "reshape"
	iOS95 := intstr.FromInt32(iVal)
	iOSStr := intstr.FromString(sVal)
	for name, tc := range map[string]struct {
		in     *intstr.IntOrString
		expect *model.IntOrString
	}{
		"nil": {
			in:     nil,
			expect: nil,
		},
		"int": {
			in: &iOS95,
			expect: &model.IntOrString{
				Type:   model.IntOrString_Int,
				IntVal: iVal,
			},
		},
		"str": {
			in: &iOSStr,
			expect: &model.IntOrString{
				Type:   model.IntOrString_String,
				StrVal: sVal,
			},
		},
		"empty": {
			in: &intstr.IntOrString{},
			expect: &model.IntOrString{
				Type:   model.IntOrString_Int,
				IntVal: 0,
			},
		},
		"unknown": {
			in: &intstr.IntOrString{
				Type: intstr.Type(95),
			},
			expect: nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := extractIntOrString(tc.in)
			if tc.expect == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tc.expect, got)
			}
		})
	}
}

func TestExtractPodDisruptionBudgetSpec(t *testing.T) {
	iVal1 := int32(95)
	iVal2 := int32(99)
	iOS1 := intstr.FromInt32(iVal1)
	iOS2 := intstr.FromInt32(iVal2)
	var labels = map[string]string{"reshape": "all"}

	sVal1 := "110%"
	sVal2 := "111%"
	iOS3 := intstr.FromString(sVal1)
	iOS4 := intstr.FromString(sVal2)

	ePolicy := policyv1.AlwaysAllow

	for name, tc := range map[string]struct {
		in     *policyv1.PodDisruptionBudgetSpec
		expect *model.PodDisruptionBudgetSpec
	}{
		"nil": {
			in:     nil,
			expect: nil,
		},
		"allInts": {
			in: &policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   &iOS1,
				MaxUnavailable: &iOS2,
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
			expect: &model.PodDisruptionBudgetSpec{
				MinAvailable: &model.IntOrString{
					Type:   model.IntOrString_Int,
					IntVal: iVal1,
				},
				MaxUnavailable: &model.IntOrString{
					Type:   model.IntOrString_Int,
					IntVal: iVal2,
				},
				Selector: []*model.LabelSelectorRequirement{
					{
						Key:      "reshape",
						Operator: "In",
						Values:   []string{"all"},
					},
				},
			},
		},
		"allStrings": {
			in: &policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   &iOS3,
				MaxUnavailable: &iOS4,
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				UnhealthyPodEvictionPolicy: &ePolicy,
			},
			expect: &model.PodDisruptionBudgetSpec{
				MinAvailable: &model.IntOrString{
					Type:   model.IntOrString_String,
					StrVal: sVal1,
				},
				MaxUnavailable: &model.IntOrString{
					Type:   model.IntOrString_String,
					StrVal: sVal2,
				},
				Selector: []*model.LabelSelectorRequirement{
					{
						Key:      "reshape",
						Operator: "In",
						Values:   []string{"all"},
					},
				},
				UnhealthyPodEvictionPolicy: string(policyv1.AlwaysAllow),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := extractPodDisruptionBudgetSpec(tc.in)
			if tc.expect == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tc.expect, got)
			}
		})
	}
}

func TestExtractPodDisruptionBudget(t *testing.T) {
	iVal := int32(95)
	sVal := "reshape"
	iOSI := intstr.FromInt32(iVal)
	iOSS := intstr.FromString(sVal)
	var labels = map[string]string{"reshape": "all"}
	ePolicy := policyv1.AlwaysAllow

	t0 := time.Now()
	t1 := t0.Add(time.Minute)

	for name, tc := range map[string]struct {
		in     *policyv1.PodDisruptionBudget
		expect *model.PodDisruptionBudget
	}{
		"nil": {
			in:     nil,
			expect: nil,
		},
		"empty": {
			in: &policyv1.PodDisruptionBudget{},
			expect: &model.PodDisruptionBudget{
				Metadata: &model.Metadata{
					Name:              "",
					Namespace:         "",
					Uid:               "",
					CreationTimestamp: 0,
					DeletionTimestamp: 0,
				},
				Spec: &model.PodDisruptionBudgetSpec{
					UnhealthyPodEvictionPolicy: "",
				},
				Status: &model.PodDisruptionBudgetStatus{
					DisruptedPods: map[string]int64{},
					Conditions:    []*model.Condition{},
				},
			},
		},
		"full": {
			in: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "gwern",
					Namespace:       "kog",
					UID:             "513",
					ResourceVersion: "platinum",
					Labels: map[string]string{
						kubernetes.VersionTagLabelKey: "ultimate",
						kubernetes.ServiceTagLabelKey: "honorable",
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
					DisruptedPods:      map[string]metav1.Time{"liborio": metav1.NewTime(t0)},
					DisruptionsAllowed: 4,
					CurrentHealthy:     5,
					DesiredHealthy:     6,
					ExpectedPods:       7,
					Conditions: []metav1.Condition{
						{
							Type:               "regular",
							Status:             metav1.ConditionUnknown,
							ObservedGeneration: 2,
							LastTransitionTime: metav1.NewTime(t1),
							Reason:             "why not",
							Message:            "instant",
						},
					},
				},
			},
			expect: &model.PodDisruptionBudget{
				Metadata: &model.Metadata{
					Name:              "gwern",
					Namespace:         "kog",
					Uid:               "513",
					ResourceVersion:   "platinum",
					CreationTimestamp: 0,
					DeletionTimestamp: 0,
					Labels: []string{
						fmt.Sprintf("%s:ultimate", kubernetes.VersionTagLabelKey),
						fmt.Sprintf("%s:honorable", kubernetes.ServiceTagLabelKey),
					},
				},
				Spec: &model.PodDisruptionBudgetSpec{
					MinAvailable: &model.IntOrString{
						Type:   model.IntOrString_Int,
						IntVal: iVal,
					},
					MaxUnavailable: &model.IntOrString{
						Type:   model.IntOrString_String,
						StrVal: sVal,
					},
					Selector: []*model.LabelSelectorRequirement{
						{
							Key:      sVal,
							Operator: "In",
							Values:   []string{"all"},
						},
					},
					UnhealthyPodEvictionPolicy: string(ePolicy),
				},
				Status: &model.PodDisruptionBudgetStatus{
					DisruptedPods:      map[string]int64{"liborio": t0.Unix()},
					DisruptionsAllowed: 4,
					CurrentHealthy:     5,
					DesiredHealthy:     6,
					ExpectedPods:       7,
					Conditions: []*model.Condition{
						{
							Type:               "regular",
							Status:             string(metav1.ConditionUnknown),
							LastTransitionTime: t1.Unix(),
							Reason:             "why not",
							Message:            "instant",
						},
					},
				},
				Tags: []string{
					"version:ultimate",
					"service:honorable",
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ExtractPodDisruptionBudget(tc.in)
			if tc.expect == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tc.expect, got)
			}
		})
	}
}

func TestExtractExtractPodDisruptionBudgetStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		in     *policyv1.PodDisruptionBudgetStatus
		expect *model.PodDisruptionBudgetStatus
	}{
		"nil": {
			in:     nil,
			expect: nil,
		},
		"empty": {
			in: &policyv1.PodDisruptionBudgetStatus{},
			expect: &model.PodDisruptionBudgetStatus{
				Conditions:    []*model.Condition{},
				DisruptedPods: map[string]int64{},
			},
		},
		"full": {
			in: &policyv1.PodDisruptionBudgetStatus{
				DisruptedPods:      map[string]metav1.Time{"liborio": metav1.NewTime(time.Now())},
				DisruptionsAllowed: 4,
				CurrentHealthy:     5,
				DesiredHealthy:     6,
				ExpectedPods:       7,
				Conditions: []metav1.Condition{
					{
						Type:               "regular",
						Status:             metav1.ConditionUnknown,
						ObservedGeneration: 2,
						LastTransitionTime: metav1.NewTime(time.Now()),
						Reason:             "why not",
						Message:            "instant",
					},
				},
			},
			expect: &model.PodDisruptionBudgetStatus{
				DisruptedPods:      map[string]int64{"liborio": time.Now().Unix()},
				DisruptionsAllowed: 4,
				CurrentHealthy:     5,
				DesiredHealthy:     6,
				ExpectedPods:       7,
				Conditions: []*model.Condition{
					{
						Type:               "regular",
						Status:             string(metav1.ConditionUnknown),
						LastTransitionTime: time.Now().Unix(),
						Reason:             "why not",
						Message:            "instant",
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := extractPodDisruptionBudgetStatus(tc.in)
			if tc.expect == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tc.expect, got)
			}
		})
	}
}
