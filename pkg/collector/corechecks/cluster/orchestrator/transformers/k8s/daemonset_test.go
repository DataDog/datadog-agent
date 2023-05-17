// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"testing"
)

func TestExtractDaemonset(t *testing.T) {
	testIntOrStr := intstr.FromString("1%")

	tests := map[string]struct {
		input    v1.DaemonSet
		expected model.DaemonSet
	}{
		"empty ds": {input: v1.DaemonSet{}, expected: model.DaemonSet{Metadata: &model.Metadata{}, Spec: &model.DaemonSetSpec{}, Status: &model.DaemonSetStatus{}}},
		"ds with resources": {
			input: v1.DaemonSet{
				Spec: v1.DaemonSetSpec{Template: getTemplateWithResourceRequirements()},
			},
			expected: model.DaemonSet{
				Metadata: &model.Metadata{},
				Spec:     &model.DaemonSetSpec{ResourceRequirements: getExpectedModelResourceRequirements()},
				Status:   &model.DaemonSetStatus{},
			},
		},
		"partial ds": {
			input: v1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "daemonset",
					Namespace: "namespace",
				},
				Spec: v1.DaemonSetSpec{
					UpdateStrategy: v1.DaemonSetUpdateStrategy{
						Type: v1.DaemonSetUpdateStrategyType("RollingUpdate"),
						RollingUpdate: &v1.RollingUpdateDaemonSet{
							MaxSurge:       &testIntOrStr,
							MaxUnavailable: &testIntOrStr,
						},
					},
				},
				Status: v1.DaemonSetStatus{
					CurrentNumberScheduled: 1,
					NumberReady:            1,
				},
			}, expected: model.DaemonSet{
				Metadata: &model.Metadata{
					Name:      "daemonset",
					Namespace: "namespace",
				},
				Spec: &model.DaemonSetSpec{
					DeploymentStrategy: "RollingUpdate",
					MaxUnavailable:     "1%",
				},
				Status: &model.DaemonSetStatus{
					CurrentNumberScheduled: 1,
					NumberReady:            1,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractDaemonSet(&tc.input))
		})
	}
}
