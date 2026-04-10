// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v1alpha2 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

type sizeTestOptions struct {
	numContainers          int
	containerNameFn        func(i int) string
	numWorkloads           int
	numObjectives          int
	numScalingRules        int
	workloadNameLength     int
	errorRate              float64
	errorMessageLength     int
	includeManualOverrides bool
	includeHorizontal      bool
	includeVertical        bool
	includeCPU             bool
	includeMemory          bool
	includeRequests        bool
	includeLimits          bool
}

type sizeTestOption func(*sizeTestOptions)

func withNumContainers(n int) sizeTestOption {
	return func(o *sizeTestOptions) { o.numContainers = n }
}

func withContainerNameLength(length int) sizeTestOption {
	return func(o *sizeTestOptions) {
		o.containerNameFn = func(i int) string {
			suffix := fmt.Sprintf("-%d", i)
			padLen := length - len(suffix)
			if padLen < 1 {
				padLen = 1
			}
			name := make([]byte, padLen)
			for j := range name {
				name[j] = 'a' + byte(j%26)
			}
			return string(name) + suffix
		}
	}
}

func withNumWorkloads(n int) sizeTestOption {
	return func(o *sizeTestOptions) { o.numWorkloads = n }
}

func withNumObjectives(n int) sizeTestOption {
	return func(o *sizeTestOptions) { o.numObjectives = n }
}

func withNumScalingRules(n int) sizeTestOption {
	return func(o *sizeTestOptions) { o.numScalingRules = n }
}

func withWorkloadNameLength(n int) sizeTestOption {
	return func(o *sizeTestOptions) { o.workloadNameLength = n }
}

func withErrorRate(rate float64) sizeTestOption {
	return func(o *sizeTestOptions) { o.errorRate = rate }
}

func withErrorMessageLength(length int) sizeTestOption {
	return func(o *sizeTestOptions) { o.errorMessageLength = length }
}

func withManualOverrides() sizeTestOption {
	return func(o *sizeTestOptions) { o.includeManualOverrides = true }
}

func withHorizontal(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeHorizontal = v }
}

func withVertical(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeVertical = v }
}

func withCPU(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeCPU = v }
}

func withMemory(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeMemory = v }
}

func withRequests(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeRequests = v }
}

func withLimits(v bool) sizeTestOption {
	return func(o *sizeTestOptions) { o.includeLimits = v }
}

func generatePaddedString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = 'a' + byte(i%26)
	}
	return string(b)
}

func defaultSizeTestOptions() sizeTestOptions {
	return sizeTestOptions{
		numContainers: 4,
		containerNameFn: func(i int) string {
			return fmt.Sprintf("somewhat-large-container-name-%d", i)
		},
		numWorkloads:       1,
		numObjectives:      3,
		numScalingRules:    5,
		workloadNameLength: 27,
		errorRate:          1.0,
		errorMessageLength: 92,
		includeHorizontal:  true,
		includeVertical:    true,
		includeCPU:         true,
		includeMemory:      true,
		includeRequests:    true,
		includeLimits:      true,
	}
}

func generateScalingPolicy(numRules int) *datadoghqcommon.DatadogPodAutoscalerScalingPolicy {
	rules := make([]datadoghqcommon.DatadogPodAutoscalerScalingRule, numRules)
	for i := range numRules {
		rules[i] = datadoghqcommon.DatadogPodAutoscalerScalingRule{
			Type:          datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType,
			Value:         50,
			PeriodSeconds: 3000,
		}
	}

	return &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
		Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect),
		Rules:    rules,
	}
}

func generateTargets(numTargets int, containerName string) []datadoghqcommon.DatadogPodAutoscalerObjective {
	targets := make([]datadoghqcommon.DatadogPodAutoscalerObjective, numTargets)
	for i := range numTargets {
		targets[i] = datadoghqcommon.DatadogPodAutoscalerObjective{
			Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
			ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
				Name: corev1.ResourceMemory,
				Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
					Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					Utilization: pointer.Ptr[int32](80),
				},
				Container: containerName,
			},
		}
	}

	return targets
}

func generateResourceList(opts sizeTestOptions) corev1.ResourceList {
	rl := corev1.ResourceList{}
	if opts.includeMemory {
		rl[corev1.ResourceMemory] = resource.MustParse("25000m")
	}
	if opts.includeCPU {
		rl[corev1.ResourceCPU] = resource.MustParse("4500Mi")
	}
	return rl
}

