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

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractClusterRoleBinding(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	tests := map[string]struct {
		input    rbacv1.ClusterRoleBinding
		expected model.ClusterRoleBinding
	}{
		"standard": {
			input: rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Name:            "cluster-role-binding",
					Namespace:       "namespace",
					ResourceVersion: "1234",
					UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "my-cluster-role",
				},
				Subjects: []rbacv1.Subject{
					{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "User",
						Name:     "firstname.lastname@company.com",
					},
				},
			},
			expected: model.ClusterRoleBinding{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"app:my-app"},
					Name:              "cluster-role-binding",
					Namespace:         "namespace",
					ResourceVersion:   "1234",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
				},
				RoleRef: &model.TypedLocalObjectReference{
					ApiGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "my-cluster-role",
				},
				Subjects: []*model.Subject{
					{
						ApiGroup: "rbac.authorization.k8s.io",
						Kind:     "User",
						Name:     "firstname.lastname@company.com",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractClusterRoleBinding(&tc.input))
		})
	}
}
