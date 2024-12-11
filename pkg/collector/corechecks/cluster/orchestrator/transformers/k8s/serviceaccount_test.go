// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractServiceAccount(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	tests := map[string]struct {
		input    corev1.ServiceAccount
		expected model.ServiceAccount
	}{
		"standard": {
			input: corev1.ServiceAccount{
				AutomountServiceAccountToken: pointer.Ptr(true),
				ImagePullSecrets: []corev1.LocalObjectReference{
					{
						Name: "registry-key",
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Name:            "service-account",
					Namespace:       "namespace",
					ResourceVersion: "1234",
					UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
				},
				Secrets: []corev1.ObjectReference{
					{
						Name: "default-token-uudge",
					},
				},
			},
			expected: model.ServiceAccount{
				AutomountServiceAccountToken: true,
				ImagePullSecrets: []*model.TypedLocalObjectReference{
					{
						Name: "registry-key",
					},
				},
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"app:my-app"},
					Name:              "service-account",
					Namespace:         "namespace",
					ResourceVersion:   "1234",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
				},
				Secrets: []*model.ObjectReference{
					{
						Name: "default-token-uudge",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractServiceAccount(&tc.input))
		})
	}
}
