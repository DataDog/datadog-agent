// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

func Test_shouldInjectConf(t *testing.T) {
	mockConfig := config.Mock()
	tests := []struct {
		name        string
		pod         *corev1.Pod
		setupConfig func()
		want        bool
	}{
		{
			name:        "mutate unlabelled, no label",
			pod:         fakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "mutate unlabelled, label enabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "mutate unlabelled, label disabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        false,
		},
		{
			name:        "no mutate unlabelled, no label",
			pod:         fakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
		{
			name:        "no mutate unlabelled, label enabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        true,
		},
		{
			name:        "no mutate unlabelled, label disabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()
			if got := shouldInjectConf(tt.pod); got != tt.want {
				t.Errorf("shouldInjectConf() = %v, want %v", got, tt.want)
			}
		})
	}
}
