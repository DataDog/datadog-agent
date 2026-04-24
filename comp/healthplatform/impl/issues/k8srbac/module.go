// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package k8srbac provides an issue module for Kubernetes RBAC / kubelet 403 errors.
// This module only provides remediation (no built-in check) as 403 errors are
// reported by the kubelet or API server check on receiving a Forbidden response.
package k8srbac

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Kubernetes RBAC forbidden issues
	IssueID = "k8s-rbac-forbidden"
)

// k8sRBACModule implements issues.Module
type k8sRBACModule struct {
	template *K8sRBACIssue
}

// NewModule creates a new Kubernetes RBAC issue module
func NewModule(config.Component) issues.Module {
	return &k8sRBACModule{
		template: NewK8sRBACIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *k8sRBACModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *k8sRBACModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - 403 errors are reported by the kubelet/API server check
func (m *k8sRBACModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
