// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package com_datadoghq_kubernetes_kubeactions implements the
// "com.datadoghq.kubernetes.kubeactions" PAR bundle: the Kubernetes actions
// (delete pod, restart/patch/rollback deployment, get resource) migrated from
// the remote-config-driven pkg/clusteragent/kubeactions subsystem.
package com_datadoghq_kubernetes_kubeactions

import (
	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// KubernetesKubeActions is the bundle exposing the migrated Kubernetes actions.
type KubernetesKubeActions struct {
	actions map[string]types.Action
}

// NewKubernetesKubeActions builds the bundle, wiring each action to a handler
// that reports lifecycle events through the kubeactions component.
func NewKubernetesKubeActions(ka kubeactions.Component) *KubernetesKubeActions {
	return &KubernetesKubeActions{
		actions: map[string]types.Action{
			kubeactions.ActionNameDeletePod:          NewDeletePodHandler(ka),
			kubeactions.ActionNameRestartDeployment:  NewRestartDeploymentHandler(ka),
			kubeactions.ActionNamePatchDeployment:    NewPatchDeploymentHandler(ka),
			kubeactions.ActionNameRollbackDeployment: NewRollbackDeploymentHandler(ka),
			kubeactions.ActionNameGetResource:        NewGetResourceHandler(ka),
		},
	}
}

// GetAction returns the handler for the named action, or nil if unknown.
func (h *KubernetesKubeActions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}

// newReport builds an ActionReport from the action type, resource reference and
// task metadata (org ID and job ID).
func newReport(actionType string, r kubeactions.ResourceRef, task *types.Task) kubeactions.ActionReport {
	report := kubeactions.ReportFromResource(actionType, r)
	if task != nil && task.Data.Attributes != nil {
		report.OrgID = task.Data.Attributes.OrgId
		report.ActionID = task.Data.Attributes.JobId
	}
	return report
}
