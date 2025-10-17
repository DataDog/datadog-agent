// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestClusterRoleCollector(t *testing.T) {
	creationTime := CreateTestTime()

	clusterRole := &rbacv1.ClusterRole{
		AggregationRule: &rbacv1.AggregationRule{
			ClusterRoleSelectors: []metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"rbac.example.com/aggregate-to-edit": "true"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "rbac.example.com/aggregate-to-edit",
							Operator: "In",
							Values:   []string{"true"},
						},
					},
				},
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
			Name:            "cluster-role",
			Namespace:       "namespace",
			ResourceVersion: "1202",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes", "pods", "services"},
				Verbs:     []string{"get", "patch", "list"},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{"cronjobs", "jobs"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"rolebindings"},
				Verbs:     []string{"create"},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewClusterRoleCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{clusterRole},
		ExpectedMetadataType:       &model.CollectorClusterRole{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