func generateContainerConstraints(opts sizeTestOptions) []datadoghqcommon.DatadogPodAutoscalerContainerConstraints {
	constraints := make([]datadoghqcommon.DatadogPodAutoscalerContainerConstraints, opts.numContainers)
	for i := range opts.numContainers {
		constraints[i] = datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			Name:    opts.containerNameFn(i),
			Enabled: pointer.Ptr(true),
			Requests: &datadoghqcommon.DatadogPodAutoscalerContainerResourceConstraints{
				MinAllowed: generateResourceList(opts),
				MaxAllowed: generateResourceList(opts),
			},
		}
	}

	return constraints
}

func generateDatadogPodAutoscaler(opts sizeTestOptions) *v1alpha2.DatadogPodAutoscaler {
	spec := v1alpha2.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "relatively-large-name-for-a-deployment",
			APIVersion: "apps/v1",
		},
		Owner:         datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		RemoteVersion: pointer.Ptr[uint64](500000),
		Objectives:    generateTargets(opts.numObjectives, opts.containerNameFn(0)),
		ApplyPolicy: &v1alpha2.DatadogPodAutoscalerApplyPolicy{
			Mode: v1alpha2.DatadogPodAutoscalerApplyModeApply,
			Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
				Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
			},
		},
	}

	if opts.includeHorizontal {
		spec.ApplyPolicy.ScaleUp = generateScalingPolicy(opts.numScalingRules)
		spec.ApplyPolicy.ScaleDown = generateScalingPolicy(opts.numScalingRules)
	}

	if opts.includeVertical {
		spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MinReplicas: pointer.Ptr[int32](100),
			MaxReplicas: pointer.Ptr[int32](2000),
			Containers:  generateContainerConstraints(opts),
		}
	}

	return &v1alpha2.DatadogPodAutoscaler{Spec: spec}
}

func generateContainerResourceList(opts sizeTestOptions) []*kubeAutoscaling.ContainerResources_ResourceList {
	var rl []*kubeAutoscaling.ContainerResources_ResourceList
	if opts.includeCPU {
		rl = append(rl, &kubeAutoscaling.ContainerResources_ResourceList{
			Name:  corev1.ResourceCPU.String(),
			Value: "4500m",
		})
	}
	if opts.includeMemory {
		rl = append(rl, &kubeAutoscaling.ContainerResources_ResourceList{
			Name:  corev1.ResourceMemory.String(),
			Value: "25000Mi",
		})
	}
	return rl
}

func generateWorkloadValues(opts sizeTestOptions, now time.Time, hasError bool) *kubeAutoscaling.WorkloadValues {
	ts := timestamppb.New(now)

	containerResources := make([]*kubeAutoscaling.ContainerResources, opts.numContainers)
	for i := range opts.numContainers {
		cr := &kubeAutoscaling.ContainerResources{
			ContainerName: opts.containerNameFn(i),
		}
		if opts.includeRequests {
			cr.Requests = generateContainerResourceList(opts)
		}
		if opts.includeLimits {
			cr.Limits = generateContainerResourceList(opts)
		}
		containerResources[i] = cr
	}

	workloadName := generatePaddedString(opts.workloadNameLength)
	wv := &kubeAutoscaling.WorkloadValues{
		Namespace: workloadName,
		Name:      workloadName,
	}

	if opts.includeHorizontal {
		wv.Horizontal = &kubeAutoscaling.WorkloadHorizontalValues{
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas:  pointer.Ptr[int32](100),
				Timestamp: ts,
			},
		}
		if opts.includeManualOverrides {
			wv.Horizontal.Manual = &kubeAutoscaling.WorkloadHorizontalData{
				Replicas:  pointer.Ptr[int32](100),
				Timestamp: ts,
			}
		}
	}

	if opts.includeVertical {
		wv.Vertical = &kubeAutoscaling.WorkloadVerticalValues{
			Auto: &kubeAutoscaling.WorkloadVerticalData{
				Timestamp: ts,
				Resources: containerResources,
			},
		}
		if opts.includeManualOverrides {
			wv.Vertical.Manual = &kubeAutoscaling.WorkloadVerticalData{
				Timestamp: ts,
				Resources: containerResources,
			}
		}
	}

	if hasError {
		errMsg := generatePaddedString(opts.errorMessageLength)
		makeError := func() *kubeAutoscaling.Error {
			return &kubeAutoscaling.Error{
				Code:    pointer.Ptr[int32](500),
				Message: errMsg,
			}
		}
		wv.Error = makeError()
		if wv.Horizontal != nil {
			wv.Horizontal.Error = makeError()
		}
		if wv.Vertical != nil {
			wv.Vertical.Error = makeError()
		}
	}

	return wv
}

