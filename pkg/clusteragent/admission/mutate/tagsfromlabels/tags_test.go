// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var scheme = kscheme.Scheme

func Test_injectTagsFromLabels(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
		found       bool
		injected    bool
	}{
		{
			name:   "nominal case",
			labels: map[string]string{"tags.datadoghq.com/env": "dev", "tags.datadoghq.com/service": "dd-agent", "tags.datadoghq.com/version": "7"},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "no labels",
			labels: map[string]string{},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				return *pod
			},
			found:    false,
			injected: false,
		},
		{
			name:   "env only",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "label found but not injected",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    common.FakePodWithEnv("foo-pod", "DD_ENV"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePodWithEnv("foo-pod", "DD_ENV")
				return *pod
			},
			found:    true,
			injected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, injected := injectTagsFromLabels(tt.labels, tt.pod)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.injected, injected)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
}

func Test_injectTags(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
	}{
		{
			name: "tag labels and injection on",
			pod: common.WithLabels(
				common.FakePod("foo-pod"),
				map[string]string{
					"admission.datadoghq.com/enabled": "true",
					"tags.datadoghq.com/env":          "dev",
					"tags.datadoghq.com/service":      "dd-agent",
					"tags.datadoghq.com/version":      "7",
				},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
		},
		{
			name: "no labels and injection on",
			pod:  common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"}),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				return *pod
			},
		},
		{
			name: "env only and injection on",
			pod: common.WithLabels(
				common.FakePod("foo-pod"),
				map[string]string{"admission.datadoghq.com/enabled": "true", "tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
		},
		{
			name: "tag label found but not injected, injection on",
			pod: common.WithLabels(
				common.FakePodWithEnv("foo-pod", "DD_ENV"),
				map[string]string{"admission.datadoghq.com/enabled": "true", "tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePodWithEnv("foo-pod", "DD_ENV"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				return *pod
			},
		},
		{
			name: "tag label found but not injected, injection label not set",
			pod: common.WithLabels(
				common.FakePodWithEnv("foo-pod", "DD_ENV"),
				map[string]string{"tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePodWithEnv("foo-pod", "DD_ENV")
				return *pod
			},
		},
	}
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	datadogConfig := fxutil.Test[config.Component](t, core.MockBundle())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewFilter(datadogConfig)
			assert.NoError(t, err)
			mutator := NewMutator(NewMutatorConfig(datadogConfig), filter)
			webhook := NewWebhook(wmeta, datadogConfig, mutator)
			_, err = webhook.inject(tt.pod, "ns", nil)
			assert.NoError(t, err)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
}
