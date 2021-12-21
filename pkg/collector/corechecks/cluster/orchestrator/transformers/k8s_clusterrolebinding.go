// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package transformers

import (
	model "github.com/DataDog/agent-payload/v5/process"

	rbacv1 "k8s.io/api/rbac/v1"
)

// ExtractK8sClusterRoleBinding returns the protobuf model corresponding to a
// Kubernetes ClusterRoleBinding resource.
func ExtractK8sClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) *model.ClusterRoleBinding {
	return &model.ClusterRoleBinding{
		Metadata: extractMetadata(&crb.ObjectMeta),
		RoleRef:  extractRoleRef(&crb.RoleRef),
		Subjects: extractSubjects(crb.Subjects),
	}
}

func extractRoleRef(r *rbacv1.RoleRef) *model.TypedLocalObjectReference {
	return &model.TypedLocalObjectReference{
		ApiGroup: r.APIGroup,
		Kind:     r.Kind,
		Name:     r.Name,
	}
}

func extractSubjects(s []rbacv1.Subject) []*model.Subject {
	subjects := make([]*model.Subject, 0, len(s))
	for _, subject := range s {
		subjects = append(subjects, &model.Subject{
			ApiGroup:  subject.APIGroup,
			Kind:      subject.Kind,
			Name:      subject.Name,
			Namespace: subject.Namespace,
		})
	}
	return subjects
}
