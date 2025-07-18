// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestServiceAccountCollector(t *testing.T) {
	creationTime := CreateTestTime()

	serviceAccount := &corev1.ServiceAccount{
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
			ResourceVersion: "1224",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: "default-token-uudge",
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewServiceAccountCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{serviceAccount},
		ExpectedMetadataType:       &model.CollectorServiceAccount{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