func generateSettingsAndValues(opts sizeTestOptions) ([]AutoscalingSettings, *kubeAutoscaling.WorkloadValuesList) {
	now := time.Now().Truncate(time.Second)

	settings := make([]AutoscalingSettings, opts.numWorkloads)
	values := make([]*kubeAutoscaling.WorkloadValues, opts.numWorkloads)
	numWithErrors := int(float64(opts.numWorkloads) * opts.errorRate)
	workloadName := generatePaddedString(opts.workloadNameLength)
	for i := range opts.numWorkloads {
		dpa := generateDatadogPodAutoscaler(opts)
		settings[i] = AutoscalingSettings{
			Namespace: workloadName,
			Name:      workloadName,
			Specs: &AutoscalingSpecs{
				V1Alpha2: &dpa.Spec,
			},
		}
		values[i] = generateWorkloadValues(opts, now, i < numWithErrors)
	}

	return settings, &kubeAutoscaling.WorkloadValuesList{Values: values}
}

func applySizeTestOptions(optFns ...sizeTestOption) sizeTestOptions {
	opts := defaultSizeTestOptions()
	for _, fn := range optFns {
		fn(&opts)
	}
	return opts
}

func TestConfigurationSize(t *testing.T) {
	maxSize := 1024 * 1024

	tests := []struct {
		name                              string
		opts                              []sizeTestOption
		expectedSettings                  int
		expectedValues                    int
		expectedMaxAutoscalersFitSettings int
		expectedMaxAutoscalersFitValues   int
	}{
		{
			name:                              "default (4 containers, 1 workload)",
			opts:                              nil,
			expectedSettings:                  2102,
			expectedValues:                    1456,
			expectedMaxAutoscalersFitSettings: 498,
			expectedMaxAutoscalersFitValues:   720,
		},
		{
			name:                              "realistic",
			opts:                              []sizeTestOption{withNumContainers(2), withContainerNameLength(8), withWorkloadNameLength(10), withNumWorkloads(6000), withErrorRate(0.1)},
			expectedSettings:                  9804001,
			expectedValues:                    3622812,
			expectedMaxAutoscalersFitSettings: 641,
			expectedMaxAutoscalersFitValues:   1738,
		},
		{
			name:                              "vertical only, memory only",
			opts:                              []sizeTestOption{withNumContainers(2), withContainerNameLength(8), withWorkloadNameLength(10), withNumWorkloads(6000), withErrorRate(0.1), withVertical(true), withHorizontal(false), withCPU(false)},
			expectedSettings:                  5904001,
			expectedValues:                    2359212,
			expectedMaxAutoscalersFitSettings: 1065,
			expectedMaxAutoscalersFitValues:   2668,
		},
		{
			name:                              "horizontal only, requests only, cpu only",
			opts:                              []sizeTestOption{withNumContainers(2), withContainerNameLength(8), withWorkloadNameLength(10), withNumWorkloads(6000), withErrorRate(0.1), withHorizontal(true), withVertical(false), withMemory(false), withLimits(false)},
			expectedSettings:                  7764001,
			expectedValues:                    877212,
			expectedMaxAutoscalersFitSettings: 810,
			expectedMaxAutoscalersFitValues:   7182,
		},
		{
			name:                              "manual overrides, custom objectives and rules",
			opts:                              []sizeTestOption{withNumContainers(2), withContainerNameLength(8), withWorkloadNameLength(10), withNumWorkloads(6000), withErrorRate(0.1), withManualOverrides(), withNumObjectives(5), withNumScalingRules(3), withErrorMessageLength(200), withRequests(true), withLimits(true)},
			expectedSettings:                  10236001,
			expectedValues:                    6787212,
			expectedMaxAutoscalersFitSettings: 614,
			expectedMaxAutoscalersFitValues:   927,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := applySizeTestOptions(tc.opts...)
			settings, values := generateSettingsAndValues(opts)

			settingsBytes, err := json.Marshal(settings)
			if err != nil {
				t.Fatalf("failed to marshal settings: %v", err)
			}
			valuesBytes, err := json.Marshal(values)
			if err != nil {
				t.Fatalf("failed to marshal values: %v", err)
			}

			assert.Equal(t, tc.expectedSettings, len(settingsBytes), "Size of settings JSON")
			assert.Equal(t, tc.expectedValues, len(valuesBytes), "Size of values JSON")

			perAutoscalerSettings := len(settingsBytes) / opts.numWorkloads
			perAutoscalerValues := len(valuesBytes) / opts.numWorkloads
			assert.Equal(t, tc.expectedMaxAutoscalersFitSettings, maxSize/perAutoscalerSettings, "Max autoscalers fitting in settings")
			assert.Equal(t, tc.expectedMaxAutoscalersFitValues, maxSize/perAutoscalerValues, "Max autoscalers fitting in values")
		})
	}
}
