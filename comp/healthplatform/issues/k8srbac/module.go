// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package k8srbac provides the issue template for Kubernetes RBAC forbidden errors.
// Detection happens inline in the kubelet check when a 403 response is received;
// this module only registers the template for issue building.
package k8srbac

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type k8sRBACModule struct{}

// NewModule creates a new Kubernetes RBAC issue module.
func NewModule(_ config.Component) issues.Module {
	return &k8sRBACModule{}
}

func (m *k8sRBACModule) IssueName() string { return IssueName }

func (m *k8sRBACModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return NewK8sRBACIssue().BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — detection happens inline in the kubelet check.
func (m *k8sRBACModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — detection happens inline in the kubelet check.
func (m *k8sRBACModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
