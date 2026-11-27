// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

// ScrubClusterRole scrubs the metadata of a ClusterRole.
func ScrubClusterRole(r *rbacv1.ClusterRole, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubClusterRoleBinding scrubs the metadata of a ClusterRoleBinding.
func ScrubClusterRoleBinding(r *rbacv1.ClusterRoleBinding, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubRole scrubs the metadata of a Role.
func ScrubRole(r *rbacv1.Role, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubRoleBinding scrubs the metadata of a RoleBinding.
func ScrubRoleBinding(r *rbacv1.RoleBinding, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}
