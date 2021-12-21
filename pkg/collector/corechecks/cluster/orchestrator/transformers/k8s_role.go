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

// ExtractK8sRole returns the protobuf model corresponding to a Kubernetes Role
// resource.
func ExtractK8sRole(r *rbacv1.Role) *model.Role {
	return &model.Role{
		Metadata: extractMetadata(&r.ObjectMeta),
		Rules:    extractPolicyRules(r.Rules),
	}
}
