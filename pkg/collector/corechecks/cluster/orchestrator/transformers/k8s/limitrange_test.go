// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestExtractResourceLimit(t *testing.T) {
	input := corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name: "limit-range",
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("50Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					MaxLimitRequestRatio: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("2"),
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("10Mi"),
					},
					Type: corev1.LimitTypeContainer,
				},
			},
		},
	}

	expected := &model.LimitRange{
		LimitTypes: []string{"Container"},
		Metadata: &model.Metadata{
			Name: "limit-range",
		},
		Spec: &model.LimitRangeSpec{
			Limits: []*model.LimitRangeItem{
				{
					Default: map[string]int64{
						"cpu":    100,
						"memory": 104857600,
					},
					DefaultRequest: map[string]int64{
						"cpu":    50,
						"memory": 52428800,
					},
					Max: map[string]int64{
						"cpu":    200,
						"memory": 209715200,
					},
					MaxLimitRequestRatio: map[string]int64{
						"cpu":    2,
						"memory": 2,
					},
					Min: map[string]int64{
						"cpu":    10,
						"memory": 10485760,
					},
					Type: "Container",
				},
			},
		},
	}

	out := ExtractLimitRange(&input)
	assert.Equal(t, expected, out)
}
