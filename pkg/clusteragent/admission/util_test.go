// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package admission

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getWebhookSkeleton(t *testing.T) {
	failurePolicy := admiv1beta1.Ignore
	sideEffects := admiv1beta1.SideEffectClassNone
	port := int32(443)
	path := "/bar"
	type args struct {
		nameSuffix string
		path       string
	}
	tests := []struct {
		name string
		args args
		want admiv1beta1.MutatingWebhook
	}{
		{
			name: "nominal case",
			args: args{
				nameSuffix: "foo",
				path:       "/bar",
			},
			want: admiv1beta1.MutatingWebhook{
				Name: "datadog.webhook.foo",
				ClientConfig: admiv1beta1.WebhookClientConfig{
					Service: &admiv1beta1.ServiceReference{
						Namespace: "default",
						Name:      "datadog-admission-controller",
						Port:      &port,
						Path:      &path,
					},
				},
				Rules: []admiv1beta1.RuleWithOperations{
					{
						Operations: []admiv1beta1.OperationType{
							admiv1beta1.Create,
						},
						Rule: admiv1beta1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
				FailurePolicy: &failurePolicy,
				SideEffects:   &sideEffects,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getWebhookSkeleton(tt.args.nameSuffix, tt.args.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getWebhookSkeleton() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_generateWebhooks(t *testing.T) {
	mockConfig := config.Mock()
	tests := []struct {
		name        string
		setupConfig func()
		want        func() []admiv1beta1.MutatingWebhook
	}{
		{
			name: "config injection, mutate all",
			setupConfig: func() {
				mockConfig.Set("admission_controller.inject_config.enabled", true)
				mockConfig.Set("admission_controller.mutate_unlabelled", true)
				mockConfig.Set("admission_controller.inject_tags.enabled", false)
			},
			want: func() []admiv1beta1.MutatingWebhook {
				webhook := getWebhookSkeleton("config", "/injectconfig")
				webhook.ObjectSelector = &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "admission.datadoghq.com/enabled",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"false"},
						},
					},
				}
				return []admiv1beta1.MutatingWebhook{webhook}
			},
		},
		{
			name: "config injection, mutate labelled",
			setupConfig: func() {
				mockConfig.Set("admission_controller.inject_config.enabled", true)
				mockConfig.Set("admission_controller.mutate_unlabelled", false)
				mockConfig.Set("admission_controller.inject_tags.enabled", false)
			},
			want: func() []admiv1beta1.MutatingWebhook {
				webhook := getWebhookSkeleton("config", "/injectconfig")
				webhook.ObjectSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
				}
				return []admiv1beta1.MutatingWebhook{webhook}
			},
		},
		{
			name: "tags injection",
			setupConfig: func() {
				mockConfig.Set("admission_controller.inject_config.enabled", false)
				mockConfig.Set("admission_controller.inject_tags.enabled", true)
			},
			want: func() []admiv1beta1.MutatingWebhook {
				webhook := getWebhookSkeleton("tags", "/injecttags")
				webhook.ObjectSelector = &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "tags.datadoghq.com/env",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
						{
							Key:      "tags.datadoghq.com/service",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
						{
							Key:      "tags.datadoghq.com/version",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
					},
				}
				return []admiv1beta1.MutatingWebhook{webhook}
			},
		},
		{
			name: "config and tags injection",
			setupConfig: func() {
				mockConfig.Set("admission_controller.inject_config.enabled", true)
				mockConfig.Set("admission_controller.inject_tags.enabled", true)
			},
			want: func() []admiv1beta1.MutatingWebhook {
				webhookConfig := getWebhookSkeleton("config", "/injectconfig")
				webhookConfig.ObjectSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
				}
				webhookTags := getWebhookSkeleton("tags", "/injecttags")
				webhookTags.ObjectSelector = &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "tags.datadoghq.com/env",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
						{
							Key:      "tags.datadoghq.com/service",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
						{
							Key:      "tags.datadoghq.com/version",
							Operator: metav1.LabelSelectorOpExists,
							Values:   []string{},
						},
					},
				}
				return []admiv1beta1.MutatingWebhook{webhookConfig, webhookTags}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()
			if got := generateWebhooks(); !reflect.DeepEqual(got, tt.want()) {
				t.Errorf("generateWebhooks() = %v, want %v", got, tt.want())
			}
		})
	}
}
