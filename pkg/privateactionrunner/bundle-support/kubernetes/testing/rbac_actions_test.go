// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"testing"

	rbac "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/rbac"
)

func TestGeneratedRBACActionsRejectMissingInputs(t *testing.T) {
	assertGeneratedActionsRejectMissingInputs(t, rbac.NewKubernetesRbac(), []generatedActionResource{
		{name: "Role", namespaced: true},
		{name: "RoleBinding", namespaced: true},
		{name: "ClusterRole"},
		{name: "ClusterRoleBinding"},
	})
}
